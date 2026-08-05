-- Database integrity hardening.
-- NOT VALID constraints protect every new write immediately and permit a safe
-- rollout on installations that need an explicit repair of historical rows.
-- Clean installations validate every constraint in this migration.

create index if not exists idx_instances_current_revision
  on instances(current_revision_id)
  where current_revision_id is not null;

create index if not exists idx_instances_last_applied_revision
  on instances(last_applied_revision_id)
  where last_applied_revision_id is not null;

create index if not exists idx_audit_events_actor_user
  on audit_events(actor_user_id)
  where actor_user_id is not null;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conrelid = 'instances'::regclass
      and conname = 'instances_current_revision_fk'
  ) then
    alter table instances
      add constraint instances_current_revision_fk
      foreign key(current_revision_id)
      references instance_revisions(id)
      on delete set null
      not valid;
  end if;

  if not exists (
    select 1 from pg_constraint
    where conrelid = 'instances'::regclass
      and conname = 'instances_last_applied_revision_fk'
  ) then
    alter table instances
      add constraint instances_last_applied_revision_fk
      foreign key(last_applied_revision_id)
      references instance_revisions(id)
      on delete set null
      not valid;
  end if;

  if not exists (
    select 1 from pg_constraint
    where conrelid = 'audit_events'::regclass
      and conname = 'audit_events_actor_user_fk'
  ) then
    alter table audit_events
      add constraint audit_events_actor_user_fk
      foreign key(actor_user_id)
      references platform_users(id)
      on delete set null
      not valid;
  end if;
end
$$;

create or replace function validate_instance_revision_ownership()
returns trigger
language plpgsql
as $$
begin
  if new.current_revision_id is not null and not exists (
    select 1 from instance_revisions ir
    where ir.id = new.current_revision_id
      and ir.instance_id = new.id
  ) then
    raise exception 'current revision % does not belong to instance %', new.current_revision_id, new.id
      using errcode = '23503';
  end if;

  if new.last_applied_revision_id is not null and not exists (
    select 1 from instance_revisions ir
    where ir.id = new.last_applied_revision_id
      and ir.instance_id = new.id
  ) then
    raise exception 'last applied revision % does not belong to instance %', new.last_applied_revision_id, new.id
      using errcode = '23503';
  end if;

  return new;
end
$$;

drop trigger if exists instances_revision_ownership_guard on instances;
create trigger instances_revision_ownership_guard
before insert or update of id, current_revision_id, last_applied_revision_id
on instances
for each row
execute function validate_instance_revision_ownership();

create or replace function prevent_instance_revision_reparenting()
returns trigger
language plpgsql
as $$
begin
  if new.instance_id is distinct from old.instance_id then
    raise exception 'instance revision % cannot be moved from instance % to instance %', old.id, old.instance_id, new.instance_id
      using errcode = '23503';
  end if;
  return new;
end
$$;

drop trigger if exists instance_revisions_owner_immutable_guard on instance_revisions;
create trigger instance_revisions_owner_immutable_guard
before update of instance_id
on instance_revisions
for each row
execute function prevent_instance_revision_reparenting();

alter table client_access_group_memberships
  drop constraint if exists client_access_group_memberships_service_code_check;

create unique index if not exists idx_client_access_groups_id_service
  on client_access_groups(id, service_code);

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conrelid = 'client_access_group_memberships'::regclass
      and conname = 'client_access_group_memberships_group_service_fk'
  ) then
    alter table client_access_group_memberships
      add constraint client_access_group_memberships_group_service_fk
      foreign key(group_id, service_code)
      references client_access_groups(id, service_code)
      on delete restrict
      not valid;
  end if;
end
$$;

comment on column client_access_groups.service_code is
  'Logical client access protocol (for example vless or l2tp), not a runtime service_definitions.code.';

comment on column client_access_group_memberships.service_code is
  'Logical client access protocol copied from the owning group and protected by a composite foreign key; allowed values are constrained on the owning group.';

comment on table vless_group_memberships is
  'Legacy read-only VLESS membership history retained for one rolling-upgrade window. client_access_group_memberships is canonical and receives all current writes.';

do $$
begin
  if not exists (
    select 1
    from instances i
    left join instance_revisions ir on ir.id = i.current_revision_id
    where i.current_revision_id is not null
      and ir.id is null
  ) then
    alter table instances validate constraint instances_current_revision_fk;
  end if;

  if not exists (
    select 1
    from instances i
    left join instance_revisions ir on ir.id = i.last_applied_revision_id
    where i.last_applied_revision_id is not null
      and ir.id is null
  ) then
    alter table instances validate constraint instances_last_applied_revision_fk;
  end if;

  if not exists (
    select 1
    from audit_events ae
    left join platform_users pu on pu.id = ae.actor_user_id
    where ae.actor_user_id is not null
      and pu.id is null
  ) then
    alter table audit_events validate constraint audit_events_actor_user_fk;
  end if;

  if not exists (
    select 1
    from client_access_group_memberships m
    join client_access_groups g on g.id = m.group_id
    where m.service_code <> g.service_code
  ) then
    alter table client_access_group_memberships
      validate constraint client_access_group_memberships_group_service_fk;
  end if;
end
$$;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values(
  gen_random_uuid(),
  'system',
  'migration.database_integrity_hardening',
  'platform',
  'database reference and access-group integrity guards installed',
  jsonb_build_object(
    'orphan_current_revisions', (
      select count(*)
      from instances i
      left join instance_revisions ir on ir.id=i.current_revision_id
      where i.current_revision_id is not null and ir.id is null
    ),
    'foreign_current_revisions', (
      select count(*)
      from instances i
      join instance_revisions ir on ir.id=i.current_revision_id
      where ir.instance_id <> i.id
    ),
    'orphan_last_applied_revisions', (
      select count(*)
      from instances i
      left join instance_revisions ir on ir.id=i.last_applied_revision_id
      where i.last_applied_revision_id is not null and ir.id is null
    ),
    'foreign_last_applied_revisions', (
      select count(*)
      from instances i
      join instance_revisions ir on ir.id=i.last_applied_revision_id
      where ir.instance_id <> i.id
    ),
    'orphan_audit_actors', (
      select count(*)
      from audit_events ae
      left join platform_users pu on pu.id=ae.actor_user_id
      where ae.actor_user_id is not null and pu.id is null
    ),
    'access_group_service_mismatches', (
      select count(*)
      from client_access_group_memberships m
      join client_access_groups g on g.id=m.group_id
      where m.service_code <> g.service_code
    )
  ),
  now()
)
on conflict do nothing;
