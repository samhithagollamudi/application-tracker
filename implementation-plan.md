# SNY Cosmos Application Tracker — Implementation Plan

Status: agreed direction as of 22 Nov 2026. Clickable prototype: https://claude.ai/artifact/2xxU4cze33oLjESzgT12BC (the reference for screens and behaviour).

## 1. Decisions so far

| Topic | Decision |
|---|---|
| Users | ~15–20 staff. Roles: **Admin**, **Consultant** (one per branch, 5 branches), **Agent network** (B2B vertical, no branch). |
| Visibility | Admin: everything. Consultant: internal students of own branch only. Agent network: all B2B students only. Consultants and agent network never see each other's students. |
| Accounts | Admin creates users; password generated and shown once; no self sign-up; no email sending. |
| Form | One form for all universities and both roles (fields from the SNY Cosmos field doc). Agent network form adds a **Consultancy** dropdown. |
| Required fields | Only **email**: required, valid format, unique per student (case-insensitive, trimmed). Everything else optional. |
| Duplicates | Same email → "student already exists" with a link if the user can see them; otherwise "already registered, contact admin" without details. |
| Universities | Admin-maintained list (name + country). "Other" on an application is **one-off**: stored on that application only, never added to the list (both roles). |
| Consultancies | Admin-maintained list (name). Agent network "Other" **adds** to the list. |
| Branch moves | Admin only, internal students only. |
| Statuses | Applied, Admitted, Reserve, Rejected, Withdrawn, Fees paid, Visa granted, Visa rejected. |
| Export | CSV of filtered applications, one row per application, columns follow the form. |
| Uploads | None. |
| Stack | Go service rendering HTML (templ + htmx, a little plain JS), PostgreSQL. |
| Hosting | Railway Pro (~$20/mo), Singapore region; nightly pg_dump to Cloudflare R2 (free tier). Data-residency question still to confirm with SNY Cosmos before go-live. |

## 2. Tech stack

- **Go 1.24+**, standard `net/http` mux (method + path patterns), `log/slog`.
- **PostgreSQL 16+**, driver **pgx/v5** (pgxpool), queries via **sqlc**, migrations via **goose** (embedded, run on start).
- **templ** for HTML components; **htmx** for partial updates; ~50 lines of plain JS (university search box, unsaved-changes guard).
- Sessions: **alexedwards/scs** with Postgres store; passwords **argon2id**; CSRF tokens on every form; login rate limiting.
- CSS: the prototype's stylesheet, ported as one static file (no CSS framework).
- Tests: Go `testing` + **testcontainers-go** (real Postgres) for repository/access tests; `httptest` for handlers.
- Packaging: one Docker image (multi-stage, distroless), config only via environment variables.

## 3. Repository layout

```
sny-tracker/
  cmd/server/main.go          # wiring: config, db pool, migrations, router, graceful shutdown
  internal/
    config/                   # env parsing (DATABASE_URL, SESSION_KEY, PORT, APP_ENV)
    db/
      migrations/             # goose SQL files (embedded)
      queries/                # *.sql for sqlc
      gen/                    # sqlc output (committed)
    auth/                     # login, sessions, password hashing, current-user middleware
    access/                   # Scope type + rules (the ONLY place visibility is decided)
    students/                 # service + handlers: list, detail, create, edit, move
    applications/             # service + handlers: add, edit, status change, delete
    catalog/                  # universities + consultancies
    users/                    # admin user management, password reset
    export/                   # CSV streaming
    activity/                 # activity log writes/reads
    web/
      layout/  components/  pages/   # templ files
      static/                 # css, htmx.min.js, app.js, logo
  docker/Dockerfile
  sqlc.yaml  Makefile  .env.example  README.md
```

Each feature package = `service.go` (business rules, takes `access.Scope`) + `handlers.go` (HTTP, templ rendering). Handlers never query the DB directly.

## 4. Database schema (first migration)

```sql
CREATE TABLE branches (id smallserial PRIMARY KEY, name text NOT NULL UNIQUE);

CREATE TABLE users (
  id bigserial PRIMARY KEY,
  name text NOT NULL,
  email citext NOT NULL UNIQUE,
  role text NOT NULL CHECK (role IN ('admin','consultant','agent_network')),
  branch_id smallint REFERENCES branches(id),
  password_hash text NOT NULL,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((role = 'consultant') = (branch_id IS NOT NULL))
);
CREATE UNIQUE INDEX one_active_consultant_per_branch ON users(branch_id) WHERE role = 'consultant' AND active;

CREATE TABLE universities (
  id bigserial PRIMARY KEY, name text NOT NULL, country text NOT NULL,
  created_by bigint REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX universities_name_uq ON universities (lower(name));

CREATE TABLE consultancies (
  id bigserial PRIMARY KEY, name text NOT NULL,
  created_by bigint REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX consultancies_name_uq ON consultancies (lower(name));

CREATE TABLE students (
  id bigserial PRIMARY KEY,
  source text NOT NULL CHECK (source IN ('internal','b2b')),
  branch_id smallint REFERENCES branches(id),
  consultancy_id bigint REFERENCES consultancies(id),
  owner_id bigint NOT NULL REFERENCES users(id),
  first_name text, family_name text, gender text, date_of_birth date,
  email citext NOT NULL, address text,
  ba_university text, ba_specialisation text, ba_duration text, ba_grade text,
  ma_university text, ma_specialisation text, ma_duration text, ma_grade text,
  xii_board text, xii_subjects text, xii_grade text,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((source = 'internal') = (branch_id IS NOT NULL)),
  CHECK (source = 'b2b' OR consultancy_id IS NULL)
);
CREATE UNIQUE INDEX students_email_uq ON students (lower(btrim(email)));

CREATE TABLE applications (
  id bigserial PRIMARY KEY,
  student_id bigint NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  university_id bigint REFERENCES universities(id),
  custom_university_name text, custom_university_country text,
  programme_level text CHECK (programme_level IN ('masters','bachelors','top_up')),
  programme text, priority integer, intake text CHECK (intake IN ('autumn','spring')), year smallint,
  application_number text, submission_date date,
  status text CHECK (status IN ('applied','admitted','reserve','rejected','withdrawn','fees_paid','visa_granted','visa_rejected')),
  personal_code text, tuition_fee numeric(12,2), tuition_currency char(3), tuition_per text CHECK (tuition_per IN ('semester','year','programme')),
  scholarship text,
  created_by bigint NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((university_id IS NOT NULL) <> (custom_university_name IS NOT NULL))
);

CREATE TABLE activity_log (
  id bigserial PRIMARY KEY, student_id bigint REFERENCES students(id) ON DELETE CASCADE,
  actor_id bigint NOT NULL REFERENCES users(id), action text NOT NULL, detail jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (token text PRIMARY KEY, data bytea NOT NULL, expiry timestamptz NOT NULL);
```

Indexes for admin filters: `students(branch_id)`, `students(source)`, `students(consultancy_id)`, `applications(student_id)`, `applications(university_id)`, `applications(status)`, `applications(intake, year)`, `applications(created_at)`, `activity_log(student_id, created_at)`, plus a `pg_trgm` GIN index on student name + email for search.

Seed migration: the 5 branches. First admin is created with a CLI command (`server create-admin --email ...`), never by a default password.

## 5. Access control (the core rule)

```go
type Scope struct { Role Role; BranchID *int16; UserID int64 }
// Admin:         no filter
// Consultant:    students.source = 'internal' AND students.branch_id = scope.BranchID
// Agent network: students.source = 'b2b'
```

- Every student/application query takes a `Scope`; the WHERE clause is built in one function (`access.StudentFilter`).
- Single-record reads (`/students/{id}`) return **404** (not 403) when out of scope.
- Writes check scope on the parent student before touching applications.
- Admin-only routes behind `RequireRole(admin)` middleware.
- A dedicated test suite seeds students across branches/sources and asserts every role sees exactly what it should, for list, detail, edit, export and by guessing IDs.

## 6. Routes (server-rendered)

| Method & path | Who | What |
|---|---|---|
| GET/POST `/login`, POST `/logout` | all | Sign in/out |
| GET `/` | all | Redirect: admin → `/applications`, others → `/students` |
| GET `/students` | all | List with filters (query params); htmx swaps the table |
| GET/POST `/students/new` | consultant, agent network | New student + first application |
| GET `/students/{id}` | all (scoped) | Detail + applications + activity |
| GET/POST `/students/{id}/edit` | all (scoped) | Edit student |
| POST `/students/{id}/move` | admin | Move branch (internal only) |
| GET/POST `/students/{id}/applications/new` | all (scoped) | Add application |
| GET/POST `/applications/{id}/edit` | all (scoped) | Edit application |
| POST `/applications/{id}/status` | all (scoped) | Inline status change (htmx) |
| POST `/applications/{id}/delete` | all (scoped) | Delete with confirm |
| GET `/applications` | admin | All applications, full filters |
| GET `/applications/export.csv` | admin | Streams filtered CSV |
| GET `/catalog/universities/search?q=` | all | htmx results for the university box |
| GET/POST `/admin/catalog` | admin | Universities & consultancies |
| GET/POST `/admin/users`, POST `/admin/users/{id}/reset`, `/toggle` | admin | User management |
| GET `/healthz` | — | Health check for Railway |

## 7. Build order (milestones)

1. **Skeleton (day 1–2)**: repo, Makefile, config, Docker, Postgres via docker-compose for local dev, goose + sqlc wired, `/healthz`, layout page with the prototype CSS. Deploy a test copy to Railway (Hobby) to prove the pipeline.
2. **Auth (day 3–4)**: users table, `create-admin` command, login/logout, sessions, CSRF, rate limiting, role middleware.
3. **Access layer + students (day 5–8)**: Scope, student list with filters, detail, create (with email uniqueness + duplicate messages), edit; activity log; the access test suite.
4. **Applications (day 9–11)**: university search box with one-off "Other", add/edit/delete, inline status, after-admission fields with currency.
5. **Agent network flow (day 12)**: consultancy dropdown with "Other → add to list", B2B scoping.
6. **Admin tools (day 13–16)**: applications view with all filters (URL-driven), branch move, catalog management, user management (generate/reset password shown once, one-consultant-per-branch rule).
7. **Export (day 17)**: streaming CSV with the agreed columns.
8. **Hardening (day 18–20)**: security headers, error pages, input limits, logging, DB size on admin dashboard, backup job (nightly pg_dump → R2) and a restore drill, README/runbook.
9. **Go-live**: Railway Pro, custom domain (e.g. apply.snycosmos.com), PITR + scheduled backups on, create admin, admin creates users, short walkthrough with SNY Cosmos.

Day counts are rough for one developer working steadily; adjust after milestone 1.

## 8. Quality bar

- `go vet`, `staticcheck`, `golangci-lint` in CI (GitHub Actions); tests run against real Postgres.
- Every migration reversible; never edit an applied migration.
- No secrets in the repo; `.env.example` only.
- Structured logs (slog JSON) with request IDs; no student data in logs.

## 9. Open items

- Data residency: must student data stay in India? (Decides Railway Singapore vs an India region.)
- Optional later: password-reset by email, audit-log screen for admin, mobile API.
