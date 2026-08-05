drop index if exists client_access_groups_external_egress_idx;
drop trigger if exists client_access_groups_external_egress_guard on client_access_groups;
drop trigger if exists external_egress_deployments_profile_guard on external_egress_deployments;
drop trigger if exists external_egress_profiles_lifecycle_guard on external_egress_profiles;
drop function if exists megavpn_validate_external_egress_reference();
drop function if exists megavpn_guard_external_egress_profile_lifecycle();
alter table client_access_groups drop column if exists external_egress_profile_id;
drop table if exists external_egress_deployments;
with removed_external_egress_secrets as (
  delete from external_egress_profile_secrets returning secret_ref_id
)
delete from secret_refs
where id in (select secret_ref_id from removed_external_egress_secrets);
drop table if exists external_egress_profile_secrets;
drop table if exists external_egress_profiles;
delete from schema_migrations where version='000024_external_egress_profiles';
