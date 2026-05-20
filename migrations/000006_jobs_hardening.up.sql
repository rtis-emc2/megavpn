-- Jobs subsystem hardening:
-- - DB-backed resource locks for mutating operations
-- - job lease indexes
-- - job_logs lookup indexes

create table if not exists resource_locks(
  id uuid primary key,
  resource_type text not null,
  resource_id uuid not null,
  lock_kind text not null,
  job_id uuid not null references jobs(id) on delete cascade,
  acquired_at timestamptz not null default now(),
  expires_at timestamptz not null,
  unique(resource_type, resource_id, lock_kind),
  check(lock_kind in ('mutate','delete','bootstrap','apply','provision'))
);

create index if not exists idx_resource_locks_job on resource_locks(job_id);
create index if not exists idx_resource_locks_expires_at on resource_locks(expires_at);
create index if not exists idx_job_logs_job_created on job_logs(job_id, created_at asc);
create index if not exists idx_jobs_locked_until on jobs(locked_until) where locked_until is not null;
create index if not exists idx_jobs_scope on jobs(scope_type, scope_id);

update jobs
set status='retrying', locked_by=null, locked_until=null
where status='running' and locked_until is not null and locked_until < now();
