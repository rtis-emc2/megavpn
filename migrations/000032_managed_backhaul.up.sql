create table if not exists backhaul_links(
  id uuid primary key,
  name text not null,
  ingress_node_id uuid not null references nodes(id) on delete restrict,
  egress_node_id uuid not null references nodes(id) on delete restrict,
  status text not null default 'planned',
  selected_transport_id uuid null,
  desired_driver text not null default 'wireguard',
  routing_table text not null default 'main',
  route_metric integer not null default 50,
  failover_policy_json jsonb not null default '{}'::jsonb,
  metadata_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint backhaul_links_distinct_nodes check (ingress_node_id <> egress_node_id),
  constraint backhaul_links_status_check check (status in ('planned','pending_apply','active','disabled','failed','deleted'))
);

create unique index if not exists backhaul_links_active_pair_idx
on backhaul_links(ingress_node_id, egress_node_id, name)
where status <> 'deleted';

create index if not exists backhaul_links_ingress_idx
on backhaul_links(ingress_node_id)
where status <> 'deleted';

create index if not exists backhaul_links_egress_idx
on backhaul_links(egress_node_id)
where status <> 'deleted';

create table if not exists backhaul_transports(
  id uuid primary key,
  link_id uuid not null references backhaul_links(id) on delete cascade,
  driver text not null,
  priority integer not null default 100,
  status text not null default 'planned',
  endpoint_host text not null default '',
  endpoint_port integer not null default 0,
  protocol text not null default '',
  interface_name text not null default '',
  tunnel_cidr text not null default '',
  ingress_address text not null default '',
  egress_address text not null default '',
  config_json jsonb not null default '{}'::jsonb,
  secret_refs_json jsonb not null default '{}'::jsonb,
  health_json jsonb not null default '{}'::jsonb,
  applied_ingress_at timestamptz null,
  applied_egress_at timestamptz null,
  last_error text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint backhaul_transports_status_check check (status in ('planned','pending_apply','active','disabled','failed'))
);

create unique index if not exists backhaul_transports_link_driver_idx
on backhaul_transports(link_id, driver);

create index if not exists backhaul_transports_active_driver_idx
on backhaul_transports(driver, status);

alter table backhaul_links
  drop constraint if exists backhaul_links_selected_transport_fk;

alter table backhaul_links
  add constraint backhaul_links_selected_transport_fk
  foreign key(selected_transport_id) references backhaul_transports(id) on delete set null;
