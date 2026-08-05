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
  node_enrollment_tokens
  node_inventory_snapshots
  node_capabilities
  node_service_discoveries
  node_service_discovery_events
  node_capability_install_events
  node_access_methods
  node_bootstrap_runs
  instances
  instance_revisions
  instance_runtime_states
  instance_runtime_observations
  service_definitions
  service_pack_templates
  client_accounts
  service_accesses
  client_service_identities
  client_subscriptions
  client_access_routes
  artifacts
  share_links
  jobs
  job_logs
  resource_locks
  audit_events
  platform_users
  roles
  permissions
  role_permissions
  platform_user_roles
  user_sessions
  platform_mail_settings
  platform_user_invites
  client_email_deliveries
  client_email_delivery_artifacts
  client_email_delivery_share_links
  secret_refs
  agent_trust_roots
  node_agent_certificates
  platform_service_pki_roots
  platform_certificates
  platform_control_plane_tls_settings
  backhaul_links
  backhaul_transports
  backhaul_transport_secrets
  address_pool_spaces
  address_pool_allocations
  binary_artifacts
  binary_manifests
  binary_download_tickets
  client_access_groups
  client_access_group_memberships
  client_access_group_instance_scopes
  client_access_group_sync_state
  firewall_address_lists
  firewall_address_entries
  firewall_policies
  firewall_rules
  firewall_revisions
  firewall_node_state
  traffic_accounting_samples
  external_egress_profiles
  external_egress_profile_secrets
  external_egress_deployments
)

critical_indexes=(
  idx_jobs_status_priority
  idx_jobs_locked_until
  idx_resource_locks_job
  idx_job_logs_job_created
  idx_share_links_token_hash
  idx_client_service_identities_lookup
  client_subscriptions_token_hash_idx
  idx_client_subscriptions_client_status
  idx_client_access_routes_baseline_access
  idx_instance_runtime_states_node_health
  idx_instance_runtime_observations_instance_time
  idx_traffic_accounting_node_sample_key
  idx_traffic_accounting_client_export
  idx_binary_artifacts_unique_active
  idx_binary_download_tickets_artifact
  idx_service_accesses_instance_status
  idx_service_accesses_instance_vless_group
  idx_client_accounts_lookup_lower
  idx_client_access_groups_service_key_active
  idx_client_access_groups_service_status
  idx_client_access_group_memberships_one_active_per_service
  idx_client_access_group_memberships_group_status
  idx_client_access_group_memberships_client_status
  idx_client_access_group_instance_scopes_instance
  idx_client_access_groups_id_service
  idx_instances_current_revision
  idx_instances_last_applied_revision
  idx_audit_events_actor_user
  idx_client_email_deliveries_id_client
  idx_artifacts_id_client
  idx_share_links_id_client
  idx_client_email_delivery_artifacts_artifact
  idx_client_email_delivery_share_links_link
  idx_backhaul_transport_secrets_ref
  firewall_address_entries_active_value_idx
  firewall_policies_node_idx
  firewall_rules_policy_priority_idx
  external_egress_profiles_status_protocol_idx
  external_egress_deployments_node_status_idx
  client_access_groups_external_egress_idx
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

for index in "${critical_indexes[@]}"; do
  present="$(psql_value "select to_regclass('$(sql_quote "$index")') is not null;")"
  if [[ "$present" != "t" ]]; then
    die "required index missing after migrations: $index"
  fi
done

token_columns="$(psql_value "select count(*) from information_schema.columns where table_schema=current_schema() and table_name='share_links' and column_name in ('token_hash','token_hint');")"
if [[ "$token_columns" != "2" ]]; then
  die "share_links token_hash/token_hint columns are required; found $token_columns"
fi

plaintext_share_token_columns="$(psql_value "select count(*) from information_schema.columns where table_schema=current_schema() and table_name='share_links' and column_name='token';")"
if [[ "$plaintext_share_token_columns" != "0" ]]; then
  die "share_links must not contain a plaintext token column"
fi

firewall_apply_columns="$(psql_value "select count(*) from information_schema.columns where table_schema=current_schema() and table_name='firewall_node_state' and column_name in ('last_preview_json','last_error','last_job_id');")"
if [[ "$firewall_apply_columns" != "3" ]]; then
  die "firewall_node_state apply hardening columns are required; found $firewall_apply_columns"
fi

firewall_node_state_status_constraint="$(psql_value "select count(*) from pg_constraint c join pg_class t on t.oid = c.conrelid join pg_namespace n on n.oid = t.relnamespace where n.nspname = current_schema() and t.relname = 'firewall_node_state' and c.conname = 'firewall_node_state_status_check' and pg_get_constraintdef(c.oid) like '%pending_disable%' and pg_get_constraintdef(c.oid) like '%disabled%' and pg_get_constraintdef(c.oid) like '%stale%';")"
if [[ "$firewall_node_state_status_constraint" != "1" ]]; then
  die "firewall_node_state status constraint must allow pending_disable/disabled/stale"
fi

vless_access_groups="$(psql_value "select count(*) from client_access_groups where service_code='vless' and status='active' and deleted_at is null;")"
if [[ "$vless_access_groups" -lt 1 ]]; then
  die "active canonical VLESS access groups are required after migrations"
fi

legacy_vless_registries="$(psql_value "select count(*) from (values ('vless_group_templates'), ('vless_group_memberships'), ('client_access_group_migration_conflicts')) legacy(name) where to_regclass(name) is not null;")"
if [[ "$legacy_vless_registries" != "0" ]]; then
  die "superseded VLESS registries must be absent after migrations; found $legacy_vless_registries"
fi

firewall_seed_groups="$(psql_value "select count(*) from firewall_address_lists where key in ('trusted_operators','vpn_client_sources','backhaul_sources','public_service_sources','blocked_destinations') and status <> 'deleted';")"
if [[ "$firewall_seed_groups" -lt 5 ]]; then
  die "firewall semantic/default address groups missing after migrations; found $firewall_seed_groups"
fi

external_egress_columns="$(psql_value "select count(*) from information_schema.columns where table_schema=current_schema() and table_name='external_egress_deployments' and column_name in ('interface_name','routing_table','fwmark','route_metric','health_json');")"
if [[ "$external_egress_columns" != "5" ]]; then
  die "external_egress_deployments isolation columns are required; found $external_egress_columns"
fi

plaintext_external_secret_columns="$(psql_value "select count(*) from information_schema.columns where table_schema=current_schema() and table_name='external_egress_profiles' and column_name in ('password','private_key','preshared_key','config');")"
if [[ "$plaintext_external_secret_columns" != "0" ]]; then
  die "external_egress_profiles must not contain plaintext provider secret columns"
fi

external_egress_guard_triggers="$(psql_value "select count(*) from information_schema.triggers where trigger_schema=current_schema() and trigger_name in ('client_access_groups_external_egress_guard','external_egress_deployments_profile_guard','external_egress_profiles_lifecycle_guard');")"
if [[ "$external_egress_guard_triggers" != "3" ]]; then
  die "external egress lifecycle guard triggers are required; found $external_egress_guard_triggers"
fi

integrity_constraints="$(psql_value "select count(*)
  from pg_constraint
  where convalidated
    and (conrelid, conname) in (
      ('instances'::regclass, 'instances_current_revision_fk'),
      ('instances'::regclass, 'instances_last_applied_revision_fk'),
      ('audit_events'::regclass, 'audit_events_actor_user_fk'),
      ('client_access_group_memberships'::regclass, 'client_access_group_memberships_group_service_fk'),
      ('client_service_identities'::regclass, 'client_service_identities_service_code_fk')
    );")"
if [[ "$integrity_constraints" != "5" ]]; then
  die "database integrity constraints are missing or not validated on a fresh schema; found $integrity_constraints of 5"
fi

revision_ownership_guards="$(psql_value "select count(*) from information_schema.triggers where trigger_schema=current_schema() and trigger_name in ('instances_revision_ownership_guard','instance_revisions_owner_immutable_guard');")"
if [[ "$revision_ownership_guards" != "2" ]]; then
  die "instance revision ownership triggers are required; found $revision_ownership_guards of 2"
fi

access_group_service_mismatches="$(psql_value "select count(*) from client_access_group_memberships m join client_access_groups g on g.id=m.group_id where m.service_code <> g.service_code;")"
if [[ "$access_group_service_mismatches" != "0" ]]; then
  die "client access group membership service mismatches found: $access_group_service_mismatches"
fi

access_group_service_constraints="$(psql_value "select count(*) from pg_constraint where conrelid='client_access_groups'::regclass and conname='client_access_groups_service_code_check';")"
membership_service_check="$(psql_value "select count(*) from pg_constraint where conrelid='client_access_group_memberships'::regclass and conname='client_access_group_memberships_service_code_check';")"
if [[ "$access_group_service_constraints" != "1" || "$membership_service_check" != "0" ]]; then
  die "logical access-service validation must live on client_access_groups and membership must inherit it through the composite FK"
fi

access_group_service_constraint_shape="$(psql_value "select count(*) from pg_constraint where conrelid='client_access_groups'::regclass and conname='client_access_groups_service_code_check' and pg_get_constraintdef(oid) like '%service_code ~%';")"
if [[ "$access_group_service_constraint_shape" != "1" ]]; then
  die "client_access_groups service_code must use identifier validation instead of a stale protocol enum"
fi

revision_reference_violations="$(psql_value "select
  (select count(*) from instances i left join instance_revisions ir on ir.id=i.current_revision_id where i.current_revision_id is not null and ir.id is null) +
  (select count(*) from instances i join instance_revisions ir on ir.id=i.current_revision_id where ir.instance_id <> i.id) +
  (select count(*) from instances i left join instance_revisions ir on ir.id=i.last_applied_revision_id where i.last_applied_revision_id is not null and ir.id is null) +
  (select count(*) from instances i join instance_revisions ir on ir.id=i.last_applied_revision_id where ir.instance_id <> i.id);")"
if [[ "$revision_reference_violations" != "0" ]]; then
  die "instance revision reference violations found: $revision_reference_violations"
fi

orphan_audit_actors="$(psql_value "select count(*) from audit_events ae left join platform_users pu on pu.id=ae.actor_user_id where ae.actor_user_id is not null and pu.id is null;")"
if [[ "$orphan_audit_actors" != "0" ]]; then
  die "audit events with missing actor users found: $orphan_audit_actors"
fi

legacy_relationship_json_columns="$(psql_value "select count(*) from information_schema.columns where table_schema=current_schema() and ((table_name='client_email_deliveries' and column_name in ('artifact_ids','share_link_ids')) or (table_name='backhaul_transports' and column_name='secret_refs_json'));")"
if [[ "$legacy_relationship_json_columns" != "0" ]]; then
  die "stable email/backhaul relationships must not remain in JSON columns; found $legacy_relationship_json_columns"
fi

service_definition_updated_at="$(psql_value "select count(*) from information_schema.columns where table_schema=current_schema() and table_name='service_definitions' and column_name='updated_at' and data_type='timestamp with time zone';")"
if [[ "$service_definition_updated_at" != "1" ]]; then
  die "service_definitions.updated_at is required"
fi

normalization_triggers="$(psql_value "select count(*) from information_schema.triggers where trigger_schema=current_schema() and trigger_name in ('service_definitions_updated_at_guard','nodes_preserve_history_guard');")"
if [[ "$normalization_triggers" != "2" ]]; then
  die "schema normalization triggers are required; found $normalization_triggers of 2"
fi

unknown_identity_service_codes="$(psql_value "select count(*) from client_service_identities identity left join service_definitions definition on definition.code=identity.service_code where definition.code is null;")"
if [[ "$unknown_identity_service_codes" != "0" ]]; then
  die "client service identities reference unknown runtime services: $unknown_identity_service_codes"
fi

normalization_audit="$(psql_value "select count(*) from audit_events where action='migration.schema_normalization' and payload_json ? 'client_service_identity_unknown_code_sample' and payload_json ? 'email_delivery_unresolved_artifact_sample' and payload_json ? 'email_delivery_unresolved_share_link_sample' and payload_json ? 'backhaul_unresolved_secret_ref_sample';")"
if [[ "$normalization_audit" != "1" ]]; then
  die "schema normalization audit evidence is missing or incomplete"
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
