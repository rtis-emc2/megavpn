drop trigger if exists nodes_preserve_history_guard on nodes;
drop function if exists prevent_managed_node_delete();

alter table backhaul_transports
  add column if not exists secret_refs_json jsonb not null default '{}'::jsonb;

update backhaul_transports transport
set secret_refs_json = refs.payload
from (
  select transport_id, jsonb_object_agg(purpose, secret_ref_id::text order by purpose) as payload
  from backhaul_transport_secrets
  group by transport_id
) refs
where refs.transport_id = transport.id;

alter table client_email_deliveries
  add column if not exists artifact_ids jsonb not null default '[]'::jsonb,
  add column if not exists share_link_ids jsonb not null default '[]'::jsonb;

update client_email_deliveries delivery
set artifact_ids = refs.payload
from (
  select delivery_id, jsonb_agg(artifact_id::text order by artifact_id::text) as payload
  from client_email_delivery_artifacts
  group by delivery_id
) refs
where refs.delivery_id = delivery.id;

update client_email_deliveries delivery
set share_link_ids = refs.payload
from (
  select delivery_id, jsonb_agg(share_link_id::text order by share_link_id::text) as payload
  from client_email_delivery_share_links
  group by delivery_id
) refs
where refs.delivery_id = delivery.id;

drop table if exists backhaul_transport_secrets;
drop table if exists client_email_delivery_share_links;
drop table if exists client_email_delivery_artifacts;

drop index if exists idx_share_links_id_client;
drop index if exists idx_artifacts_id_client;
drop index if exists idx_client_email_deliveries_id_client;

alter table client_service_identities
  drop constraint if exists client_service_identities_service_code_fk;

drop trigger if exists service_definitions_updated_at_guard on service_definitions;
drop function if exists touch_service_definitions_updated_at();

alter table service_definitions
  drop column if exists updated_at;
