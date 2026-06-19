create table if not exists client_access_routes(
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  service_access_id uuid null references service_accesses(id) on delete cascade,
  instance_id uuid null references instances(id) on delete set null,
  node_id uuid null references nodes(id) on delete set null,
  name text not null,
  status text not null check(status in ('pending','active','disabled','revoked')),
  action text not null default 'allow' check(action in ('allow','deny')),
  destination_type text not null check(destination_type in ('endpoint','cidr','dns','service')),
  destination text not null,
  protocol text not null default 'any' check(protocol in ('any','tcp','udp','icmp')),
  ports text not null default '*',
  description text null,
  policy_json jsonb not null default '{}'::jsonb,
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_client_access_routes_client_status
on client_access_routes(client_account_id,status,created_at desc);

create index if not exists idx_client_access_routes_node_status
on client_access_routes(node_id,status,created_at desc);

create index if not exists idx_client_access_routes_access
on client_access_routes(service_access_id);

create unique index if not exists idx_client_access_routes_baseline_access
on client_access_routes(service_access_id)
where service_access_id is not null and (metadata_json->>'baseline') = 'true';

insert into audit_events(id,actor_type,action,resource_type,resource_id,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.client_access_routes','routing',null,'client access routing registry installed','{}'::jsonb,now())
on conflict do nothing;
