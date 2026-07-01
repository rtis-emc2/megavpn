-- Node map location metadata.
-- Release: 7.0.1.1

alter table nodes
  add column if not exists location_label text not null default '';

alter table nodes
  add column if not exists latitude double precision null;

alter table nodes
  add column if not exists longitude double precision null;

alter table nodes
  add column if not exists accuracy_radius_km double precision null;
