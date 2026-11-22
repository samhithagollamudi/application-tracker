#!/bin/sh
# Restore a backup from R2 into a TARGET database (for restore drills or recovery).
#
#   TARGET_DATABASE_URL=postgres://... ./scripts/restore.sh [object-key]
#
# Without object-key the newest backup is used. Encrypted (.age) backups need
# BACKUP_AGE_IDENTITY_FILE (the age private key file, kept offline).
# Refuses to touch a non-empty database unless FORCE=1.
# Needs: aws CLI, pg_restore/psql 16+, age (for .age files). Same R2 env vars as backup.sh.
set -eu

: "${TARGET_DATABASE_URL:?set TARGET_DATABASE_URL}" "${R2_ENDPOINT:?}" "${R2_BUCKET:?}" "${AWS_ACCESS_KEY_ID:?}" "${AWS_SECRET_ACCESS_KEY:?}"
PREFIX="${BACKUP_PREFIX:-sny-tracker}"
export AWS_DEFAULT_REGION=auto

KEY="${1:-}"
if [ -z "$KEY" ]; then
  KEY=$(aws s3 ls "s3://$R2_BUCKET/$PREFIX/" --recursive --endpoint-url "$R2_ENDPOINT" \
    | sort | tail -n 1 | awk '{print $4}')
fi
[ -n "$KEY" ] || { echo "no backups found under $PREFIX/" >&2; exit 1; }
echo "restoring s3://$R2_BUCKET/$KEY"

TABLES=$(psql "$TARGET_DATABASE_URL" -tAc "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'")
if [ "$TABLES" != "0" ] && [ "${FORCE:-}" != "1" ]; then
  echo "target database is not empty ($TABLES tables). Use a fresh database, or FORCE=1 to overwrite." >&2
  exit 1
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
aws s3 cp "s3://$R2_BUCKET/$KEY" "$TMP/backup" --endpoint-url "$R2_ENDPOINT" --only-show-errors
case "$KEY" in
  *.age)
    : "${BACKUP_AGE_IDENTITY_FILE:?encrypted backup: set BACKUP_AGE_IDENTITY_FILE}"
    age -d -i "$BACKUP_AGE_IDENTITY_FILE" -o "$TMP/backup.dump" "$TMP/backup" ;;
  *) mv "$TMP/backup" "$TMP/backup.dump" ;;
esac

pg_restore --clean --if-exists --no-owner --no-privileges --exit-on-error -d "$TARGET_DATABASE_URL" "$TMP/backup.dump"

echo "restored. Sanity check:"
psql "$TARGET_DATABASE_URL" -c "SELECT (SELECT count(*) FROM students) AS students, (SELECT count(*) FROM applications) AS applications, (SELECT count(*) FROM users) AS users, (SELECT max(version_id) FROM goose_db_version) AS schema_version"
