#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

DATABASE_DSN="${MEGAVPN_DATABASE_DSN:-${MEGAVPN_RELEASE_DATABASE_DSN:-${MEGAVPN_TEST_DATABASE_DSN:-}}}"
RESTORE_DATABASE_DSN="${MEGAVPN_RELEASE_RESTORE_DATABASE_DSN:-${MEGAVPN_MIGRATION_DRILL_RESTORE_DATABASE_DSN:-}}"
ALLOW_EXISTING="${MEGAVPN_MIGRATION_DRILL_ALLOW_EXISTING:-0}"
RUN_BACKUP_RESTORE="${MEGAVPN_MIGRATION_DRILL_RUN_BACKUP_RESTORE:-auto}"

log() {
  printf '[postgres-migration-drill] %s\n' "$*"
}

die() {
  printf '[postgres-migration-drill] ERROR: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

is_enabled() {
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
    1|true|yes|y|on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

psql_value() {
  psql "$DATABASE_DSN" -XAt -v ON_ERROR_STOP=1 -c "$1"
}

sql_quote() {
  printf "%s" "$1" | sed "s/'/''/g"
}

[[ -n "$DATABASE_DSN" ]] || die "MEGAVPN_DATABASE_DSN, MEGAVPN_RELEASE_DATABASE_DSN or MEGAVPN_TEST_DATABASE_DSN is required"
require_command psql

critical_tables=(
  nodes
  node_agents
  instances
  instance_runtime_states
  instance_runtime_observations
  service_definitions
  client_accounts
  service_accesses
  client_service_identities
  client_access_routes
  artifacts
  share_links
  jobs
  job_logs
  resource_locks
  audit_events
  firewall_address_lists
  firewall_address_entries
  firewall_policies
  firewall_rules
  firewall_revisions
  firewall_node_state
  traffic_accounting_samples
)

table_sql=""
for table in "${critical_tables[@]}"; do
  if [[ -n "$table_sql" ]]; then
    table_sql+=","
  fi
  table_sql+="'$(sql_quote "$table")'"
done

schema_migrations_exists="$(psql_value "select to_regclass('schema_migrations') is not null;")"
schema_migration_count="0"
if [[ "$schema_migrations_exists" == "t" ]]; then
  schema_migration_count="$(psql_value "select count(*) from schema_migrations;")"
fi
existing_critical_tables="$(psql_value "select coalesce(string_agg(tablename, ', ' order by tablename), '') from pg_tables where schemaname=current_schema() and tablename in ($table_sql);")"
if [[ "$schema_migration_count" != "0" || -n "$existing_critical_tables" ]]; then
  if ! is_enabled "$ALLOW_EXISTING"; then
    die "database is not empty enough for zero-migration proof: schema_migrations=$schema_migration_count critical_tables=[$existing_critical_tables]. Use a disposable empty database or set MEGAVPN_MIGRATION_DRILL_ALLOW_EXISTING=1 for diagnostics only."
  fi
  log "existing schema accepted only because MEGAVPN_MIGRATION_DRILL_ALLOW_EXISTING=$ALLOW_EXISTING"
fi

migration_list="$(find migrations -maxdepth 1 -type f -name '*.up.sql' | sort)"
[[ -n "$migration_list" ]] || die "no migrations/*.up.sql files found"
expected_count="$(printf '%s\n' "$migration_list" | wc -l | tr -d '[:space:]')"
latest_version="$(basename "$(printf '%s\n' "$migration_list" | tail -n 1)" .up.sql)"

log "apply migrations from zero/disposable database"
MEGAVPN_DATABASE_DSN="$DATABASE_DSN" go run ./cmd/migrate

log "apply migrations again to prove idempotent runner skip behavior"
MEGAVPN_DATABASE_DSN="$DATABASE_DSN" go run ./cmd/migrate

actual_count="$(psql_value "select count(*) from schema_migrations;")"
if [[ "$actual_count" != "$expected_count" ]]; then
  die "schema_migrations count=$actual_count, want $expected_count"
fi
actual_latest="$(psql_value "select coalesce(max(version), '') from schema_migrations;")"
if [[ "$actual_latest" != "$latest_version" ]]; then
  die "latest schema_migrations version=$actual_latest, want $latest_version"
fi

while IFS= read -r file; do
  [[ -n "$file" ]] || continue
  version="$(basename "$file" .up.sql)"
  present="$(psql_value "select exists(select 1 from schema_migrations where version='$(sql_quote "$version")');")"
  if [[ "$present" != "t" ]]; then
    die "migration version was not recorded: $version"
  fi
done <<<"$migration_list"

for table in "${critical_tables[@]}"; do
  present="$(psql_value "select to_regclass('$(sql_quote "$table")') is not null;")"
  if [[ "$present" != "t" ]]; then
    die "required table missing after migrations: $table"
  fi
done

token_columns="$(psql_value "select count(*) from information_schema.columns where table_schema=current_schema() and table_name='share_links' and column_name in ('token_hash','token_hint');")"
if [[ "$token_columns" != "2" ]]; then
  die "share_links token_hash/token_hint columns are required; found $token_columns"
fi

firewall_apply_columns="$(psql_value "select count(*) from information_schema.columns where table_schema=current_schema() and table_name='firewall_node_state' and column_name in ('last_preview_json','last_error','last_job_id');")"
if [[ "$firewall_apply_columns" != "3" ]]; then
  die "firewall_node_state apply hardening columns are required; found $firewall_apply_columns"
fi

if [[ "$RUN_BACKUP_RESTORE" == "auto" ]]; then
  if [[ -n "$RESTORE_DATABASE_DSN" ]]; then
    RUN_BACKUP_RESTORE="1"
  else
    RUN_BACKUP_RESTORE="0"
  fi
fi

if is_enabled "$RUN_BACKUP_RESTORE"; then
  [[ -n "$RESTORE_DATABASE_DSN" ]] || die "MEGAVPN_RELEASE_RESTORE_DATABASE_DSN or MEGAVPN_MIGRATION_DRILL_RESTORE_DATABASE_DSN is required for backup/restore drill"
  require_command pg_dump
  require_command pg_restore
  backup_dir="$(mktemp -d)"
  artifact_root="$(mktemp -d)"
  log "run backup/restore drill after migration invariants"
  MEGAVPN_DATABASE_DSN="$DATABASE_DSN" \
    MEGAVPN_BACKUP_DIR="$backup_dir" \
    MEGAVPN_ARTIFACT_ROOT="$artifact_root" \
    scripts/ops/backup.sh
  archive="$(ls -t "$backup_dir"/megavpn-backup-*.tar.gz | head -n 1)"
  [[ -f "$archive" ]] || die "backup archive was not created"
  MEGAVPN_RESTORE_CONFIRM=1 \
    MEGAVPN_DATABASE_DSN="$RESTORE_DATABASE_DSN" \
    MEGAVPN_ARTIFACT_ROOT="$artifact_root.restore" \
    scripts/ops/restore.sh "$archive"
else
  log "backup/restore drill skipped; set MEGAVPN_RELEASE_RESTORE_DATABASE_DSN or MEGAVPN_MIGRATION_DRILL_RUN_BACKUP_RESTORE=1"
fi

log "migration drill completed: migrations=$actual_count latest=$actual_latest"
