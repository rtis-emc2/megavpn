-- Add automatic GeoIP cache fields for node map rendering.

alter table nodes
  add column if not exists geoip_provider text not null default '';

alter table nodes
  add column if not exists geoip_status text not null default 'pending';

alter table nodes
  add column if not exists geoip_ip text not null default '';

alter table nodes
  add column if not exists geoip_country_code text not null default '';

alter table nodes
  add column if not exists geoip_country_name text not null default '';

alter table nodes
  add column if not exists geoip_region text not null default '';

alter table nodes
  add column if not exists geoip_city text not null default '';

alter table nodes
  add column if not exists geoip_org text not null default '';

alter table nodes
  add column if not exists geoip_asn text not null default '';

alter table nodes
  add column if not exists geoip_resolved_at timestamptz null;

alter table nodes
  add column if not exists geoip_error text not null default '';

update nodes
set geoip_status = 'pending'
where status <> 'retired'
  and coalesce(geoip_status, '') = '';
