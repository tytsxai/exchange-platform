#!/bin/bash
set -euo pipefail

DB_URL=${DB_URL:-""}
MIGRATIONS_DIR=${MIGRATIONS_DIR:-"migrations"}
INIT_SQL=${INIT_SQL:-"scripts/init-db.sql"}
APP_ENV=${APP_ENV:-"dev"}

if [ "$APP_ENV" != "dev" ] && [ -z "$DB_URL" ]; then
  echo "In non-dev environment, DB_URL is required for migrate.sh" >&2
  exit 1
fi

if [ "$DB_URL" = "" ]; then
  DB_URL="postgres://exchange:exchange123@localhost:5436/exchange?sslmode=disable"
fi

if [ "$APP_ENV" != "dev" ]; then
  if echo "$DB_URL" | grep -Eq 'sslmode=disable([&#]|$)'; then
    echo "In non-dev environment, DB_URL must not use sslmode=disable" >&2
    exit 1
  fi
  if echo "$DB_URL" | grep -Eq 'exchange123'; then
    echo "In non-dev environment, DB_URL must not use default password" >&2
    exit 1
  fi
fi

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
    # If the DB was initialized before schema_migrations existed (e.g. via docker-entrypoint-initdb.d),
    # record init-db.sql as applied to avoid re-applying non-idempotent DDL.
    already_inited=$(psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q -t -A -c \
      "SELECT 1 FROM information_schema.tables WHERE table_schema='exchange_user' AND table_name='users' LIMIT 1;")
    if [ -n "$already_inited" ]; then
      echo "${INIT_SQL} appears already applied; recording in schema_migrations..."
      psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q -c \
        "INSERT INTO public.schema_migrations (version) VALUES ('init-db.sql') ON CONFLICT DO NOTHING;"
    else
      echo "Applying ${INIT_SQL}..."
      psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q -f "$INIT_SQL"
      psql -X "$DB_URL" -v ON_ERROR_STOP=1 -q -c \
        "INSERT INTO public.schema_migrations (version) VALUES ('init-db.sql') ON CONFLICT DO NOTHING;"
    fi
  fi
fi

migration_candidates=()

if [ -d "$MIGRATIONS_DIR" ]; then
  while IFS= read -r file; do
    migration_candidates+=("$file")
  done < <(find "$MIGRATIONS_DIR" -maxdepth 1 -type f -name '*.sql' | sort)
fi

# Backward-compatible path:
# keep loading numbered SQL migrations from scripts/ (e.g. scripts/003_audit_logs.sql).
if [ -d "scripts" ]; then
  while IFS= read -r file; do
    migration_candidates+=("$file")
  done < <(find "scripts" -maxdepth 1 -type f -name '[0-9][0-9][0-9]_*.sql' | sort)
fi

if [ "${#migration_candidates[@]}" -gt 0 ]; then
  mapfile -t unique_migrations < <(printf '%s\n' "${migration_candidates[@]}" | sort -u)
  for file in "${unique_migrations[@]}"; do
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
