-- MegaVPN MVP secret refs and agent trust foundation.

create table if not exists secret_refs(
  id uuid primary key,
  secret_type text not null check(secret_type in ('password','uuid','private_key','public_key','certificate','psk','ssh_key','api_token','opaque')),
  ciphertext bytea not null,
  key_version text not null,
  nonce bytea null,
  meta_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  rotated_at timestamptz null
);

create table if not exists agent_trust_roots(
  id uuid primary key,
  name text not null,
  status text not null check(status in ('active','rotated','revoked')),
  ca_cert_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  ca_key_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  created_by uuid null references platform_users(id) on delete set null,
  created_at timestamptz not null default now(),
  rotated_at timestamptz null
);

create table if not exists node_agent_certificates(
  id uuid primary key,
  node_id uuid not null references nodes(id) on delete cascade,
  trust_root_id uuid not null references agent_trust_roots(id) on delete restrict,
  serial_no text not null unique,
  cert_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  key_secret_ref_id uuid not null references secret_refs(id) on delete restrict,
  status text not null check(status in ('issued','active','rotated','revoked','expired')),
  issued_at timestamptz not null,
  expires_at timestamptz not null,
  revoked_at timestamptz null
);

create index if not exists idx_node_agent_certificates_node on node_agent_certificates(node_id, status);
create index if not exists idx_node_agent_certificates_expires on node_agent_certificates(expires_at);

insert into audit_events(id, actor_user_id, actor_type, action, resource_type, summary, payload_json, created_at)
values(
  gen_random_uuid(),
  null,
  'system',
  'migration.secret_refs_and_agent_trust',
  'platform',
  'secret refs and agent trust foundation installed',
  '{}'::jsonb,
  now()
)
on conflict do nothing;
