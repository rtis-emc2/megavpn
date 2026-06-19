create table if not exists platform_certificates(
  id uuid primary key,
  name text not null,
  description text not null default '',
  source text not null check(source in ('imported','self_signed','managed_ca','ca_issued','letsencrypt')),
  kind text not null check(kind in ('leaf','ca')),
  status text not null default 'active',
  common_name text not null default '',
  san_json jsonb not null default '[]'::jsonb,
  issuer_name text not null default '',
  parent_certificate_id uuid null references platform_certificates(id) on delete set null,
  cert_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  key_secret_ref_id uuid null references secret_refs(id) on delete restrict,
  chain_secret_ref_id uuid null references secret_refs(id) on delete restrict,
  not_before timestamptz null,
  not_after timestamptz null,
  is_default boolean not null default false,
  meta_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_platform_certificates_kind_status
on platform_certificates(kind,status,created_at desc);

create index if not exists idx_platform_certificates_default
on platform_certificates(is_default,kind,status);

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.platform_certificates','platform','platform certificates installed','{}'::jsonb,now())
on conflict do nothing;
