#!/usr/bin/env bash
# One-command local setup for the SNY Cosmos Application Tracker.
#
#   ./scripts/dev.sh          start everything (first run: creates the DB and demo data)
#   ./scripts/dev.sh stop     stop the local database
#   ./scripts/dev.sh reset    delete the local database and demo data, then start fresh
#
# Needs Go 1.26+ and EITHER Docker OR a local PostgreSQL (14+) install.
# Everything it creates lives in .dev/ (gitignored). Works on macOS and Linux
# (on Windows, run it inside WSL).
#
# Optional env: PORT (app), DB_PORT (database). If unset, the first free port
# from 8080 / 5433 upwards is used and remembered in .dev/ for later runs.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT=$(pwd)
DEV="$ROOT/.dev"
PGDATA_DIR="$DEV/pgdata"
mkdir -p "$DEV"

in_use() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

# pick_port NAME DEFAULT: explicit env wins; else the remembered port; else the first free one.
pick_port() {
  local name=$1 start=$2 file="$DEV/$1" p
  if [ -n "${!name:-}" ]; then echo "${!name}"; return; fi
  if [ -f "$file" ]; then cat "$file"; return; fi
  for p in $(seq "$start" $((start + 20))); do
    in_use "$p" || { echo "$p" > "$file"; echo "$p"; return; }
  done
  echo "$start"
}

say()  { printf '\033[1;35m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mxx\033[0m %s\n' "$*" >&2; exit 1; }

# ---------- prerequisites ----------
check_go() {
  command -v go >/dev/null 2>&1 || die "Go is not installed. Install Go 1.26 or newer: https://go.dev/dl/ (macOS: brew install go)"
  local v major minor
  v=$(go env GOVERSION | sed 's/^go//')
  major=${v%%.*}; minor=$(echo "$v" | cut -d. -f2)
  if [ "$major" -lt 1 ] || { [ "$major" -eq 1 ] && [ "$minor" -lt 26 ]; }; then
    die "Go $v is too old; need 1.26 or newer (https://go.dev/dl/)."
  fi
}

have_docker() { command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; }

# Find PostgreSQL server binaries (PATH, Homebrew, or Debian/Ubuntu layout).
find_pg_bin() {
  if command -v pg_ctl >/dev/null 2>&1 && command -v initdb >/dev/null 2>&1; then
    dirname "$(command -v pg_ctl)"; return
  fi
  if command -v brew >/dev/null 2>&1; then
    for f in postgresql@17 postgresql@16 postgresql@15 postgresql@14 postgresql; do
      local p; p="$(brew --prefix "$f" 2>/dev/null)/bin"
      [ -x "$p/pg_ctl" ] && { echo "$p"; return; }
    done
  fi
  local d
  for d in /usr/lib/postgresql/*/bin; do
    [ -x "$d/pg_ctl" ] && { echo "$d"; return; }
  done
  return 1
}

# ---------- database ----------
DB_MODE_FILE="$DEV/db-mode"

start_db_docker() {
  say "Starting PostgreSQL 16 in Docker on port $DB_PORT"
  DB_PORT="$DB_PORT" docker compose up -d --wait db >/dev/null
  echo docker > "$DB_MODE_FILE"
}

start_db_native() {
  local bin="$1"
  # LC_ALL=C avoids macOS "postmaster became multithreaded" locale errors.
  export LC_ALL=C
  if [ ! -d "$PGDATA_DIR" ]; then
    say "Creating a local database in .dev/pgdata (using $bin)"
    "$bin/initdb" -D "$PGDATA_DIR" -U postgres --auth=trust -E UTF8 --locale=C >/dev/null
  fi
  if ! "$bin/pg_ctl" -D "$PGDATA_DIR" status >/dev/null 2>&1; then
    say "Starting PostgreSQL on port $DB_PORT"
    "$bin/pg_ctl" -D "$PGDATA_DIR" -l "$DEV/postgres.log" -w \
      -o "-p $DB_PORT -k /tmp -c listen_addresses=localhost" start >/dev/null \
      || die "PostgreSQL didn't start. See .dev/postgres.log (is port $DB_PORT in use? try DB_PORT=5434 $0)"
  fi
  local psql="$bin/psql -h localhost -p $DB_PORT -U postgres -v ON_ERROR_STOP=1 -qtA"
  if [ "$($psql -c "SELECT 1 FROM pg_roles WHERE rolname = 'sny'")" != "1" ]; then
    $psql -c "CREATE ROLE sny LOGIN PASSWORD 'sny' CREATEDB" >/dev/null
  fi
  if [ "$($psql -c "SELECT 1 FROM pg_database WHERE datname = 'sny_tracker'")" != "1" ]; then
    $psql -c "CREATE DATABASE sny_tracker OWNER sny" >/dev/null
  fi
  echo "native:$bin" > "$DB_MODE_FILE"
}

start_db() {
  mkdir -p "$DEV"
  # Reuse whatever this checkout used before.
  if [ -f "$DB_MODE_FILE" ]; then
    case "$(cat "$DB_MODE_FILE")" in
      docker) have_docker && { start_db_docker; return; } ;;
      native:*) local b; b=$(cut -d: -f2- < "$DB_MODE_FILE"); [ -x "$b/pg_ctl" ] && { start_db_native "$b"; return; } ;;
    esac
  fi
  if have_docker; then start_db_docker; return; fi
  local bin
  if bin=$(find_pg_bin); then start_db_native "$bin"; return; fi
  die "No database available. Install ONE of:
     - Docker Desktop: https://www.docker.com/products/docker-desktop/  (then start it)
     - PostgreSQL:     macOS: brew install postgresql@16
                       Ubuntu/Debian: sudo apt install postgresql
   and run this script again."
}

stop_db() {
  [ -f "$DB_MODE_FILE" ] || { say "Nothing to stop."; return; }
  case "$(cat "$DB_MODE_FILE")" in
    docker) DB_PORT="$DB_PORT" docker compose stop db >/dev/null && say "Stopped the Docker database." ;;
    native:*)
      local b; b=$(cut -d: -f2- < "$DB_MODE_FILE")
      LC_ALL=C "$b/pg_ctl" -D "$PGDATA_DIR" stop -m fast >/dev/null 2>&1 && say "Stopped the local database." || say "Database was not running." ;;
  esac
}

reset_all() {
  stop_db || true
  if [ -f "$DB_MODE_FILE" ] && [ "$(cat "$DB_MODE_FILE")" = docker ]; then
    DB_PORT="$DB_PORT" docker compose down -v >/dev/null
  fi
  rm -rf "$DEV"
  mkdir -p "$DEV"
  say "Local data removed."
}

# ---------- main ----------
DB_PORT=$(pick_port DB_PORT 5433)
DB_URL="postgres://sny:sny@localhost:$DB_PORT/sny_tracker?sslmode=disable"

case "${1:-up}" in
  stop)  stop_db; exit 0 ;;
  reset) reset_all ;;
  up|"") ;;
  *) die "usage: $0 [up|stop|reset]" ;;
esac

check_go
start_db

PORT=$(pick_port PORT 8080)
if in_use "$PORT"; then
  die "Port $PORT is already in use by another program. Run: PORT=8081 $0"
fi

# Keep .env in sync for 'make run' (only if this script wrote it).
if [ ! -f .env ] || grep -q '^# written by scripts/dev.sh' .env; then
  printf '# written by scripts/dev.sh\nDATABASE_URL=%s\nPORT=%s\nAPP_ENV=development\n' "$DB_URL" "$PORT" > .env
fi

say "Building the server"
go build -o "$DEV/server" ./cmd/server

export DATABASE_URL="$DB_URL" PORT="$PORT" APP_ENV=development
say "Applying database migrations"
"$DEV/server" migrate >/dev/null

if [ ! -f "$DEV/demo-users.txt" ]; then
  say "Loading demo data"
  if "$DEV/server" seed-demo 2>/dev/null | sed -n '/Demo data created/,$p' > "$DEV/demo-users.txt.tmp" && [ -s "$DEV/demo-users.txt.tmp" ]; then
    mv "$DEV/demo-users.txt.tmp" "$DEV/demo-users.txt"
  else
    rm -f "$DEV/demo-users.txt.tmp"
    warn "Database already has data, so no demo data was added."
  fi
fi

echo
if [ -f "$DEV/demo-users.txt" ]; then
  cat "$DEV/demo-users.txt"
  echo "  (saved in .dev/demo-users.txt)"
fi
say "App running at http://localhost:$PORT   (Ctrl+C to stop; './scripts/dev.sh stop' stops the database)"
echo
exec "$DEV/server"
