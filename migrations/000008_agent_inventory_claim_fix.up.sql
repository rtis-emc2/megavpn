-- Agent inventory claim fix:
-- inventory jobs are read-only collection jobs and must not be blocked by
-- stale node bootstrap locks left by older alpha builds.

delete from resource_locks rl
using jobs j
where rl.job_id = j.id
  and j.type in ('node.inventory', 'node.inventory.sync')
  and j.status in ('queued', 'retrying');

update jobs
set locked_by = null,
    locked_until = null
where type in ('node.inventory', 'node.inventory.sync')
  and status in ('queued', 'retrying');
