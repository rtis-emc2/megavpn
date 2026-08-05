drop trigger if exists instances_revision_ownership_guard on instances;
drop function if exists validate_instance_revision_ownership();
drop trigger if exists instance_revisions_owner_immutable_guard on instance_revisions;
drop function if exists prevent_instance_revision_reparenting();

alter table if exists client_access_group_memberships
  drop constraint if exists client_access_group_memberships_group_service_fk;
alter table if exists audit_events
  drop constraint if exists audit_events_actor_user_fk;
alter table if exists instances
  drop constraint if exists instances_last_applied_revision_fk;
alter table if exists instances
  drop constraint if exists instances_current_revision_fk;

alter table if exists client_access_group_memberships
  add constraint client_access_group_memberships_service_code_check
  check(service_code in ('vless','openvpn','l2tp','wireguard','shadowsocks','http_proxy','mtproto'));

drop index if exists idx_client_access_groups_id_service;
drop index if exists idx_audit_events_actor_user;
drop index if exists idx_instances_last_applied_revision;
drop index if exists idx_instances_current_revision;
