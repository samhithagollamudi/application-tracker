#!/bin/sh
# Nightly backup: pg_dump (custom format) -> optional age encryption -> Cloudflare R2.
# Runs as a Railway cron service built from docker/backup/Dockerfile.
#
# Required env: DATABASE_URL, R2_ENDPOINT (https://<account>.r2.cloudflarestorage.com),
#               R2_BUCKET, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY (R2 API token)
# Optional env: BACKUP_AGE_RECIPIENT (age public key; strongly recommended - the
#               dump contains student personal data), BACKUP_PREFIX (default sny-tracker)
set -eu

: "${DATABASE_URL:?}" "${R2_ENDPOINT:?}" "${R2_BUCKET:?}" "${AWS_ACCESS_KEY_ID:?}" "${AWS_SECRET_ACCESS_KEY:?}"
PREFIX="${BACKUP_PREFIX:-sny-tracker}"
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
YEAR=$(date -u +%Y)
MONTH=$(date -u +%m)
export AWS_DEFAULT_REGION=auto

KEY="$PREFIX/$YEAR/$MONTH/sny-$STAMP.dump"
if [ -n "${BACKUP_AGE_RECIPIENT:-}" ]; then
  KEY="$KEY.age"
  pg_dump --format=custom --no-owner --no-privileges "$DATABASE_URL" \
    | age -r "$BACKUP_AGE_RECIPIENT" \
    | aws s3 cp - "s3://$R2_BUCKET/$KEY" --endpoint-url "$R2_ENDPOINT" --only-show-errors
else
  echo "WARNING: BACKUP_AGE_RECIPIENT not set; uploading an unencrypted dump" >&2
  pg_dump --format=custom --no-owner --no-privileges "$DATABASE_URL" \
    | aws s3 cp - "s3://$R2_BUCKET/$KEY" --endpoint-url "$R2_ENDPOINT" --only-show-errors
fi

# Fail loudly if the object is missing or suspiciously small (< 1 KB).
SIZE=$(aws s3api head-object --bucket "$R2_BUCKET" --key "$KEY" --endpoint-url "$R2_ENDPOINT" \
  --query ContentLength --output text)
if [ "$SIZE" -lt 1024 ]; then
  echo "backup $KEY is only $SIZE bytes - something is wrong" >&2
  exit 1
fi
echo "backup ok: s3://$R2_BUCKET/$KEY ($SIZE bytes)"
