insert into firewall_address_lists(id,key,label,description,scope,status,created_at,updated_at)
values
  (gen_random_uuid(),'backhaul_sources','Backhaul source ranges','Managed ingress-to-egress tunnel and backhaul source networks.','global','active',now(),now()),
  (gen_random_uuid(),'public_service_sources','Public service sources','Internet or restricted source ranges allowed to reach public service listeners.','global','active',now(),now()),
  (gen_random_uuid(),'blocked_destinations','Blocked destinations','Reusable destination ranges for deny or quarantine rules.','global','active',now(),now())
on conflict(key) do update
set label=excluded.label,
    description=excluded.description,
    scope=excluded.scope,
    status=firewall_address_lists.status,
    updated_at=now();

insert into audit_events(id,actor_type,action,resource_type,summary,payload_json,created_at)
select gen_random_uuid(),'system','migration.firewall_semantic_groups','firewall','firewall semantic address groups seeded','{}'::jsonb,now()
where not exists (
  select 1 from audit_events where action='migration.firewall_semantic_groups'
);
