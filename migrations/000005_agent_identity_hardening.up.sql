-- Final alpha hardening for agent identity lifecycle.
-- Keep at most one active enrollment token per node.
update node_enrollment_tokens
set status='expired'
where status='active' and expires_at <= now();

with ranked as (
  select id,
         row_number() over (partition by node_id order by created_at desc) as rn
  from node_enrollment_tokens
  where status='active'
)
update node_enrollment_tokens t
set status='revoked'
from ranked r
where t.id = r.id and r.rn > 1;

create unique index if not exists ux_node_enrollment_tokens_one_active
  on node_enrollment_tokens(node_id)
  where status='active';

create index if not exists idx_node_agents_active_node
  on node_agents(node_id, status)
  where revoked_at is null;
