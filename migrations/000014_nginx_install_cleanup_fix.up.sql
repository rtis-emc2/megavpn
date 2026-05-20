-- Nginx installer cleanup / fallback hardening marker.
-- Runtime behavior is implemented in the agent installer. This migration records the build-level fix in audit.

insert into audit_events (
  id,
  actor_type,
  action,
  resource_type,
  summary,
  payload_json,
  created_at
)
values (
  gen_random_uuid(),
  'system',
  'migration.nginx_install_cleanup_fix',
  'platform',
  'nginx install cleanup and fallback hardening installed',
  '{"scope":["nginx_org_repo_noninteractive_gpg","ubuntu_repo_clean_fallback","ubuntu_26_04_resolute_preflight","apt_policy_guard"]}'::jsonb,
  now()
)
on conflict do nothing;
