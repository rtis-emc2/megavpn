-- Release: 7.0.1.2
-- Centralized VLESS access group templates.

create table if not exists vless_group_templates (
  key text primary key,
  label text not null,
  description text not null default '',
  access_mode text not null default 'instance_default',
  egress_mode text not null default 'default',
  egress_node_id uuid null references nodes(id) on delete set null,
  target_instance_id uuid null references instances(id) on delete set null,
  outbound_tag text not null default 'direct',
  ad_block boolean not null default false,
  rules_json jsonb not null default '[]'::jsonb,
  extra_rules_json jsonb not null default '[]'::jsonb,
  status text not null default 'active',
  source text not null default 'operator',
  version integer not null default 1,
  display_order integer not null default 1000,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint vless_group_templates_key_check
    check (key ~ '^[a-z0-9][a-z0-9_.:-]{0,63}$'),
  constraint vless_group_templates_access_mode_check
    check (access_mode in ('instance_default','local_breakout','egress_node','instance_only','block')),
  constraint vless_group_templates_egress_mode_check
    check (egress_mode in ('default','local_breakout','egress_node','instance_only','block')),
  constraint vless_group_templates_outbound_tag_check
    check (outbound_tag ~ '^[A-Za-z0-9_.:-]{1,64}$'),
  constraint vless_group_templates_rules_array_check
    check (jsonb_typeof(rules_json) = 'array'),
  constraint vless_group_templates_extra_rules_array_check
    check (jsonb_typeof(extra_rules_json) = 'array'),
  constraint vless_group_templates_status_check
    check (status in ('active','disabled','deleted'))
);

create index if not exists vless_group_templates_status_order_idx
  on vless_group_templates(status, display_order, label, key);

insert into vless_group_templates(
  key,label,description,access_mode,egress_mode,outbound_tag,ad_block,rules_json,status,source,version,display_order
) values
  (
    'default',
    'Default access',
    'Use the VLESS instance default route. If the instance is configured for managed egress, this group follows that route.',
    'instance_default',
    'default',
    'direct',
    false,
    '[]'::jsonb,
    'active',
    'default',
    1,
    10
  ),
  (
    'current_node_exit',
    'Current node exit',
    'Force traffic to exit from the same node that accepts the VLESS connection.',
    'local_breakout',
    'local_breakout',
    'direct',
    false,
    '[]'::jsonb,
    'active',
    'default',
    1,
    20
  ),
  (
    'default_ads_blocked',
    'Default access with ad blocking',
    'Use the instance default route and block managed advertising domains before the final outbound rule.',
    'instance_default',
    'default',
    'direct',
    true,
    '[{"type":"field","domain":["geosite:category-ads-all"],"outbound_tag":"block"}]'::jsonb,
    'active',
    'default',
    1,
    30
  ),
  (
    'blocked',
    'Blocked',
    'Deny all traffic for clients assigned to this group.',
    'block',
    'block',
    'block',
    false,
    '[]'::jsonb,
    'active',
    'default',
    1,
    90
  )
on conflict(key) do nothing;
