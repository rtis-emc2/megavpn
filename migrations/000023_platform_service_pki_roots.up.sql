-- Platform service PKI roots.

create table if not exists platform_service_pki_roots(
  id uuid primary key,
  service_code text not null,
  pki_profile text not null default 'default',
  status text not null check(status in ('active','rotated','revoked')),
  ca_cert_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  ca_key_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  common_name text not null,
  created_at timestamptz not null default now(),
  rotated_at timestamptz null
);

create unique index if not exists idx_platform_service_pki_roots_active
on platform_service_pki_roots(service_code, pki_profile)
where status = 'active';

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.platform_service_pki_roots','platform','platform service pki roots installed','{}'::jsonb,now())
on conflict do nothing;
