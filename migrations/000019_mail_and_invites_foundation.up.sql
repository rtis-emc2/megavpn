create table if not exists platform_mail_settings(
  id boolean primary key default true,
  enabled boolean not null default false,
  provider text not null default 'smtp',
  smtp_host text not null default '',
  smtp_port integer not null default 587,
  smtp_username text not null default '',
  smtp_password_secret_ref_id uuid null references secret_refs(id) on delete set null,
  smtp_auth_mode text not null default 'plain',
  smtp_tls_mode text not null default 'starttls',
  from_email text not null default '',
  from_name text not null default '',
  reply_to_email text not null default '',
  invite_url_base text not null default '',
  last_test_at timestamptz null,
  last_error text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint platform_mail_settings_singleton check (id = true),
  constraint platform_mail_settings_provider_check check (provider in ('smtp')),
  constraint platform_mail_settings_auth_mode_check check (smtp_auth_mode in ('none','plain')),
  constraint platform_mail_settings_tls_mode_check check (smtp_tls_mode in ('none','starttls','starttls_required'))
);

insert into platform_mail_settings(id, created_at, updated_at)
values (true, now(), now())
on conflict (id) do nothing;

create table if not exists platform_user_invites(
  id uuid primary key,
  user_id uuid not null references platform_users(id) on delete cascade,
  email text not null,
  token_hash text not null unique,
  token_hint text not null default '',
  status text not null default 'pending',
  expires_at timestamptz not null,
  sent_at timestamptz null,
  accepted_at timestamptz null,
  delivery_error text not null default '',
  created_by uuid null references platform_users(id) on delete set null,
  created_at timestamptz not null default now(),
  constraint platform_user_invites_status_check check (status in ('pending','sent','accepted','revoked','expired','delivery_failed'))
);

create index if not exists idx_platform_user_invites_user_status on platform_user_invites(user_id, status);
create index if not exists idx_platform_user_invites_expires on platform_user_invites(expires_at);

create table if not exists client_email_deliveries(
  id uuid primary key,
  client_account_id uuid not null references client_accounts(id) on delete cascade,
  email text not null,
  subject text not null,
  status text not null default 'queued',
  artifact_ids jsonb not null default '[]'::jsonb,
  share_link_ids jsonb not null default '[]'::jsonb,
  payload_json jsonb not null default '{}'::jsonb,
  error_text text not null default '',
  created_by uuid null references platform_users(id) on delete set null,
  sent_at timestamptz null,
  created_at timestamptz not null default now(),
  constraint client_email_deliveries_status_check check (status in ('queued','sent','failed'))
);

create index if not exists idx_client_email_deliveries_client_created on client_email_deliveries(client_account_id, created_at desc);

