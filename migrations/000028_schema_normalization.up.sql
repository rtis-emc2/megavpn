-- Normalize stable relationships that previously lived in JSONB and protect
-- historical node data from accidental physical deletion.

alter table service_definitions
  add column if not exists updated_at timestamptz not null default now();

create or replace function touch_service_definitions_updated_at()
returns trigger
language plpgsql
as $$
begin
  new.updated_at = now();
  return new;
end
$$;

drop trigger if exists service_definitions_updated_at_guard on service_definitions;
create trigger service_definitions_updated_at_guard
before update on service_definitions
for each row
execute function touch_service_definitions_updated_at();

do $$
begin
  if not exists (
    select 1
    from pg_constraint
    where conrelid = 'client_service_identities'::regclass
      and conname = 'client_service_identities_service_code_fk'
  ) then
    alter table client_service_identities
      add constraint client_service_identities_service_code_fk
      foreign key(service_code)
      references service_definitions(code)
      on update cascade
      on delete restrict
      not valid;
  end if;
end
$$;

create unique index if not exists idx_client_email_deliveries_id_client
  on client_email_deliveries(id, client_account_id);

create unique index if not exists idx_artifacts_id_client
  on artifacts(id, client_account_id);

create unique index if not exists idx_share_links_id_client
  on share_links(id, client_account_id);

create table if not exists client_email_delivery_artifacts(
  delivery_id uuid not null,
  client_account_id uuid not null,
  artifact_id uuid not null,
  created_at timestamptz not null default now(),
  primary key(delivery_id, artifact_id),
  constraint client_email_delivery_artifacts_delivery_fk
    foreign key(delivery_id, client_account_id)
    references client_email_deliveries(id, client_account_id)
    on delete cascade,
  constraint client_email_delivery_artifacts_artifact_fk
    foreign key(artifact_id, client_account_id)
    references artifacts(id, client_account_id)
    on delete cascade
);

create index if not exists idx_client_email_delivery_artifacts_artifact
  on client_email_delivery_artifacts(artifact_id, delivery_id);

create table if not exists client_email_delivery_share_links(
  delivery_id uuid not null,
  client_account_id uuid not null,
  share_link_id uuid not null,
  created_at timestamptz not null default now(),
  primary key(delivery_id, share_link_id),
  constraint client_email_delivery_share_links_delivery_fk
    foreign key(delivery_id, client_account_id)
    references client_email_deliveries(id, client_account_id)
    on delete cascade,
  constraint client_email_delivery_share_links_share_link_fk
    foreign key(share_link_id, client_account_id)
    references share_links(id, client_account_id)
    on delete cascade
);

create index if not exists idx_client_email_delivery_share_links_link
  on client_email_delivery_share_links(share_link_id, delivery_id);

insert into client_email_delivery_artifacts(
  delivery_id, client_account_id, artifact_id, created_at
)
select distinct
  delivery.id,
  delivery.client_account_id,
  artifact.id,
  delivery.created_at
from client_email_deliveries delivery
cross join lateral jsonb_array_elements_text(
  case
    when jsonb_typeof(delivery.artifact_ids) = 'array' then delivery.artifact_ids
    else '[]'::jsonb
  end
) as item(value)
join artifacts artifact
  on artifact.id::text = lower(btrim(item.value))
 and artifact.client_account_id = delivery.client_account_id
on conflict(delivery_id, artifact_id) do nothing;

insert into client_email_delivery_share_links(
  delivery_id, client_account_id, share_link_id, created_at
)
select distinct
  delivery.id,
  delivery.client_account_id,
  share_link.id,
  delivery.created_at
from client_email_deliveries delivery
cross join lateral jsonb_array_elements_text(
  case
    when jsonb_typeof(delivery.share_link_ids) = 'array' then delivery.share_link_ids
    else '[]'::jsonb
  end
) as item(value)
join share_links share_link
  on share_link.id::text = lower(btrim(item.value))
 and share_link.client_account_id = delivery.client_account_id
on conflict(delivery_id, share_link_id) do nothing;

create table if not exists backhaul_transport_secrets(
  transport_id uuid not null references backhaul_transports(id) on delete cascade,
  purpose text not null,
  secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key(transport_id, purpose),
  constraint backhaul_transport_secrets_purpose_check
    check(purpose ~ '^[a-z][a-z0-9_]{0,63}$')
);

create index if not exists idx_backhaul_transport_secrets_ref
  on backhaul_transport_secrets(secret_ref_id, transport_id);

insert into backhaul_transport_secrets(
  transport_id, purpose, secret_ref_id, created_at, updated_at
)
select
  transport.id,
  item.key,
  secret_ref.id,
  transport.created_at,
  transport.updated_at
from backhaul_transports transport
cross join lateral jsonb_each(
  case
    when jsonb_typeof(transport.secret_refs_json) = 'object' then transport.secret_refs_json
    else '{}'::jsonb
  end
) as item(key, value)
join secret_refs secret_ref
  on secret_ref.id::text = lower(btrim(item.value #>> '{}'))
where item.key ~ '^[a-z][a-z0-9_]{0,63}$'
on conflict(transport_id, purpose) do nothing;

insert into audit_events(
  id, actor_type, action, resource_type, summary, payload_json, created_at
)
values(
  gen_random_uuid(),
  'system',
  'migration.schema_normalization',
  'platform',
  'stable email and backhaul relationships normalized',
  jsonb_build_object(
    'client_service_identity_unknown_codes', (
      select count(*)
      from client_service_identities identity
      left join service_definitions definition on definition.code = identity.service_code
      where definition.code is null
    ),
    'client_service_identity_unknown_code_sample', (
      select coalesce(jsonb_agg(sample.service_code order by sample.service_code), '[]'::jsonb)
      from (
        select distinct identity.service_code
        from client_service_identities identity
        left join service_definitions definition on definition.code = identity.service_code
        where definition.code is null
        order by identity.service_code
        limit 100
      ) sample
    ),
    'email_delivery_invalid_artifact_containers', (
      select count(*)
      from client_email_deliveries
      where jsonb_typeof(artifact_ids) <> 'array'
    ),
    'email_delivery_unresolved_artifact_refs', (
      select count(*)
      from client_email_deliveries delivery
      cross join lateral jsonb_array_elements_text(
        case when jsonb_typeof(delivery.artifact_ids) = 'array' then delivery.artifact_ids else '[]'::jsonb end
      ) item(value)
      where not exists (
        select 1
        from artifacts artifact
        where artifact.id::text = lower(btrim(item.value))
          and artifact.client_account_id = delivery.client_account_id
      )
    ),
    'email_delivery_unresolved_artifact_sample', (
      select coalesce(jsonb_agg(sample.payload order by sample.delivery_id, sample.reference), '[]'::jsonb)
      from (
        select
          delivery.id as delivery_id,
          item.value as reference,
          jsonb_build_object(
            'delivery_id', delivery.id,
            'client_account_id', delivery.client_account_id,
            'artifact_ref', item.value
          ) as payload
        from client_email_deliveries delivery
        cross join lateral jsonb_array_elements_text(
          case when jsonb_typeof(delivery.artifact_ids) = 'array' then delivery.artifact_ids else '[]'::jsonb end
        ) item(value)
        where not exists (
          select 1
          from artifacts artifact
          where artifact.id::text = lower(btrim(item.value))
            and artifact.client_account_id = delivery.client_account_id
        )
        order by delivery.id, item.value
        limit 100
      ) sample
    ),
    'email_delivery_invalid_share_link_containers', (
      select count(*)
      from client_email_deliveries
      where jsonb_typeof(share_link_ids) <> 'array'
    ),
    'email_delivery_unresolved_share_link_refs', (
      select count(*)
      from client_email_deliveries delivery
      cross join lateral jsonb_array_elements_text(
        case when jsonb_typeof(delivery.share_link_ids) = 'array' then delivery.share_link_ids else '[]'::jsonb end
      ) item(value)
      where not exists (
        select 1
        from share_links share_link
        where share_link.id::text = lower(btrim(item.value))
          and share_link.client_account_id = delivery.client_account_id
      )
    ),
    'email_delivery_unresolved_share_link_sample', (
      select coalesce(jsonb_agg(sample.payload order by sample.delivery_id, sample.reference), '[]'::jsonb)
      from (
        select
          delivery.id as delivery_id,
          item.value as reference,
          jsonb_build_object(
            'delivery_id', delivery.id,
            'client_account_id', delivery.client_account_id,
            'share_link_ref', item.value
          ) as payload
        from client_email_deliveries delivery
        cross join lateral jsonb_array_elements_text(
          case when jsonb_typeof(delivery.share_link_ids) = 'array' then delivery.share_link_ids else '[]'::jsonb end
        ) item(value)
        where not exists (
          select 1
          from share_links share_link
          where share_link.id::text = lower(btrim(item.value))
            and share_link.client_account_id = delivery.client_account_id
        )
        order by delivery.id, item.value
        limit 100
      ) sample
    ),
    'backhaul_invalid_secret_containers', (
      select count(*)
      from backhaul_transports
      where jsonb_typeof(secret_refs_json) <> 'object'
    ),
    'backhaul_unresolved_secret_refs', (
      select count(*)
      from backhaul_transports transport
      cross join lateral jsonb_each(
        case when jsonb_typeof(transport.secret_refs_json) = 'object' then transport.secret_refs_json else '{}'::jsonb end
      ) item(key, value)
      where item.key !~ '^[a-z][a-z0-9_]{0,63}$'
         or not exists (
           select 1
           from secret_refs secret_ref
           where secret_ref.id::text = lower(btrim(item.value #>> '{}'))
         )
    ),
    'backhaul_unresolved_secret_ref_sample', (
      select coalesce(jsonb_agg(sample.payload order by sample.transport_id, sample.purpose), '[]'::jsonb)
      from (
        select
          transport.id as transport_id,
          item.key as purpose,
          jsonb_build_object(
            'transport_id', transport.id,
            'purpose', item.key,
            'secret_ref_id', item.value #>> '{}'
          ) as payload
        from backhaul_transports transport
        cross join lateral jsonb_each(
          case when jsonb_typeof(transport.secret_refs_json) = 'object' then transport.secret_refs_json else '{}'::jsonb end
        ) item(key, value)
        where item.key !~ '^[a-z][a-z0-9_]{0,63}$'
           or not exists (
             select 1
             from secret_refs secret_ref
             where secret_ref.id::text = lower(btrim(item.value #>> '{}'))
           )
        order by transport.id, item.key
        limit 100
      ) sample
    )
  ),
  now()
);

alter table client_email_deliveries
  drop column artifact_ids,
  drop column share_link_ids;

alter table backhaul_transports
  drop column secret_refs_json;

do $$
begin
  if not exists (
    select 1
    from client_service_identities identity
    left join service_definitions definition on definition.code = identity.service_code
    where definition.code is null
  ) then
    alter table client_service_identities
      validate constraint client_service_identities_service_code_fk;
  end if;
end
$$;

create or replace function prevent_managed_node_delete()
returns trigger
language plpgsql
as $$
begin
  if lower(coalesce(current_setting('megavpn.allow_node_delete', true), 'off')) in ('on', 'true', '1') then
    return old;
  end if;

  raise exception 'physical node deletion is disabled; retire the node to preserve retained traffic and inventory history'
    using errcode = '55000',
          hint = 'For an approved permanent purge, use SET LOCAL megavpn.allow_node_delete = on inside the purge transaction.';
end
$$;

drop trigger if exists nodes_preserve_history_guard on nodes;
create trigger nodes_preserve_history_guard
before delete on nodes
for each row
execute function prevent_managed_node_delete();

comment on column client_service_identities.service_code is
  'Runtime service driver code protected by service_definitions(code).';

comment on table client_email_delivery_artifacts is
  'Artifacts attached to a client email delivery; composite foreign keys prevent cross-client attachment.';

comment on table client_email_delivery_share_links is
  'Share links attached to a client email delivery; composite foreign keys prevent cross-client attachment.';

comment on table backhaul_transport_secrets is
  'Typed secret references owned by a backhaul transport. Secret material remains encrypted in secret_refs.';

comment on table nodes is
  'Managed nodes use status=retired for lifecycle removal. Physical DELETE is guarded to preserve retained traffic and inventory history.';
