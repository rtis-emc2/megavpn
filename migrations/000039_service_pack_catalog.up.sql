create extension if not exists pgcrypto;

create table if not exists service_pack_templates (
  key text primary key,
  label text not null,
  description text not null default '',
  base_name_template text not null default '',
  endpoint_hint text not null default '',
  requires_endpoint_host boolean not null default true,
  platform_notes_json jsonb not null default '[]'::jsonb,
  recommendations_json jsonb not null default '[]'::jsonb,
  components_json jsonb not null default '[]'::jsonb,
  status text not null default 'active',
  source text not null default 'default',
  version integer not null default 1,
  display_order integer not null default 1000,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint service_pack_templates_key_check check (key ~ '^[a-z0-9][a-z0-9_-]{1,95}$'),
  constraint service_pack_templates_status_check check (status in ('active','disabled','deleted')),
  constraint service_pack_templates_version_check check (version > 0),
  constraint service_pack_templates_components_array_check check (jsonb_typeof(components_json) = 'array'),
  constraint service_pack_templates_platform_notes_array_check check (jsonb_typeof(platform_notes_json) = 'array'),
  constraint service_pack_templates_recommendations_array_check check (jsonb_typeof(recommendations_json) = 'array')
);

create index if not exists idx_service_pack_templates_status_order
  on service_pack_templates(status, display_order, label);

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.service_pack_catalog', 'service_pack', 'service pack template catalog created', '{}'::jsonb, now());
