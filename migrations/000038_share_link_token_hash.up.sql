create extension if not exists pgcrypto;

alter table share_links
  add column if not exists token_hash text null;

alter table share_links
  add column if not exists token_hint text not null default '';

update share_links
set token_hash = encode(digest(token, 'sha256'), 'hex'),
    token_hint = case
      when length(token) <= 14 then token
      else substring(token from 1 for 8) || '...' || substring(token from length(token) - 5 for 6)
    end
where token_hash is null
  and token is not null
  and token <> '';

update share_links
set token_hash = encode(digest(id::text || ':' || created_at::text, 'sha256'), 'hex'),
    token_hint = 'revoked',
    status = 'revoked'
where token_hash is null;

alter table share_links
  alter column token_hash set not null;

alter table share_links
  alter column token drop not null;

alter table share_links
  drop constraint if exists share_links_token_key;

create unique index if not exists idx_share_links_token_hash
  on share_links(token_hash);

update share_links
set token = null
where token is not null;

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
values (gen_random_uuid(), 'system', 'migration.share_link_token_hash', 'security', 'share link plaintext tokens migrated to token_hash/token_hint', '{}'::jsonb, now());
