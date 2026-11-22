# SNY Cosmos Application Tracker

Go + templ + htmx + PostgreSQL. Scope and decisions: [implementation-plan.md](implementation-plan.md). Operations (deploy, backups, restore drill): [docs/RUNBOOK.md](docs/RUNBOOK.md).

## Local development

Requirements: Go 1.26+, PostgreSQL 16 (`make db-up` with Docker, or any local install).

```bash
make tools                 # templ, sqlc, goose, staticcheck into $(go env GOPATH)/bin
cp .env.example .env       # point DATABASE_URL at your Postgres
make db-up                 # optional: Postgres 16 in Docker on :5432
make run                   # regenerates code, builds, migrates, serves on :8080
```

Sample data for trying it out (development only, empty database only):

```bash
go run ./cmd/server seed-demo      # prints demo users (one per role) and their shared password
```

- `make generate` after editing `*.templ` or `internal/db/queries/*.sql`. Generated code is committed.
- `make test` — DB tests use `TEST_DATABASE_URL` (falls back to `DATABASE_URL`); each test creates and drops its own database, so the user needs `CREATEDB`. Without a URL they are skipped.
- `make lint` — `go vet` + `staticcheck`.

## Commands

```
server                                      migrate, then serve HTTP
server migrate                              apply migrations and exit
server create-admin --email E --name "N"    create an admin; prints a generated password once
server seed-demo [--domain demo.local]      sample data (refused in production / non-empty DB)
```

## Configuration (environment only)

| Variable | Required | Notes |
|---|---|---|
| `DATABASE_URL` | yes | Postgres connection string |
| `PORT` | no | default 8080 |
| `APP_ENV` | no | `development` (default) or `production` (secure cookies, HSTS, trusts `X-Forwarded-For`) |

## Layout

```
cmd/server/            entry point: config, pool, migrations, commands, graceful shutdown
internal/
  config/              env parsing
  db/                  pool, embedded goose migrations, sqlc queries + generated code, dbtest helper
  access/              Scope + StudentFilter: the ONLY place visibility is decided
  auth/                login, sessions (scs/Postgres), CSRF, rate limiting, role middleware
  students/            list, detail, create, edit, branch move
  applications/        add/edit/delete, inline status, admin search
  catalog/             universities, consultancies, branches (+ admin management)
  users/               admin user management
  export/              streaming CSV
  activity/            per-student activity log
  demo/                seed-demo data
  server/              routing + end-to-end tests (access matrix, flows, admin, export)
  testapp/             test harness: full server on a fresh DB, signed-in clients per role
  web/                 templ layout/pages/components, embedded static assets
docker/                app Dockerfile; backup/ job Dockerfile
scripts/               backup.sh (nightly pg_dump → R2), restore.sh (drill/recovery)
```
