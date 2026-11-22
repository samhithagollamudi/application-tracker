# Runbook — SNY Cosmos Application Tracker

Operational notes for running the tracker on Railway with backups to Cloudflare R2.

## Services

| Service | Source | Notes |
|---|---|---|
| `app` | `docker/Dockerfile` (via `railway.toml`) | Go server. Health check `GET /healthz` (pings the DB). |
| `postgres` | Railway PostgreSQL 16 | Turn on PITR / scheduled backups on the Pro plan. |
| `backup` | `docker/backup/Dockerfile` | Cron service, nightly `pg_dump` → R2. |

Region: Singapore (pending SNY Cosmos's answer on data residency, see plan §9).

## App environment variables

| Variable | Value |
|---|---|
| `DATABASE_URL` | `${{Postgres.DATABASE_URL}}` (Railway reference) |
| `APP_ENV` | `production` — secure `__Host-` cookie, HSTS, trusts Railway's `X-Forwarded-For` for rate limiting |
| `PORT` | set by Railway automatically |

Migrations run automatically at startup (embedded goose files). A failed migration stops the deploy; Railway keeps the previous version running.

## First deploy

1. Create the project (Pro), add PostgreSQL, add the `app` service from the GitHub repo. `railway.toml` selects the Dockerfile and health check.
2. Set the variables above. Deploy; check `/healthz` returns `ok`.
3. Create the first admin from a Railway shell on the app service:
   ```
   /server create-admin --email admin@snycosmos.com --name "Admin"
   ```
   The password is printed **once**. Store it in the admin's password manager.
4. Sign in, open **Universities & consultancies** and add the real lists, then **Users & branches** to create one consultant per branch and the agent-network user(s). Each password is shown once; hand it over in person or by phone, never by email.
5. Add the custom domain (e.g. `apply.snycosmos.com`) in Railway and point DNS at it.

Never run `seed-demo` against production (it refuses when `APP_ENV=production`, and only runs on an empty database anyway).

## Backups

### Setup (once)

1. Cloudflare R2: create a private bucket (e.g. `sny-tracker-backups`) and an API token scoped to that bucket with Object Read & Write.
2. Add a **lifecycle rule** on the bucket: delete objects after 35 days.
3. Generate an encryption key pair on an admin's laptop (`brew install age`):
   ```
   age-keygen -o sny-backup-key.txt
   ```
   The public key (`age1...`) goes into Railway. **The private key file never goes to Railway**; keep two offline copies (e.g. the admin's password manager + a sealed USB). Without it, encrypted backups cannot be restored.
4. Add the `backup` service: same repo, Dockerfile path `docker/backup/Dockerfile`, cron schedule `30 20 * * *` (02:00 IST), no public networking. Variables:

| Variable | Value |
|---|---|
| `DATABASE_URL` | `${{Postgres.DATABASE_URL}}` |
| `R2_ENDPOINT` | `https://<account-id>.r2.cloudflarestorage.com` |
| `R2_BUCKET` | bucket name |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | R2 API token |
| `BACKUP_AGE_RECIPIENT` | the `age1...` public key |

Each run writes `sny-tracker/YYYY/MM/sny-<timestamp>.dump.age` and fails (red in Railway) if the upload is missing or under 1 KB. Check the cron history weekly.

### Restore drill (do this before go-live, then quarterly)

1. Create a **separate** empty database (a new Railway Postgres, or local: `createdb sny_restore_test`).
2. On a machine with `aws`, `age` and Postgres 16 client tools, with the R2 variables exported:
   ```
   TARGET_DATABASE_URL=postgres://.../sny_restore_test \
   BACKUP_AGE_IDENTITY_FILE=~/sny-backup-key.txt \
   ./scripts/restore.sh            # newest backup; or pass an object key
   ```
3. The script prints counts of students, applications and users and the schema version. Compare with the live admin screens. Optionally point a local server at the restored DB (`DATABASE_URL=... go run ./cmd/server`) and sign in.
4. Drop the test database. Note the date and result.

The script refuses a non-empty target unless `FORCE=1`.

### Real recovery

- Recent mistake (minutes–days): prefer Railway's point-in-time restore on the Postgres service.
- Lost database: create a new Railway Postgres, restore the newest R2 backup into it with `scripts/restore.sh`, switch the app's `DATABASE_URL`, redeploy.

## Routine admin tasks (in the app)

- **Forgotten password**: Users & branches → Reset password. The new password is shown once; the user is signed out everywhere.
- **Staff member leaves**: Deactivate. Their sessions end immediately. A consultant must be deactivated before a replacement can be created for that branch.
- **Student in the wrong branch**: open the student (admin) → Move to branch. The owner becomes that branch's consultant.
- **Duplicate/typo in a list**: entries can be removed only while unused.

## Exports

- Up to 5,000 rows: the CSV downloads immediately.
- More than that: the export runs in the background (2 workers). The admin keeps working and gets a notice with a Download link; the **Exports** page lists the last 20 with progress. Files are stored gzip-compressed in the `exports` table and deleted after 24 hours (a 200,000-row export is ~49 MB of CSV, ~3.6 MB stored).
- A deploy/restart while an export runs is safe: it is queued again and rebuilt on startup.
- Each file is read from one consistent database snapshot, so rows edited during the export are never duplicated or skipped.

## Logs and monitoring

- Logs are JSON (`slog`) with a request ID per request (`X-Request-ID` response header). Only method, path, status and timing are logged — never query strings or form bodies, so student data stays out of logs.
- `csv export` / `csv export queued` / `csv export ready` lines record who exported, how many rows and how long it took.
- Database size is shown at the bottom of **Users & branches**.

## Security notes

- Passwords: argon2id; generated (≈114 bits); no self sign-up, no email.
- Sessions: server-side in Postgres, 12 h max / 4 h idle, rotated at login; bumping `users.auth_version` (reset/deactivate) invalidates all of a user's sessions.
- CSRF token on every POST (form field or `X-CSRF-Token` header for htmx).
- Login rate limiting: 5 failures per user ID and 20 per IP per 15 minutes (in-memory; resets on deploy).
- Visibility is enforced in one place (`internal/access`), covered by `internal/server/access_test.go`. Out-of-scope records return 404.
- CSP allows scripts from self only; the site sends `noindex` and `robots.txt` disallows all.
