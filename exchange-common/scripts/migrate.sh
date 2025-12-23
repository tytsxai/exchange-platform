#!/bin/bash
set -euo pipefail

DB_URL=${DB_URL:-"postgres://exchange:exchange123@localhost:5436/exchange?sslmode=disable"}
MIGRATIONS_DIR=${MIGRATIONS_DIR:-"migrations"}
INIT_SQL=${INIT_SQL:-"scripts/init-db.sql"}

require_cmd() {
  local cmd=$1
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

require_cmd psql

psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q <<'SQL'
CREATE TABLE IF NOT EXISTS public.schema_migrations (
  version text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);
SQL

if [ -f "$INIT_SQL" ]; then
  applied=$(psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q -t -A -c \
    "SELECT 1 FROM public.schema_migrations WHERE version = 'init-db.sql' LIMIT 1;")
  if [ -z "$applied" ]; then
    echo "Applying ${INIT_SQL}..."
    psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q -f "$INIT_SQL"
    psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q -c \
      "INSERT INTO public.schema_migrations (version) VALUES ('init-db.sql');"
  fi
fi

if [ -d "$MIGRATIONS_DIR" ]; then
  for file in "$MIGRATIONS_DIR"/*.sql; do
    [ -e "$file" ] || continue
    version=$(basename "$file")
    applied=$(psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q -t -A -c \
      "SELECT 1 FROM public.schema_migrations WHERE version = '${version}' LIMIT 1;")
    if [ -n "$applied" ]; then
      continue
    fi
    echo "Applying migration ${version}..."
    psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q -f "$file"
    psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q -c \
      "INSERT INTO public.schema_migrations (version) VALUES ('${version}');"
  done
fi

echo "Migrations complete."
