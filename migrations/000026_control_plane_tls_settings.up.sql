create table if not exists platform_control_plane_tls_settings(
  id boolean primary key default true,
  enabled boolean not null default true,
  mode text not null default 'managed_certificate',
  public_base_url text not null default '',
  server_name text not null default '',
  listen_port integer not null default 443,
  upstream_url text not null default 'http://127.0.0.1:8080',
  certificate_id uuid null references platform_certificates(id) on delete set null,
  self_signed_common_name text not null default '',
  self_signed_san_json jsonb not null default '[]'::jsonb,
  last_applied_at timestamptz null,
  last_error text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint platform_control_plane_tls_settings_singleton check (id = true),
  constraint platform_control_plane_tls_settings_mode_check check (mode in ('managed_certificate','self_signed_fallback')),
  constraint platform_control_plane_tls_settings_port_check check (listen_port between 1 and 65535)
);

insert into platform_control_plane_tls_settings(id, created_at, updated_at)
values(true, now(), now())
on conflict(id) do nothing;

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.control_plane_tls_settings','platform','control plane tls settings installed','{}'::jsonb,now())
on conflict do nothing;
