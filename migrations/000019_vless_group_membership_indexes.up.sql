create index if not exists idx_service_accesses_instance_status
on service_accesses(instance_id, status);

create index if not exists idx_service_accesses_instance_vless_group
on service_accesses(instance_id, (coalesce(
  nullif(metadata_json->>'vless_group',''),
  nullif(metadata_json->>'xray_group',''),
  nullif(metadata_json->>'outbound_group',''),
  nullif(metadata_json->'inbound_service'->>'vless_group','')
)))
where status in ('pending','active','disabled');

create index if not exists idx_client_accounts_lookup_lower
on client_accounts(lower(username), lower(coalesce(email,'')))
where status <> 'deleted';
