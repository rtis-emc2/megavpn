-- Client VLESS subscription token registry.

create extension if not exists pgcrypto;

create table if not exists client_subscriptions(
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  token_hash text not null,
  token_hint text not null default '',
  status text not null default 'active' check(status in ('active','expired','revoked')),
  expires_at timestamptz not null,
  download_count bigint not null default 0,
  last_used_at timestamptz null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists client_subscriptions_token_hash_idx
  on client_subscriptions(token_hash);

create index if not exists client_subscriptions_client_status_idx
  on client_subscriptions(client_account_id, status, expires_at desc);

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
select gen_random_uuid(), 'system', 'migration.client_vless_subscriptions', 'client', 'client VLESS subscription token registry created', '{}'::jsonb, now()
where not exists(select 1 from audit_events where action='migration.client_vless_subscriptions');
