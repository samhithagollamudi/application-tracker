# SNY Cosmos Application Tracker

Go + templ + htmx + PostgreSQL. See [implementation-plan.md](implementation-plan.md) for scope and decisions.

## Local development

Requirements: Go 1.26+, PostgreSQL 16 (via `make db-up` with Docker, or any local install).

```bash
make tools                 # templ, sqlc, goose, staticcheck into $(go env GOPATH)/bin
cp .env.example .env       # point DATABASE_URL at your Postgres
make db-up                 # optional: Postgres 16 in Docker on :5432
make run                   # regenerates code, builds, migrates, serves on :8080
```

- `make generate` after editing `*.templ` or `internal/db/queries/*.sql`. Generated code is committed.
- `make test` — DB tests use `TEST_DATABASE_URL` (falls back to `DATABASE_URL`); each test creates and drops its own database, so the user needs `CREATEDB`. Without a URL they are skipped.
- `make lint` — `go vet` + `staticcheck`.

## Commands

```
server            migrate, then serve HTTP
server migrate    apply migrations and exit
```

## Configuration (environment only)

| Variable | Required | Notes |
|---|---|---|
| `DATABASE_URL` | yes | Postgres connection string |
| `SESSION_KEY` | production | 64 hex chars: `openssl rand -hex 32` |
| `PORT` | no | default 8080 |
| `APP_ENV` | no | `development` (default) or `production` |

## Layout

```
cmd/server/            entry point, routing, graceful shutdown
internal/config/       env parsing
internal/db/           pool, embedded goose migrations, sqlc queries + generated code
internal/httpx/        request IDs, logging (paths only, no student data), recovery, security headers
internal/web/          templ layout/pages, embedded static assets (CSS ported from the prototype)
docker/Dockerfile      multi-stage, distroless
```
