alter table node_agents add column if not exists last_auth_failure_at timestamptz null;
alter table node_agents add column if not exists last_auth_failure_reason text not null default '';
alter table node_agents add column if not exists last_job_poll_at timestamptz null;
alter table node_agents add column if not exists last_job_claim_at timestamptz null;
alter table node_agents add column if not exists last_job_claim_job_id uuid null references jobs(id) on delete set null;
alter table node_agents add column if not exists last_job_claim_type text not null default '';
alter table node_agents add column if not exists last_job_result_at timestamptz null;
alter table node_agents add column if not exists last_job_result_job_id uuid null references jobs(id) on delete set null;
alter table node_agents add column if not exists last_job_result_type text not null default '';
alter table node_agents add column if not exists last_job_result_status text not null default '';
alter table node_agents add column if not exists last_inventory_sync_at timestamptz null;
alter table node_agents add column if not exists last_discovery_sync_at timestamptz null;

create index if not exists idx_node_agents_last_job_claim_at on node_agents(last_job_claim_at desc);
create index if not exists idx_node_agents_last_job_result_at on node_agents(last_job_result_at desc);
