create table if not exists mail_delivery_events (
    id uuid primary key,
    message_type text not null check (message_type in ('test', 'user_invite', 'password_reset', 'client_artifact')),
    recipient_email text not null,
    subject text not null default '',
    status text not null check (status in ('sent', 'failed')),
    actor_user_id uuid references platform_users(id) on delete set null,
    actor_username text not null default '',
    resource_type text not null default '',
    resource_id uuid,
    error_text text not null default '',
    sent_at timestamptz,
    created_at timestamptz not null default now()
);

create index if not exists idx_mail_delivery_events_created_at
    on mail_delivery_events(created_at desc);
create index if not exists idx_mail_delivery_events_status_created_at
    on mail_delivery_events(status, created_at desc);
create index if not exists idx_mail_delivery_events_type_created_at
    on mail_delivery_events(message_type, created_at desc);
create index if not exists idx_mail_delivery_events_recipient
    on mail_delivery_events(lower(recipient_email), created_at desc);

insert into mail_delivery_events(
    id, message_type, recipient_email, subject, status, actor_user_id, actor_username,
    resource_type, resource_id, error_text, sent_at, created_at
)
select
    d.id,
    'client_artifact',
    d.email,
    d.subject,
    case when d.status = 'sent' then 'sent' else 'failed' end,
    d.created_by,
    coalesce(u.username, ''),
    'client_account',
    d.client_account_id,
    left(coalesce(d.error_text, ''), 1000),
    d.sent_at,
    d.created_at
from client_email_deliveries d
left join platform_users u on u.id = d.created_by
where d.status in ('sent', 'failed')
on conflict (id) do nothing;

insert into mail_delivery_events(
    id, message_type, recipient_email, subject, status, actor_user_id, actor_username,
    resource_type, resource_id, error_text, sent_at, created_at
)
select
    i.id,
    'user_invite',
    i.email,
    'RTIS MegaVPN operator invitation',
    case when i.status in ('sent', 'accepted') then 'sent' else 'failed' end,
    i.created_by,
    coalesce(u.username, ''),
    'platform_user',
    i.user_id,
    left(coalesce(i.delivery_error, ''), 1000),
    i.sent_at,
    i.created_at
from platform_user_invites i
left join platform_users u on u.id = i.created_by
where i.status in ('sent', 'accepted', 'delivery_failed')
on conflict (id) do nothing;
