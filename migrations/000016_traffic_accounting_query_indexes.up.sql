-- Release 7.1.0.6: traffic accounting query and retention hardening.
-- These indexes support overview/export ordering and common operator filters.

create index if not exists idx_traffic_accounting_bucket_end_received
  on traffic_accounting_samples(bucket_end desc, received_at desc);

create index if not exists idx_traffic_accounting_client_export
  on traffic_accounting_samples(client_account_id, bucket_end desc, received_at desc)
  where client_account_id is not null;

create index if not exists idx_traffic_accounting_node_export
  on traffic_accounting_samples(node_id, bucket_end desc, received_at desc);

create index if not exists idx_traffic_accounting_protocol_export
  on traffic_accounting_samples(protocol, bucket_end desc, received_at desc);

insert into audit_events(id, actor_type, action, resource_type, summary, payload_json, created_at)
select
  gen_random_uuid(),
  'system',
  'migration.traffic_accounting_query_indexes',
  'traffic',
  'traffic accounting query indexes installed',
  '{"release":"7.1.0.6","indexes":["bucket_end_received","client_export","node_export","protocol_export"]}'::jsonb,
  now()
where not exists(select 1 from audit_events where action='migration.traffic_accounting_query_indexes');
