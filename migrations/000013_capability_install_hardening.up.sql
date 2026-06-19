-- MegaVPN 0.6.10.1-alpha: capability install hardening.
-- Makes capability result storage tolerant to failed installers and installer-owned states.

alter table node_capabilities
  drop constraint if exists node_capabilities_status_check;

alter table node_capabilities
  add constraint node_capabilities_status_check
  check(status in ('available','missing','broken','disabled','installing','failed','degraded','unknown'));

alter table node_capabilities
  drop constraint if exists node_capabilities_source_check;

alter table node_capabilities
  add constraint node_capabilities_source_check
  check(source in ('inventory','manual','bootstrap','installer','verification','system'));

alter table node_capability_install_events
  drop constraint if exists node_capability_install_events_status_check;

alter table node_capability_install_events
  add constraint node_capability_install_events_status_check
  check(status in ('queued','running','succeeded','failed','verified','preflight_failed','fallback_used','cancelled'));

create index if not exists idx_node_capability_install_events_job
  on node_capability_install_events(job_id, created_at desc);

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
values(gen_random_uuid(),'system','migration.capability_install_hardening','platform','capability install hardening schema installed','{}'::jsonb,now())
on conflict do nothing;
