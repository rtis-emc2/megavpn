alter table instances drop constraint if exists instances_slug_key;
alter table instances drop constraint if exists instances_node_id_name_key;

create unique index if not exists instances_active_slug_idx
  on instances(slug)
  where status <> 'deleted';

create unique index if not exists instances_active_node_name_idx
  on instances(node_id, name)
  where status <> 'deleted';
