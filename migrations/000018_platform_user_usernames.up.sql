-- Platform user usernames and bootstrap login support.

alter table platform_users
  add column if not exists username text;

update platform_users
set username = lower(
  case
    when position('@' in email) > 0 then split_part(email, '@', 1)
    else email
  end
)
where username is null or btrim(username) = '';

alter table platform_users
  alter column username set not null;

create unique index if not exists idx_platform_users_username on platform_users(username);

insert into audit_events(id, actor_user_id, actor_type, action, resource_type, summary, payload_json, created_at)
values(
  gen_random_uuid(),
  null,
  'system',
  'migration.platform_user_usernames',
  'platform',
  'platform user usernames installed',
  '{}'::jsonb,
  now()
)
on conflict do nothing;
