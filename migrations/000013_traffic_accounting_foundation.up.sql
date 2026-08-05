-- Release: 7.1.0.2
-- Traffic accounting foundation: aggregate counters only, no URLs, payloads or
-- request bodies. Samples are retained by the control plane for 180 days.

alter table permissions
  drop constraint if exists permissions_scope_type_check;

alter table permissions
  add constraint permissions_scope_type_check
  check(scope_type in ('global','node','instance','client','artifact','secret','job','audit','endpoint','traffic'));

create table if not exists traffic_accounting_samples(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  instance_id uuid null references instances(id) on delete set null,
  service_access_id uuid null references service_accesses(id) on delete set null,
  client_account_id uuid null references client_accounts(id) on delete set null,
  sample_key text not null default '',
  source text not null default 'agent' check(source in ('agent','import','manual','system')),
  protocol text not null default 'unknown',
  direction text not null default 'bidirectional' check(direction in ('ingress','egress','bidirectional','unknown')),
  bucket_start timestamptz not null,
  bucket_end timestamptz not null,
  rx_bytes bigint not null default 0 check(rx_bytes >= 0),
  tx_bytes bigint not null default 0 check(tx_bytes >= 0),
  rx_packets bigint not null default 0 check(rx_packets >= 0),
  tx_packets bigint not null default 0 check(tx_packets >= 0),
  flow_count bigint not null default 0 check(flow_count >= 0),
  metadata_json jsonb not null default '{}'::jsonb,
  observed_at timestamptz not null default now(),
  received_at timestamptz not null default now(),
  check(bucket_end > bucket_start)
);

create unique index if not exists idx_traffic_accounting_node_sample_key
  on traffic_accounting_samples(node_id, sample_key)
  where sample_key <> '';
create index if not exists idx_traffic_accounting_received_at on traffic_accounting_samples(received_at);
create index if not exists idx_traffic_accounting_bucket_start on traffic_accounting_samples(bucket_start);
create index if not exists idx_traffic_accounting_node_bucket on traffic_accounting_samples(node_id, bucket_start);
create index if not exists idx_traffic_accounting_client_bucket on traffic_accounting_samples(client_account_id, bucket_start);
create index if not exists idx_traffic_accounting_instance_bucket on traffic_accounting_samples(instance_id, bucket_start);

insert into permissions(id, code, name, scope_type, created_at)
values
  (gen_random_uuid(), 'traffic.read', 'Read traffic accounting', 'traffic', now())
on conflict(code) do nothing;

insert into role_permissions(role_id, permission_id)
select r.id, p.id
from roles r
join permissions p on p.code='traffic.read'
where r.code in ('readonly','engineer','admin','superadmin')
on conflict do nothing;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
select gen_random_uuid(), 'system', 'migration.traffic_accounting_foundation', 'traffic', 'traffic accounting aggregate storage installed', '{"retention_days":180}'::jsonb, now()
where not exists(select 1 from audit_events where action='migration.traffic_accounting_foundation');
