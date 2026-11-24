# CLAUDE.md

SNY Cosmos Application Tracker: a server-rendered Go app (templ + htmx + PostgreSQL) for ~20 staff to track students' university applications. Scope and decisions: `implementation-plan.md`. Operations (Railway deploy, backups, restore drill): `docs/RUNBOOK.md`.

Status: plan milestones 1–8 are built. Milestone 9 (go-live on Railway) is next. The data-residency question (Singapore vs India) is still open.

## Run it

```bash
./scripts/dev.sh            # starts Postgres (Docker, or local install in .dev/), migrates, seeds demo data, serves
./scripts/dev.sh stop       # stop the database
./scripts/dev.sh reset      # wipe .dev/ and start fresh
```

- Ports are picked automatically and remembered in `.dev/PORT` and `.dev/DB_PORT`. The app URL is printed at startup.
- Demo logins (one per role, shared password) are in `.dev/demo-users.txt`.
- Needs Go 1.26+ and either Docker or PostgreSQL 14+. On macOS Homebrew Postgres, `pg_ctl` must run with `LC_ALL=C` (the script does this).
- The server embeds static files and templates, so restart it after changing `internal/web/**`.

## Test and lint

```bash
TEST_DATABASE_URL="postgres://sny:sny@localhost:$(cat .dev/DB_PORT)/sny_tracker?sslmode=disable" go test ./...
go vet ./... && staticcheck ./...
```

- DB tests skip without `TEST_DATABASE_URL`. Each test creates and drops its own database (the user needs `CREATEDB`).
- `internal/testapp` runs the full server against a fresh database and gives signed-in clients per role. End-to-end tests live in `internal/server/*_test.go`.
- CI (`.github/workflows/ci.yml`) also fails if generated code is stale.

## Rules for changing the code

- **Visibility is decided only in `internal/access`** (`Scope`, `StudentFilter`). Every student or application query and write must AND in `access.StudentFilter`. Out-of-scope records return **404**, never 403. If you add an endpoint, add it to the matrix in `internal/server/access_test.go`.
- **Generated code is committed.** After editing `*.templ`, run `templ generate`. After editing `internal/db/queries/*.sql` or migrations, run `sqlc generate`. Tool versions are pinned in `Makefile` (`make tools`).
- **Migrations** (`internal/db/migrations`, goose, embedded, run at startup): never edit an applied migration. Add a new numbered file with both Up and Down; `TestMigrationsReversible` checks this.
- Fixed queries use sqlc (`internal/db/gen`). Filtered lists (students, admin applications, CSV export) are hand-written pgx with `access.Args` placeholders. The admin list and the CSV export share `applications.Where`, so the export always matches the screen.
- **No inline JS.** The CSP is `script-src 'self'`, so behaviour goes in `internal/web/static/app.js` (data-attributes such as `data-confirm` and `data-copy`). Every POST form needs `@layout.CSRF()`; htmx sends the token via the `X-CSRF-Token` header.
- Never log student data. Request logs record method, path and status only.
- Countries are an admin-managed list (`countries` table, Universities & consultancies → Countries tab). The application form's Country dropdown comes first and narrows the university search; a list university always keeps its own country.
- Only the student's email is required (unique, case- and space-insensitive). "Other" universities stay on that one application and are never added to the list. "Other" consultancies typed by the agent network ARE added to the list.
- **Lists are paged** (50 per page, `model.Page`, `?page=N`): count first, then fetch only that page. Per-row extras (student app counts, universities, latest status) are computed only for the page's rows via a CTE. Keep it that way; the target is 1 lakh+ students.
- **Exports**: `POST /applications/export` streams immediately up to `export.DefaultDirectLimit` (5,000) rows; above that it creates a row in `exports` and `export.Runner` builds the file in the background from one repeatable-read snapshot (gzip in the DB, deleted after 24 h, requeued after a restart). Admin pages poll `/exports/notify` (htmx; status 286 stops polling) to show the "ready" notice. Measured locally: 200,000 rows in ~0.7 s.
- Roles: `admin` (everything), `consultant` (internal students of their own branch, one active consultant per branch), `agent_network` (all B2B students).

## Working preferences (from the repo owner)

- Commit after each completed step with a clear message, on `main`.
- Remote: `github.com/samhithagollamudi/application-tracker` (push via `gh` auth).
