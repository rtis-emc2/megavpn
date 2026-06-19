alter table nodes drop constraint if exists nodes_name_key;

create unique index if not exists nodes_name_active_key
  on nodes(name)
  where status <> 'retired';
