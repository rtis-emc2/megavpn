-- Node map location accuracy metadata.
-- Release: 0.7.0.1-beta

alter table nodes
  add column if not exists accuracy_radius_km double precision null;
