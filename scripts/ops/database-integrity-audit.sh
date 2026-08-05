#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

VALIDATE_CONSTRAINTS=0
case "${1:-}" in
  "") ;;
  --validate-constraints) VALIDATE_CONSTRAINTS=1 ;;
  *)
    printf 'usage: %s [--validate-constraints]\n' "$0" >&2
    exit 2
    ;;
esac

DATABASE_DSN="${MEGAVPN_DATABASE_DSN:-${MEGAVPN_RELEASE_DATABASE_DSN:-${MEGAVPN_TEST_DATABASE_DSN:-}}}"
if [[ -z "$DATABASE_DSN" ]]; then
  printf 'MEGAVPN_DATABASE_DSN, MEGAVPN_RELEASE_DATABASE_DSN or MEGAVPN_TEST_DATABASE_DSN is required\n' >&2
  exit 2
fi
command -v psql >/dev/null 2>&1 || {
  printf 'psql is required\n' >&2
  exit 2
}

read -r -d '' FINDINGS_SQL <<'SQL' || true
with findings(severity, name, count) as (
  values
    ('critical', 'missing current instance revision', (
      select count(*) from instances i
      left join instance_revisions ir on ir.id=i.current_revision_id
      where i.current_revision_id is not null and ir.id is null
    )),
    ('critical', 'current revision belongs to another instance', (
      select count(*) from instances i
      join instance_revisions ir on ir.id=i.current_revision_id
      where ir.instance_id <> i.id
    )),
    ('critical', 'missing last applied instance revision', (
      select count(*) from instances i
      left join instance_revisions ir on ir.id=i.last_applied_revision_id
      where i.last_applied_revision_id is not null and ir.id is null
    )),
    ('critical', 'last applied revision belongs to another instance', (
      select count(*) from instances i
      join instance_revisions ir on ir.id=i.last_applied_revision_id
      where ir.instance_id <> i.id
    )),
    ('critical', 'audit actor references missing platform user', (
      select count(*) from audit_events ae
      left join platform_users pu on pu.id=ae.actor_user_id
      where ae.actor_user_id is not null and pu.id is null
    )),
    ('critical', 'access-group membership service differs from group service', (
      select count(*) from client_access_group_memberships m
      join client_access_groups g on g.id=m.group_id
      where m.service_code <> g.service_code
    )),
    ('critical', 'client service identity references unknown runtime service', (
      select count(*) from client_service_identities identity
      left join service_definitions definition on definition.code=identity.service_code
      where definition.code is null
    )),
    ('critical', 'superseded VLESS registries still installed', (
      select count(*)
      from (values
        ('vless_group_templates'),
        ('vless_group_memberships'),
        ('client_access_group_migration_conflicts')
      ) legacy(name)
      where to_regclass(legacy.name) is not null
    )),
    ('critical', 'normalized relationship tables missing', (
      select count(*)
      from (values
        ('client_email_delivery_artifacts'),
        ('client_email_delivery_share_links'),
        ('backhaul_transport_secrets')
      ) required(name)
      where to_regclass(required.name) is null
    )),
    ('critical', 'legacy relationship JSON columns still installed', (
      select count(*)
      from information_schema.columns
      where table_schema=current_schema()
        and (
          (table_name='client_email_deliveries' and column_name in ('artifact_ids','share_link_ids'))
          or (table_name='backhaul_transports' and column_name='secret_refs_json')
        )
    )),
    ('critical', 'required normalization triggers missing', (
      select 2 - count(*)
      from information_schema.triggers
      where trigger_schema=current_schema()
        and trigger_name in ('service_definitions_updated_at_guard','nodes_preserve_history_guard')
    )),
    ('warning', 'installed integrity constraints awaiting validation', (
      select count(*) from pg_constraint
      where not convalidated
        and (conrelid, conname) in (
          ('instances'::regclass, 'instances_current_revision_fk'),
          ('instances'::regclass, 'instances_last_applied_revision_fk'),
          ('audit_events'::regclass, 'audit_events_actor_user_fk'),
          ('client_access_group_memberships'::regclass, 'client_access_group_memberships_group_service_fk'),
          ('client_service_identities'::regclass, 'client_service_identities_service_code_fk')
        )
    ))
)
select
  case when count=0 then 'ok' when severity='critical' then 'FAIL' else 'review' end as status,
  severity,
  name,
  count
from findings
order by case severity when 'critical' then 0 else 1 end, count desc, name;
SQL

printf '[database-integrity-audit] database=%s\n' \
  "$(psql "$DATABASE_DSN" -XAt -v ON_ERROR_STOP=1 -c 'select current_database();')"
psql "$DATABASE_DSN" -X -v ON_ERROR_STOP=1 -c "$FINDINGS_SQL"

critical_count="$(psql "$DATABASE_DSN" -XAt -v ON_ERROR_STOP=1 <<'SQL'
select
  (select count(*) from instances i left join instance_revisions ir on ir.id=i.current_revision_id where i.current_revision_id is not null and ir.id is null) +
  (select count(*) from instances i join instance_revisions ir on ir.id=i.current_revision_id where ir.instance_id <> i.id) +
  (select count(*) from instances i left join instance_revisions ir on ir.id=i.last_applied_revision_id where i.last_applied_revision_id is not null and ir.id is null) +
  (select count(*) from instances i join instance_revisions ir on ir.id=i.last_applied_revision_id where ir.instance_id <> i.id) +
  (select count(*) from audit_events ae left join platform_users pu on pu.id=ae.actor_user_id where ae.actor_user_id is not null and pu.id is null) +
  (select count(*) from client_access_group_memberships m join client_access_groups g on g.id=m.group_id where m.service_code <> g.service_code) +
  (select count(*) from client_service_identities identity left join service_definitions definition on definition.code=identity.service_code where definition.code is null) +
  (select count(*) from (values ('vless_group_templates'), ('vless_group_memberships'), ('client_access_group_migration_conflicts')) legacy(name) where to_regclass(legacy.name) is not null) +
  (select count(*) from (values ('client_email_delivery_artifacts'), ('client_email_delivery_share_links'), ('backhaul_transport_secrets')) required(name) where to_regclass(required.name) is null) +
  (select count(*) from information_schema.columns where table_schema=current_schema() and ((table_name='client_email_deliveries' and column_name in ('artifact_ids','share_link_ids')) or (table_name='backhaul_transports' and column_name='secret_refs_json'))) +
  (select 2 - count(*) from information_schema.triggers where trigger_schema=current_schema() and trigger_name in ('service_definitions_updated_at_guard','nodes_preserve_history_guard'));
SQL
)"

if [[ "$critical_count" != "0" ]]; then
  printf '[database-integrity-audit] FAIL: %s critical rows require an explicit repair\n' "$critical_count" >&2
  exit 1
fi

if [[ "$VALIDATE_CONSTRAINTS" == "1" ]]; then
  installed_constraints="$(psql "$DATABASE_DSN" -XAt -v ON_ERROR_STOP=1 -c "select count(*) from pg_constraint where (conrelid, conname) in (('instances'::regclass, 'instances_current_revision_fk'), ('instances'::regclass, 'instances_last_applied_revision_fk'), ('audit_events'::regclass, 'audit_events_actor_user_fk'), ('client_access_group_memberships'::regclass, 'client_access_group_memberships_group_service_fk'), ('client_service_identities'::regclass, 'client_service_identities_service_code_fk'));")"
  if [[ "$installed_constraints" != "5" ]]; then
    printf '[database-integrity-audit] FAIL: integrity migration is incomplete; found %s of 5 constraints\n' "$installed_constraints" >&2
    exit 1
  fi
  psql "$DATABASE_DSN" -X -v ON_ERROR_STOP=1 <<'SQL'
alter table instances validate constraint instances_current_revision_fk;
alter table instances validate constraint instances_last_applied_revision_fk;
alter table audit_events validate constraint audit_events_actor_user_fk;
alter table client_access_group_memberships validate constraint client_access_group_memberships_group_service_fk;
alter table client_service_identities validate constraint client_service_identities_service_code_fk;
SQL
  printf '[database-integrity-audit] integrity constraints validated\n'
fi

printf '[database-integrity-audit] critical integrity checks passed\n'
