-- Release: 7.0.1.29
-- Document and preserve operator-controlled address-list status semantics for
-- the default firewall seed path. Runtime seeding is corrected in Go code; this
-- migration records the release boundary for upgraded installations.

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
select gen_random_uuid(), 'system', 'migration.firewall_seed_status_preserve', 'firewall', 'firewall default seed preserves operator-disabled address lists', '{}'::jsonb, now()
where not exists(select 1 from audit_events where action='migration.firewall_seed_status_preserve');
