-- Node map location accuracy metadata.
-- Release: 7.0.1.1

alter table nodes
  add column if not exists accuracy_radius_km double precision null;
