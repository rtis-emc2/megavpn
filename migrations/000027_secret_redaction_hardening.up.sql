-- Remove historical plaintext bootstrap and agent-rotation secrets from JSON payload history.

update jobs
set payload_json = jsonb_set(
  payload_json - 'new_agent_token',
  '{new_agent_token_hash}',
  to_jsonb(encode(digest(coalesce(payload_json->>'new_agent_token',''), 'sha256'), 'hex')),
  true
)
where type = 'node.agent.rotate_token'
  and payload_json ? 'new_agent_token';

update jobs
set status = 'cancelled',
    finished_at = coalesce(finished_at, now()),
    locked_by = null,
    locked_until = null,
    result_json = coalesce(result_json, '{}'::jsonb) ||
      jsonb_build_object(
        'message', 'token rotation cancelled by migration because plaintext payload was redacted; queue a fresh rotation',
        'redacted_by_migration', '000027_secret_redaction_hardening'
      )
where type = 'node.agent.rotate_token'
  and (
    status in ('queued','retrying')
    or (status = 'running' and locked_until is not null and locked_until < now())
  )
  and not (payload_json ? 'new_agent_token_secret_ref_id');

delete from resource_locks
where job_id in (
  select id
  from jobs
  where type = 'node.agent.rotate_token'
    and status = 'cancelled'
    and coalesce(result_json->>'redacted_by_migration','') = '000027_secret_redaction_hardening'
);

update jobs
set result_json = (coalesce(result_json, '{}'::jsonb) - 'agent_bootstrapenv') ||
  jsonb_build_object(
    'agent_bootstrapenv_redacted', true,
    'redacted_by_migration', '000027_secret_redaction_hardening'
  )
where coalesce(result_json, '{}'::jsonb) ? 'agent_bootstrapenv';

update node_bootstrap_runs
set result_payload_json = (coalesce(result_payload_json, '{}'::jsonb) - 'agent_bootstrapenv') ||
  jsonb_build_object(
    'agent_bootstrapenv_redacted', true,
    'redacted_by_migration', '000027_secret_redaction_hardening'
  )
where coalesce(result_payload_json, '{}'::jsonb) ? 'agent_bootstrapenv';

update job_logs
set payload_json = (coalesce(payload_json, '{}'::jsonb) - 'new_agent_token' - 'agent_bootstrapenv') ||
  jsonb_build_object(
    'secrets_redacted', true,
    'redacted_by_migration', '000027_secret_redaction_hardening'
  )
where coalesce(payload_json, '{}'::jsonb) ?| array['new_agent_token','agent_bootstrapenv'];

insert into audit_events(id, actor_user_id, actor_type, action, resource_type, summary, payload_json, created_at)
values(
  gen_random_uuid(),
  null,
  'system',
  'migration.secret_redaction_hardening',
  'platform',
  'historical plaintext job/bootstrap secrets redacted',
  '{}'::jsonb,
  now()
)
on conflict do nothing;
