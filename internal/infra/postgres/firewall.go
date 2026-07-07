package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

var firewallKeyRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{1,62}$`)
var firewallUUIDRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (s *Store) EnsureDefaultFirewallPolicies(ctx context.Context) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `insert into firewall_address_lists(id,key,label,description,scope,status,created_at,updated_at)
values
	($1,'trusted_control_plane','Trusted control plane','Control-plane hosts allowed to manage node agent and SSH access.','control_plane','active',now(),now()),
	($2,'trusted_operators','Trusted operators','Operator source networks for privileged access.','global','active',now(),now()),
	($3,'vpn_client_sources','VPN client source ranges','Default source ranges used by managed VPN clients and private overlay networks.','global','active',now(),now())
on conflict(key) do update set label=excluded.label, description=excluded.description, scope=excluded.scope, status=firewall_address_lists.status, updated_at=now()`,
		id.New(), id.New(), id.New()); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `with target as (
	select id from firewall_address_lists where key='vpn_client_sources' and status <> 'deleted'
), seed(value,value_type,label) as (
	values
		('10.0.0.0/8','cidr','RFC1918 private client range'),
		('172.16.0.0/12','cidr','RFC1918 private client range'),
		('192.168.0.0/16','cidr','RFC1918 private client range'),
		('100.64.0.0/10','cidr','CGNAT client range'),
		('fd00::/8','cidr','IPv6 unique local client range')
)
insert into firewall_address_entries(id,list_id,value,value_type,label,status,created_at,updated_at)
select gen_random_uuid(), target.id, seed.value, seed.value_type, seed.label, 'active', now(), now()
from target
cross join seed
on conflict(list_id,value) where status <> 'deleted' do nothing`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `insert into firewall_policies(id,key,label,description,scope,default_input_policy,default_forward_policy,default_output_policy,status,created_at,updated_at)
values
	($1,'control_plane_default','Control plane baseline','Baseline policy for control-plane host exposure.','control_plane','accept','accept','accept','active',now(),now()),
	($2,'node_base','Node baseline','Baseline node firewall policy. Default accept remains until explicit enforcement is enabled.','node','accept','accept','accept','active',now(),now())
on conflict(key) do nothing`, id.New(), id.New()); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `update firewall_policies
set label='Default node firewall',
	description='Default node firewall: deny unsolicited input and forwarding when strict mode is selected, keep node egress open, and allow common edge entrypoints plus managed VPN client forwarding ranges.',
	default_input_policy='drop',
	default_forward_policy='drop',
	default_output_policy='accept',
	status=case when status='deleted' then status else 'active' end,
	updated_at=now()
where key='node_base'`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `with policy as (
	select id from firewall_policies where key='node_base' and status <> 'deleted'
), trusted_operators as (
	select id from firewall_address_lists where key='trusted_operators' and status <> 'deleted'
), vpn_client_sources as (
	select id from firewall_address_lists where key='vpn_client_sources' and status <> 'deleted'
), seed(priority,chain,action,direction,protocol,src_list_key,dst_ports,state_match,comment,enabled,baseline_key) as (
	values
		(50,'input','drop','in','any','', '', ARRAY['invalid']::text[], 'Drop invalid input packets.', true, 'drop_invalid_input'),
		(55,'forward','drop','forward','any','', '', ARRAY['invalid']::text[], 'Drop invalid forwarded packets.', true, 'drop_invalid_forward'),
		(100,'input','accept','in','icmp','', '', ARRAY[]::text[], 'Allow IPv4 ICMP diagnostics.', true, 'allow_icmp_v4'),
		(105,'input','accept','in','icmpv6','', '', ARRAY[]::text[], 'Allow IPv6 ICMP diagnostics.', true, 'allow_icmp_v6'),
		(120,'input','accept','in','tcp','', '80,443', ARRAY['new','established']::text[], 'Allow public HTTP and HTTPS edge entrypoints.', true, 'allow_edge_http_https'),
		(200,'input','accept','in','tcp','trusted_operators', '22', ARRAY['new','established']::text[], 'Allow SSH management from trusted operators after the address list is populated.', false, 'allow_ssh_trusted_operators'),
		(300,'forward','accept','forward','any','vpn_client_sources', '', ARRAY['new','established']::text[], 'Allow managed VPN client source ranges to forward through the node.', true, 'allow_vpn_client_forward')
)
insert into firewall_rules(
	id,policy_id,priority,chain,action,direction,protocol,src_list_id,dst_list_id,src_cidr,dst_cidr,src_ports,dst_ports,state_match,comment,enabled,log,status,metadata_json,created_at,updated_at
)
select
	gen_random_uuid(), policy.id, seed.priority, seed.chain, seed.action, seed.direction, seed.protocol,
	case seed.src_list_key
		when 'trusted_operators' then trusted_operators.id
		when 'vpn_client_sources' then vpn_client_sources.id
		else null
	end,
	null, '', '', '', seed.dst_ports, seed.state_match, seed.comment, seed.enabled, false, 'active',
	jsonb_build_object('baseline','default_firewall','baseline_key',seed.baseline_key),
	now(), now()
from policy
cross join seed
left join trusted_operators on seed.src_list_key='trusted_operators'
left join vpn_client_sources on seed.src_list_key='vpn_client_sources'
where not exists (
	select 1
	from firewall_rules r
	where r.policy_id=policy.id
	  and r.status <> 'deleted'
	  and (r.metadata_json->>'baseline_key'=seed.baseline_key or r.comment=seed.comment)
)`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) FirewallInventory(ctx context.Context) (domain.FirewallInventory, error) {
	if err := s.EnsureDefaultFirewallPolicies(ctx); err != nil {
		return domain.FirewallInventory{}, err
	}
	lists, err := s.listFirewallAddressLists(ctx)
	if err != nil {
		return domain.FirewallInventory{}, err
	}
	entries, err := s.listFirewallAddressEntries(ctx)
	if err != nil {
		return domain.FirewallInventory{}, err
	}
	policies, err := s.listFirewallPolicies(ctx)
	if err != nil {
		return domain.FirewallInventory{}, err
	}
	rules, err := s.listFirewallRules(ctx, "")
	if err != nil {
		return domain.FirewallInventory{}, err
	}
	states, err := s.listFirewallNodeStates(ctx)
	if err != nil {
		return domain.FirewallInventory{}, err
	}
	return domain.FirewallInventory{
		AddressLists: lists,
		Entries:      entries,
		Policies:     policies,
		Rules:        rules,
		NodeStates:   states,
	}, nil
}

func (s *Store) CreateFirewallPolicy(ctx context.Context, input domain.FirewallPolicy) (domain.FirewallPolicy, error) {
	policy, err := normalizeFirewallPolicy(input, domain.FirewallPolicy{})
	if err != nil {
		return domain.FirewallPolicy{}, err
	}
	nodeID := nullableFirewallID(policy.NodeID)
	row := s.db.QueryRow(ctx, `with inserted as (
insert into firewall_policies(id,key,label,description,scope,node_id,default_input_policy,default_forward_policy,default_output_policy,status,created_at,updated_at)
values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now(),now())
returning id,key,label,description,scope,node_id,default_input_policy,default_forward_policy,default_output_policy,status,created_at,updated_at
)
select p.id,p.key,p.label,p.description,p.scope,coalesce(p.node_id::text,''),coalesce(n.name,''),p.default_input_policy,p.default_forward_policy,p.default_output_policy,p.status,0,p.created_at,p.updated_at
from inserted p
left join nodes n on n.id=p.node_id`,
		id.New(), policy.Key, policy.Label, policy.Description, policy.Scope, nodeID,
		policy.DefaultInputPolicy, policy.DefaultForwardPolicy, policy.DefaultOutputPolicy, policy.Status)
	created, err := scanFirewallPolicy(row)
	if err != nil {
		return domain.FirewallPolicy{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.policy.create", "firewall", &created.ID, "firewall policy created: "+created.Key)
	return created, nil
}

func (s *Store) UpdateFirewallPolicy(ctx context.Context, policyID string, input domain.FirewallPolicy) (domain.FirewallPolicy, error) {
	policyID = strings.TrimSpace(policyID)
	if policyID == "" {
		return domain.FirewallPolicy{}, fmt.Errorf("firewall policy id is required")
	}
	current, err := s.getFirewallPolicy(ctx, policyID)
	if err != nil {
		return domain.FirewallPolicy{}, err
	}
	policy, err := normalizeFirewallPolicy(input, current)
	if err != nil {
		return domain.FirewallPolicy{}, err
	}
	nodeID := nullableFirewallID(policy.NodeID)
	row := s.db.QueryRow(ctx, `with updated as (
update firewall_policies
set key=$2,label=$3,description=$4,scope=$5,node_id=$6,default_input_policy=$7,default_forward_policy=$8,default_output_policy=$9,status=$10,updated_at=now()
where id=$1 and status <> 'deleted'
returning id,key,label,description,scope,node_id,default_input_policy,default_forward_policy,default_output_policy,status,created_at,updated_at
)
select p.id,p.key,p.label,p.description,p.scope,coalesce(p.node_id::text,''),coalesce(n.name,''),p.default_input_policy,p.default_forward_policy,p.default_output_policy,p.status,
	(select count(*) from firewall_rules r where r.policy_id=p.id and r.status <> 'deleted')::int,
	p.created_at,p.updated_at
from updated p
left join nodes n on n.id=p.node_id`,
		current.ID, policy.Key, policy.Label, policy.Description, policy.Scope, nodeID,
		policy.DefaultInputPolicy, policy.DefaultForwardPolicy, policy.DefaultOutputPolicy, policy.Status)
	updated, err := scanFirewallPolicy(row)
	if err != nil {
		return domain.FirewallPolicy{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.policy.update", "firewall", &updated.ID, "firewall policy updated: "+updated.Key)
	return updated, nil
}

func (s *Store) DeleteFirewallPolicy(ctx context.Context, policyID string) (domain.FirewallPolicy, error) {
	policyID = strings.TrimSpace(policyID)
	if policyID == "" {
		return domain.FirewallPolicy{}, fmt.Errorf("firewall policy id is required")
	}
	current, err := s.getFirewallPolicy(ctx, policyID)
	if err != nil {
		return domain.FirewallPolicy{}, err
	}
	if in(current.Key, "control_plane_default", "node_base") {
		return domain.FirewallPolicy{}, fmt.Errorf("baseline firewall policy cannot be deleted")
	}
	var refs int
	if err := s.db.QueryRow(ctx, `select count(*)::int from firewall_node_state where policy_id=$1`, current.ID).Scan(&refs); err != nil {
		return domain.FirewallPolicy{}, err
	}
	if refs > 0 {
		return domain.FirewallPolicy{}, fmt.Errorf("firewall policy is assigned to %d node state record(s); apply another policy before deleting it", refs)
	}
	row := s.db.QueryRow(ctx, `with deleted_rules as (
	update firewall_rules set status='deleted',enabled=false,updated_at=now() where policy_id=$1 and status <> 'deleted'
), deleted_policy as (
	update firewall_policies
	set key=$2,status='deleted',updated_at=now()
	where id=$1 and status <> 'deleted'
	returning id,key,label,description,scope,node_id,default_input_policy,default_forward_policy,default_output_policy,status,created_at,updated_at
)
select p.id,p.key,p.label,p.description,p.scope,coalesce(p.node_id::text,''),coalesce(n.name,''),p.default_input_policy,p.default_forward_policy,p.default_output_policy,p.status,0,p.created_at,p.updated_at
from deleted_policy p
left join nodes n on n.id=p.node_id`, current.ID, archivedFirewallKey(current.Key, current.ID))
	deleted, err := scanFirewallPolicy(row)
	if err != nil {
		return domain.FirewallPolicy{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.policy.delete", "firewall", &deleted.ID, "firewall policy deleted: "+current.Key)
	return deleted, nil
}

func (s *Store) CreateFirewallAddressList(ctx context.Context, input domain.FirewallAddressList) (domain.FirewallAddressList, error) {
	input.Key = strings.ToLower(strings.TrimSpace(input.Key))
	input.Label = strings.TrimSpace(input.Label)
	input.Scope = normalizeFirewallScope(input.Scope, "global")
	input.Status = normalizeFirewallStatus(input.Status)
	if input.Key == "" {
		input.Key = slugifyFirewallKey(input.Label)
	}
	if !firewallKeyRE.MatchString(input.Key) {
		return domain.FirewallAddressList{}, fmt.Errorf("firewall address-list key must use lowercase letters, numbers, dot, dash or underscore")
	}
	if input.Label == "" {
		return domain.FirewallAddressList{}, fmt.Errorf("firewall address-list label is required")
	}
	row := s.db.QueryRow(ctx, `insert into firewall_address_lists(id,key,label,description,scope,status,created_at,updated_at)
values($1,$2,$3,$4,$5,$6,now(),now())
returning id,key,label,description,scope,status,0,created_at,updated_at`,
		id.New(), input.Key, input.Label, strings.TrimSpace(input.Description), input.Scope, input.Status)
	list, err := scanFirewallAddressList(row)
	if err != nil {
		return domain.FirewallAddressList{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.address_list.create", "firewall", &list.ID, "firewall address-list created: "+list.Key)
	return list, nil
}

func (s *Store) UpdateFirewallAddressList(ctx context.Context, listID string, input domain.FirewallAddressList) (domain.FirewallAddressList, error) {
	listID = strings.TrimSpace(listID)
	if listID == "" {
		return domain.FirewallAddressList{}, fmt.Errorf("firewall address-list id is required")
	}
	current, err := s.getFirewallAddressList(ctx, listID)
	if err != nil {
		return domain.FirewallAddressList{}, err
	}
	input.Key = strings.ToLower(strings.TrimSpace(firewallFirst(input.Key, current.Key)))
	input.Label = strings.TrimSpace(firewallFirst(input.Label, current.Label))
	input.Scope = normalizeFirewallScope(input.Scope, current.Scope)
	input.Status = normalizeFirewallStatus(firewallFirst(input.Status, current.Status))
	if !firewallKeyRE.MatchString(input.Key) {
		return domain.FirewallAddressList{}, fmt.Errorf("firewall address-list key must use lowercase letters, numbers, dot, dash or underscore")
	}
	if input.Label == "" {
		return domain.FirewallAddressList{}, fmt.Errorf("firewall address-list label is required")
	}
	row := s.db.QueryRow(ctx, `update firewall_address_lists
set key=$2,label=$3,description=$4,scope=$5,status=$6,updated_at=now()
where id=$1 and status <> 'deleted'
returning id,key,label,description,scope,status,
	(select count(*) from firewall_address_entries e where e.list_id=firewall_address_lists.id and e.status <> 'deleted')::int,
	created_at,updated_at`,
		current.ID, input.Key, input.Label, strings.TrimSpace(input.Description), input.Scope, input.Status)
	updated, err := scanFirewallAddressList(row)
	if err != nil {
		return domain.FirewallAddressList{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.address_list.update", "firewall", &updated.ID, "firewall address-list updated: "+updated.Key)
	return updated, nil
}

func (s *Store) DeleteFirewallAddressList(ctx context.Context, listID string) (domain.FirewallAddressList, error) {
	listID = strings.TrimSpace(listID)
	if listID == "" {
		return domain.FirewallAddressList{}, fmt.Errorf("firewall address-list id is required")
	}
	current, err := s.getFirewallAddressList(ctx, listID)
	if err != nil {
		return domain.FirewallAddressList{}, err
	}
	var refs int
	if err := s.db.QueryRow(ctx, `select count(*)::int from firewall_rules where status <> 'deleted' and (src_list_id=$1 or dst_list_id=$1)`, current.ID).Scan(&refs); err != nil {
		return domain.FirewallAddressList{}, err
	}
	if refs > 0 {
		return domain.FirewallAddressList{}, fmt.Errorf("firewall address-list is referenced by %d active rule(s)", refs)
	}
	row := s.db.QueryRow(ctx, `with deleted_entries as (
	update firewall_address_entries set status='deleted',updated_at=now() where list_id=$1 and status <> 'deleted'
), deleted_list as (
	update firewall_address_lists set key=$2,status='deleted',updated_at=now() where id=$1 and status <> 'deleted'
	returning id,key,label,description,scope,status,created_at,updated_at
)
select id,key,label,description,scope,status,0,created_at,updated_at from deleted_list`, current.ID, archivedFirewallKey(current.Key, current.ID))
	deleted, err := scanFirewallAddressList(row)
	if err != nil {
		return domain.FirewallAddressList{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.address_list.delete", "firewall", &deleted.ID, "firewall address-list deleted: "+deleted.Key)
	return deleted, nil
}

func (s *Store) CreateFirewallAddressEntry(ctx context.Context, listID string, input domain.FirewallAddressEntry) (domain.FirewallAddressEntry, error) {
	listID = strings.TrimSpace(listID)
	if listID == "" {
		return domain.FirewallAddressEntry{}, fmt.Errorf("firewall address-list id is required")
	}
	value, valueType, err := normalizeFirewallAddressEntry(input.Value, input.ValueType)
	if err != nil {
		return domain.FirewallAddressEntry{}, err
	}
	status := normalizeFirewallStatus(input.Status)
	row := s.db.QueryRow(ctx, `with inserted as (
insert into firewall_address_entries(id,list_id,value,value_type,label,status,created_at,updated_at)
values($1,$2,$3,$4,$5,$6,now(),now())
returning id,list_id,value,value_type,label,status,created_at,updated_at
)
select e.id,e.list_id::text,l.key,e.value,e.value_type,e.label,e.status,e.created_at,e.updated_at
from inserted e
join firewall_address_lists l on l.id=e.list_id`,
		id.New(), listID, value, valueType, strings.TrimSpace(input.Label), status)
	entry, err := scanFirewallAddressEntry(row)
	if err != nil {
		return domain.FirewallAddressEntry{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.address_entry.create", "firewall", &entry.ID, "firewall address entry created")
	return entry, nil
}

func (s *Store) UpdateFirewallAddressEntry(ctx context.Context, listID, entryID string, input domain.FirewallAddressEntry) (domain.FirewallAddressEntry, error) {
	listID = strings.TrimSpace(listID)
	entryID = strings.TrimSpace(entryID)
	if listID == "" || entryID == "" {
		return domain.FirewallAddressEntry{}, fmt.Errorf("firewall address-list id and entry id are required")
	}
	value, valueType, err := normalizeFirewallAddressEntry(input.Value, input.ValueType)
	if err != nil {
		return domain.FirewallAddressEntry{}, err
	}
	status := normalizeFirewallStatus(input.Status)
	row := s.db.QueryRow(ctx, `with updated as (
update firewall_address_entries
set value=$3,value_type=$4,label=$5,status=$6,updated_at=now()
where id=$2 and list_id=$1 and status <> 'deleted'
returning id,list_id,value,value_type,label,status,created_at,updated_at
)
select e.id,e.list_id::text,l.key,e.value,e.value_type,e.label,e.status,e.created_at,e.updated_at
from updated e
join firewall_address_lists l on l.id=e.list_id`,
		listID, entryID, value, valueType, strings.TrimSpace(input.Label), status)
	entry, err := scanFirewallAddressEntry(row)
	if err != nil {
		return domain.FirewallAddressEntry{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.address_entry.update", "firewall", &entry.ID, "firewall address entry updated")
	return entry, nil
}

func (s *Store) DeleteFirewallAddressEntry(ctx context.Context, listID, entryID string) (domain.FirewallAddressEntry, error) {
	listID = strings.TrimSpace(listID)
	entryID = strings.TrimSpace(entryID)
	if listID == "" || entryID == "" {
		return domain.FirewallAddressEntry{}, fmt.Errorf("firewall address-list id and entry id are required")
	}
	row := s.db.QueryRow(ctx, `with deleted as (
update firewall_address_entries
set status='deleted',updated_at=now()
where id=$2 and list_id=$1 and status <> 'deleted'
returning id,list_id,value,value_type,label,status,created_at,updated_at
)
select e.id,e.list_id::text,l.key,e.value,e.value_type,e.label,e.status,e.created_at,e.updated_at
from deleted e
join firewall_address_lists l on l.id=e.list_id`, listID, entryID)
	entry, err := scanFirewallAddressEntry(row)
	if err != nil {
		return domain.FirewallAddressEntry{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.address_entry.delete", "firewall", &entry.ID, "firewall address entry deleted")
	return entry, nil
}

func (s *Store) CreateFirewallRule(ctx context.Context, policyID string, input domain.FirewallRule) (domain.FirewallRule, error) {
	policyID = strings.TrimSpace(policyID)
	if policyID == "" {
		return domain.FirewallRule{}, fmt.Errorf("firewall policy id is required")
	}
	rule, err := normalizeFirewallRule(input)
	if err != nil {
		return domain.FirewallRule{}, err
	}
	srcListID := nullableFirewallID(rule.SrcListID)
	dstListID := nullableFirewallID(rule.DstListID)
	row := s.db.QueryRow(ctx, `with inserted as (
insert into firewall_rules(
	id,policy_id,priority,chain,action,direction,protocol,src_list_id,dst_list_id,src_cidr,dst_cidr,src_ports,dst_ports,state_match,comment,enabled,log,status,metadata_json,created_at,updated_at
) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,now(),now())
returning id,policy_id,priority,chain,action,direction,protocol,src_list_id,dst_list_id,src_cidr,dst_cidr,src_ports,dst_ports,state_match,comment,enabled,log,status,metadata_json,created_at,updated_at
)
select r.id,r.policy_id::text,r.priority,r.chain,r.action,r.direction,r.protocol,coalesce(r.src_list_id::text,''),coalesce(sl.key,''),coalesce(r.dst_list_id::text,''),coalesce(dl.key,''),r.src_cidr,r.dst_cidr,r.src_ports,r.dst_ports,r.state_match,r.comment,r.enabled,r.log,r.status,r.metadata_json,r.created_at,r.updated_at
from inserted r
left join firewall_address_lists sl on sl.id=r.src_list_id
left join firewall_address_lists dl on dl.id=r.dst_list_id`,
		id.New(), policyID, rule.Priority, rule.Chain, rule.Action, rule.Direction, rule.Protocol,
		srcListID, dstListID, rule.SrcCIDR, rule.DstCIDR, rule.SrcPorts, rule.DstPorts,
		rule.StateMatch, rule.Comment, rule.Enabled, rule.Log, rule.Status, mustJSON(rule.Metadata))
	created, err := scanFirewallRule(row)
	if err != nil {
		return domain.FirewallRule{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.rule.create", "firewall", &created.ID, "firewall rule created")
	return created, nil
}

func (s *Store) UpdateFirewallRule(ctx context.Context, policyID, ruleID string, input domain.FirewallRule) (domain.FirewallRule, error) {
	policyID = strings.TrimSpace(policyID)
	ruleID = strings.TrimSpace(ruleID)
	if policyID == "" || ruleID == "" {
		return domain.FirewallRule{}, fmt.Errorf("firewall policy id and rule id are required")
	}
	rule, err := normalizeFirewallRule(input)
	if err != nil {
		return domain.FirewallRule{}, err
	}
	srcListID := nullableFirewallID(rule.SrcListID)
	dstListID := nullableFirewallID(rule.DstListID)
	row := s.db.QueryRow(ctx, `with updated as (
update firewall_rules set
	priority=$3,chain=$4,action=$5,direction=$6,protocol=$7,src_list_id=$8,dst_list_id=$9,src_cidr=$10,dst_cidr=$11,
	src_ports=$12,dst_ports=$13,state_match=$14,comment=$15,enabled=$16,log=$17,status=$18,metadata_json=$19,updated_at=now()
where id=$2 and policy_id=$1 and status <> 'deleted'
returning id,policy_id,priority,chain,action,direction,protocol,src_list_id,dst_list_id,src_cidr,dst_cidr,src_ports,dst_ports,state_match,comment,enabled,log,status,metadata_json,created_at,updated_at
)
select r.id,r.policy_id::text,r.priority,r.chain,r.action,r.direction,r.protocol,coalesce(r.src_list_id::text,''),coalesce(sl.key,''),coalesce(r.dst_list_id::text,''),coalesce(dl.key,''),r.src_cidr,r.dst_cidr,r.src_ports,r.dst_ports,r.state_match,r.comment,r.enabled,r.log,r.status,r.metadata_json,r.created_at,r.updated_at
from updated r
left join firewall_address_lists sl on sl.id=r.src_list_id
left join firewall_address_lists dl on dl.id=r.dst_list_id`,
		policyID, ruleID, rule.Priority, rule.Chain, rule.Action, rule.Direction, rule.Protocol,
		srcListID, dstListID, rule.SrcCIDR, rule.DstCIDR, rule.SrcPorts, rule.DstPorts,
		rule.StateMatch, rule.Comment, rule.Enabled, rule.Log, rule.Status, mustJSON(rule.Metadata))
	updated, err := scanFirewallRule(row)
	if err != nil {
		return domain.FirewallRule{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.rule.update", "firewall", &updated.ID, "firewall rule updated")
	return updated, nil
}

func (s *Store) DeleteFirewallRule(ctx context.Context, policyID, ruleID string) (domain.FirewallRule, error) {
	policyID = strings.TrimSpace(policyID)
	ruleID = strings.TrimSpace(ruleID)
	if policyID == "" || ruleID == "" {
		return domain.FirewallRule{}, fmt.Errorf("firewall policy id and rule id are required")
	}
	row := s.db.QueryRow(ctx, `with deleted as (
update firewall_rules
set status='deleted',enabled=false,updated_at=now()
where id=$2 and policy_id=$1 and status <> 'deleted'
returning id,policy_id,priority,chain,action,direction,protocol,src_list_id,dst_list_id,src_cidr,dst_cidr,src_ports,dst_ports,state_match,comment,enabled,log,status,metadata_json,created_at,updated_at
)
select r.id,r.policy_id::text,r.priority,r.chain,r.action,r.direction,r.protocol,coalesce(r.src_list_id::text,''),coalesce(sl.key,''),coalesce(r.dst_list_id::text,''),coalesce(dl.key,''),r.src_cidr,r.dst_cidr,r.src_ports,r.dst_ports,r.state_match,r.comment,r.enabled,r.log,r.status,r.metadata_json,r.created_at,r.updated_at
from deleted r
left join firewall_address_lists sl on sl.id=r.src_list_id
left join firewall_address_lists dl on dl.id=r.dst_list_id`, policyID, ruleID)
	deleted, err := scanFirewallRule(row)
	if err != nil {
		return domain.FirewallRule{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.rule.delete", "firewall", &deleted.ID, "firewall rule deleted")
	return deleted, nil
}

type firewallJobPayload struct {
	policy      domain.FirewallPolicy
	revisionID  string
	revisionNo  int
	rulePayload []any
	payload     map[string]any
}

func (s *Store) buildFirewallJobPayload(ctx context.Context, nodeID, policyID string, enforceDefaultPolicy bool) (firewallJobPayload, error) {
	nodeID = strings.TrimSpace(nodeID)
	policyID = strings.TrimSpace(policyID)
	if nodeID == "" {
		return firewallJobPayload{}, fmt.Errorf("node id is required")
	}
	if policyID == "" {
		policy, err := s.defaultFirewallPolicyForNode(ctx, nodeID)
		if err != nil {
			return firewallJobPayload{}, err
		}
		policyID = policy.ID
	}
	policy, err := s.getFirewallPolicy(ctx, policyID)
	if err != nil {
		return firewallJobPayload{}, err
	}
	rules, err := s.listFirewallRules(ctx, policyID)
	if err != nil {
		return firewallJobPayload{}, err
	}
	rulePayload := firewallRulesPayload(rules)
	addressListPayload, err := s.firewallAddressListsPayload(ctx)
	if err != nil {
		return firewallJobPayload{}, err
	}
	revisionID := id.New()
	revisionNo, err := s.nextFirewallRevisionNo(ctx, policy.ID)
	if err != nil {
		return firewallJobPayload{}, err
	}
	payload := map[string]any{
		"node_id":                nodeID,
		"policy_id":              policy.ID,
		"policy_key":             policy.Key,
		"revision_id":            revisionID,
		"revision_no":            revisionNo,
		"default_input_policy":   policy.DefaultInputPolicy,
		"default_forward_policy": policy.DefaultForwardPolicy,
		"default_output_policy":  policy.DefaultOutputPolicy,
		"enforce_default_policy": enforceDefaultPolicy,
		"rules":                  rulePayload,
		"address_lists":          addressListPayload,
	}
	return firewallJobPayload{
		policy:      policy,
		revisionID:  revisionID,
		revisionNo:  revisionNo,
		rulePayload: rulePayload,
		payload:     payload,
	}, nil
}

func (s *Store) CreateFirewallPreviewJob(ctx context.Context, nodeID, policyID string, enforceDefaultPolicy bool) (domain.Job, error) {
	nodeID = strings.TrimSpace(nodeID)
	built, err := s.buildFirewallJobPayload(ctx, nodeID, policyID, enforceDefaultPolicy)
	if err != nil {
		return domain.Job{}, err
	}
	job, err := s.CreateJob(ctx, domain.Job{Type: "node.firewall.preview", ScopeType: "node", ScopeID: &nodeID, NodeID: &nodeID, Payload: built.payload})
	if err != nil {
		return domain.Job{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "firewall.preview", "node", &nodeID, "firewall preview queued")
	return job, nil
}

func (s *Store) CreateFirewallApplyJob(ctx context.Context, nodeID, policyID string, enforceDefaultPolicy bool) (domain.Job, error) {
	nodeID = strings.TrimSpace(nodeID)
	built, err := s.buildFirewallJobPayload(ctx, nodeID, policyID, enforceDefaultPolicy)
	if err != nil {
		return domain.Job{}, err
	}
	job, payloadJSON, err := normalizeJobForInsert(domain.Job{Type: "node.firewall.apply", ScopeType: "node", ScopeID: &nodeID, NodeID: &nodeID, Payload: built.payload})
	if err != nil {
		return domain.Job{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.Job{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `insert into firewall_revisions(id,policy_id,revision_no,rendered_hash,rules_json,status,created_at)
values($1,$2,$3,'',$4,'active',now())`, built.revisionID, built.policy.ID, built.revisionNo, mustJSON(built.rulePayload))
	if err != nil {
		return domain.Job{}, err
	}
	if err := insertJobRow(ctx, tx, job, payloadJSON); err != nil {
		return domain.Job{}, err
	}
	_, err = tx.Exec(ctx, `insert into firewall_node_state(id,node_id,policy_id,revision_id,desired_revision_id,status,observed_json,last_job_id,updated_at)
values($1,$2,$3,null,$4,'pending','{}'::jsonb,$5,now())
on conflict(node_id) do update set policy_id=excluded.policy_id, desired_revision_id=excluded.desired_revision_id, status='pending', last_job_id=excluded.last_job_id, updated_at=now()`,
		id.New(), nodeID, built.policy.ID, built.revisionID, job.ID)
	if err != nil {
		return domain.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Job{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "job.create", "job", &job.ID, "job queued")
	_, _ = s.CreateAudit(ctx, "system", "firewall.apply", "node", &nodeID, "firewall apply queued")
	return job, nil
}

func (s *Store) ApplyFirewallJobResult(ctx context.Context, job domain.Job, status string, result map[string]any) error {
	if strings.TrimSpace(job.Type) != "node.firewall.apply" {
		return nil
	}
	nodeID := ""
	if job.ScopeID != nil {
		nodeID = strings.TrimSpace(*job.ScopeID)
	}
	if nodeID == "" && job.NodeID != nil {
		nodeID = strings.TrimSpace(*job.NodeID)
	}
	if nodeID == "" {
		nodeID = strings.TrimSpace(stringify(job.Payload["node_id"]))
	}
	if nodeID == "" {
		return nil
	}
	policyID := strings.TrimSpace(stringify(job.Payload["policy_id"]))
	revisionID := strings.TrimSpace(stringify(job.Payload["revision_id"]))
	state := "failed"
	if status == "succeeded" {
		state = "applied"
	}
	_, err := s.db.Exec(ctx, `insert into firewall_node_state(id,node_id,policy_id,revision_id,desired_revision_id,status,observed_json,last_job_id,updated_at)
values($1,$2,$3,$4,$4,$5,$6,$7,now())
on conflict(node_id) do update set policy_id=excluded.policy_id, revision_id=case when $5='applied' then excluded.revision_id else firewall_node_state.revision_id end, desired_revision_id=excluded.desired_revision_id, status=excluded.status, observed_json=excluded.observed_json, last_job_id=excluded.last_job_id, updated_at=now()`,
		id.New(), nodeID, nullIfEmpty(policyID), nullIfEmpty(revisionID), state, mustJSON(result), job.ID)
	return err
}

func (s *Store) defaultFirewallPolicyForNode(ctx context.Context, nodeID string) (domain.FirewallPolicy, error) {
	row := s.db.QueryRow(ctx, `select p.id,p.key,p.label,p.description,p.scope,coalesce(p.node_id::text,''),coalesce(n.name,''),p.default_input_policy,p.default_forward_policy,p.default_output_policy,p.status,0,p.created_at,p.updated_at
from firewall_policies p
left join nodes n on n.id=p.node_id
where p.status='active' and (p.node_id=$1 or (p.node_id is null and p.key='node_base'))
order by case when p.node_id=$1 then 0 else 1 end, p.created_at asc
limit 1`, nodeID)
	return scanFirewallPolicy(row)
}

func (s *Store) getFirewallPolicy(ctx context.Context, policyID string) (domain.FirewallPolicy, error) {
	policyID = strings.TrimSpace(policyID)
	row := s.db.QueryRow(ctx, `select p.id,p.key,p.label,p.description,p.scope,coalesce(p.node_id::text,''),coalesce(n.name,''),p.default_input_policy,p.default_forward_policy,p.default_output_policy,p.status,
	(select count(*) from firewall_rules r where r.policy_id=p.id and r.status <> 'deleted')::int,
	p.created_at,p.updated_at
from firewall_policies p
left join nodes n on n.id=p.node_id
where ($2::uuid is not null and p.id=$2::uuid) or p.key=$1`, policyID, nullableFirewallLookupUUID(policyID))
	return scanFirewallPolicy(row)
}

func (s *Store) getFirewallAddressList(ctx context.Context, listID string) (domain.FirewallAddressList, error) {
	listID = strings.TrimSpace(listID)
	row := s.db.QueryRow(ctx, `select l.id,l.key,l.label,l.description,l.scope,l.status,
	(select count(*) from firewall_address_entries e where e.list_id=l.id and e.status <> 'deleted')::int,
	l.created_at,l.updated_at
from firewall_address_lists l
where (($2::uuid is not null and l.id=$2::uuid) or l.key=$1) and l.status <> 'deleted'`, listID, nullableFirewallLookupUUID(listID))
	return scanFirewallAddressList(row)
}

func (s *Store) nextFirewallRevisionNo(ctx context.Context, policyID string) (int, error) {
	var next int
	err := s.db.QueryRow(ctx, `select coalesce(max(revision_no),0)+1 from firewall_revisions where policy_id=$1`, policyID).Scan(&next)
	return next, err
}

func (s *Store) firewallAddressListsPayload(ctx context.Context) ([]any, error) {
	lists, err := s.listFirewallAddressLists(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.listFirewallAddressEntries(ctx)
	if err != nil {
		return nil, err
	}
	entriesByList := map[string][]domain.FirewallAddressEntry{}
	for _, entry := range entries {
		entriesByList[entry.ListID] = append(entriesByList[entry.ListID], entry)
	}
	out := make([]any, 0, len(lists))
	for _, list := range lists {
		entryPayload := make([]any, 0, len(entriesByList[list.ID]))
		for _, entry := range entriesByList[list.ID] {
			entryPayload = append(entryPayload, map[string]any{
				"id":         entry.ID,
				"value":      entry.Value,
				"value_type": entry.ValueType,
				"label":      entry.Label,
				"status":     entry.Status,
			})
		}
		out = append(out, map[string]any{
			"id":          list.ID,
			"key":         list.Key,
			"label":       list.Label,
			"description": list.Description,
			"scope":       list.Scope,
			"status":      list.Status,
			"entries":     entryPayload,
		})
	}
	return out, nil
}

func (s *Store) listFirewallAddressLists(ctx context.Context) ([]domain.FirewallAddressList, error) {
	rows, err := s.db.Query(ctx, `select l.id,l.key,l.label,l.description,l.scope,l.status,
	(select count(*) from firewall_address_entries e where e.list_id=l.id and e.status <> 'deleted')::int,
	l.created_at,l.updated_at
from firewall_address_lists l
where l.status <> 'deleted'
order by l.scope,l.key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FirewallAddressList{}
	for rows.Next() {
		item, err := scanFirewallAddressList(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) listFirewallAddressEntries(ctx context.Context) ([]domain.FirewallAddressEntry, error) {
	rows, err := s.db.Query(ctx, `select e.id,e.list_id::text,l.key,e.value,e.value_type,e.label,e.status,e.created_at,e.updated_at
from firewall_address_entries e
join firewall_address_lists l on l.id=e.list_id
where e.status <> 'deleted' and l.status <> 'deleted'
order by l.key,e.value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FirewallAddressEntry{}
	for rows.Next() {
		item, err := scanFirewallAddressEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) listFirewallPolicies(ctx context.Context) ([]domain.FirewallPolicy, error) {
	rows, err := s.db.Query(ctx, `select p.id,p.key,p.label,p.description,p.scope,coalesce(p.node_id::text,''),coalesce(n.name,''),p.default_input_policy,p.default_forward_policy,p.default_output_policy,p.status,
	(select count(*) from firewall_rules r where r.policy_id=p.id and r.status <> 'deleted')::int,
	p.created_at,p.updated_at
from firewall_policies p
left join nodes n on n.id=p.node_id
where p.status <> 'deleted'
order by p.scope,p.key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FirewallPolicy{}
	for rows.Next() {
		item, err := scanFirewallPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) listFirewallRules(ctx context.Context, policyID string) ([]domain.FirewallRule, error) {
	args := []any{}
	query := `select r.id,r.policy_id::text,r.priority,r.chain,r.action,r.direction,r.protocol,coalesce(r.src_list_id::text,''),coalesce(sl.key,''),coalesce(r.dst_list_id::text,''),coalesce(dl.key,''),r.src_cidr,r.dst_cidr,r.src_ports,r.dst_ports,r.state_match,r.comment,r.enabled,r.log,r.status,r.metadata_json,r.created_at,r.updated_at
from firewall_rules r
left join firewall_address_lists sl on sl.id=r.src_list_id
left join firewall_address_lists dl on dl.id=r.dst_list_id
where r.status <> 'deleted'`
	if strings.TrimSpace(policyID) != "" {
		query += ` and r.policy_id=$1`
		args = append(args, policyID)
	}
	query += ` order by r.policy_id,r.priority,r.created_at`
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FirewallRule{}
	for rows.Next() {
		item, err := scanFirewallRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) listFirewallNodeStates(ctx context.Context) ([]domain.FirewallNodeState, error) {
	rows, err := s.db.Query(ctx, `select s.id,s.node_id::text,n.name,coalesce(s.policy_id::text,''),coalesce(p.key,''),coalesce(s.revision_id::text,''),coalesce(s.desired_revision_id::text,''),s.status,s.observed_json,coalesce(s.last_job_id::text,''),s.updated_at
from firewall_node_state s
join nodes n on n.id=s.node_id
left join firewall_policies p on p.id=s.policy_id
where n.status <> 'retired'
order by n.name`)
	if err != nil {
		if isFirewallCatalogUnavailable(err) {
			return []domain.FirewallNodeState{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := []domain.FirewallNodeState{}
	for rows.Next() {
		item, err := scanFirewallNodeState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type firewallScanner interface {
	Scan(dest ...any) error
}

func scanFirewallAddressList(row firewallScanner) (domain.FirewallAddressList, error) {
	var item domain.FirewallAddressList
	err := row.Scan(&item.ID, &item.Key, &item.Label, &item.Description, &item.Scope, &item.Status, &item.EntryCount, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanFirewallAddressEntry(row firewallScanner) (domain.FirewallAddressEntry, error) {
	var item domain.FirewallAddressEntry
	err := row.Scan(&item.ID, &item.ListID, &item.ListKey, &item.Value, &item.ValueType, &item.Label, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanFirewallPolicy(row firewallScanner) (domain.FirewallPolicy, error) {
	var item domain.FirewallPolicy
	var nodeID string
	err := row.Scan(&item.ID, &item.Key, &item.Label, &item.Description, &item.Scope, &nodeID, &item.NodeName, &item.DefaultInputPolicy, &item.DefaultForwardPolicy, &item.DefaultOutputPolicy, &item.Status, &item.RuleCount, &item.CreatedAt, &item.UpdatedAt)
	if nodeID != "" {
		item.NodeID = &nodeID
	}
	return item, err
}

func scanFirewallRule(row firewallScanner) (domain.FirewallRule, error) {
	var item domain.FirewallRule
	var srcListID string
	var dstListID string
	var rawMeta []byte
	err := row.Scan(
		&item.ID, &item.PolicyID, &item.Priority, &item.Chain, &item.Action, &item.Direction, &item.Protocol,
		&srcListID, &item.SrcListKey, &dstListID, &item.DstListKey,
		&item.SrcCIDR, &item.DstCIDR, &item.SrcPorts, &item.DstPorts, &item.StateMatch, &item.Comment,
		&item.Enabled, &item.Log, &item.Status, &rawMeta, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return domain.FirewallRule{}, err
	}
	if srcListID != "" {
		item.SrcListID = &srcListID
	}
	if dstListID != "" {
		item.DstListID = &dstListID
	}
	if len(rawMeta) > 0 {
		_ = json.Unmarshal(rawMeta, &item.Metadata)
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return item, nil
}

func scanFirewallNodeState(row firewallScanner) (domain.FirewallNodeState, error) {
	var item domain.FirewallNodeState
	var policyID string
	var revisionID string
	var desiredRevisionID string
	var lastJobID string
	var rawObserved []byte
	err := row.Scan(&item.ID, &item.NodeID, &item.NodeName, &policyID, &item.PolicyKey, &revisionID, &desiredRevisionID, &item.Status, &rawObserved, &lastJobID, &item.UpdatedAt)
	if err != nil {
		return domain.FirewallNodeState{}, err
	}
	if policyID != "" {
		item.PolicyID = &policyID
	}
	if revisionID != "" {
		item.RevisionID = &revisionID
	}
	if desiredRevisionID != "" {
		item.DesiredRevisionID = &desiredRevisionID
	}
	if lastJobID != "" {
		item.LastJobID = &lastJobID
	}
	_ = json.Unmarshal(rawObserved, &item.Observed)
	if item.Observed == nil {
		item.Observed = map[string]any{}
	}
	return item, nil
}

func normalizeFirewallPolicy(input, current domain.FirewallPolicy) (domain.FirewallPolicy, error) {
	policy := input
	policy.Key = strings.ToLower(strings.TrimSpace(firewallFirst(policy.Key, current.Key)))
	policy.Label = strings.TrimSpace(firewallFirst(policy.Label, current.Label))
	policy.Description = strings.TrimSpace(policy.Description)
	policy.Scope = normalizeFirewallPolicyScope(firewallFirst(policy.Scope, current.Scope))
	var err error
	policy.DefaultInputPolicy, err = normalizeFirewallDefaultPolicy(firewallFirst(policy.DefaultInputPolicy, current.DefaultInputPolicy, "accept"))
	if err != nil {
		return domain.FirewallPolicy{}, fmt.Errorf("default input policy: %w", err)
	}
	policy.DefaultForwardPolicy, err = normalizeFirewallDefaultPolicy(firewallFirst(policy.DefaultForwardPolicy, current.DefaultForwardPolicy, "accept"))
	if err != nil {
		return domain.FirewallPolicy{}, fmt.Errorf("default forward policy: %w", err)
	}
	policy.DefaultOutputPolicy, err = normalizeFirewallDefaultPolicy(firewallFirst(policy.DefaultOutputPolicy, current.DefaultOutputPolicy, "accept"))
	if err != nil {
		return domain.FirewallPolicy{}, fmt.Errorf("default output policy: %w", err)
	}
	policy.Status = normalizeFirewallStatus(firewallFirst(policy.Status, current.Status))
	if policy.Label == "" {
		return domain.FirewallPolicy{}, fmt.Errorf("firewall policy label is required")
	}
	if policy.Key == "" {
		policy.Key = slugifyFirewallKey(policy.Label)
	}
	if !firewallKeyRE.MatchString(policy.Key) {
		return domain.FirewallPolicy{}, fmt.Errorf("firewall policy key must use lowercase letters, numbers, dot, dash or underscore")
	}
	if policy.NodeID != nil {
		nodeID := strings.TrimSpace(*policy.NodeID)
		if nodeID == "" || policy.Scope != "node" {
			policy.NodeID = nil
		} else {
			policy.NodeID = &nodeID
		}
	}
	return policy, nil
}

func normalizeFirewallRule(input domain.FirewallRule) (domain.FirewallRule, error) {
	rule := input
	if input.Status == "" && !input.Enabled {
		rule.Enabled = true
	}
	if rule.Priority <= 0 {
		rule.Priority = 1000
	}
	if rule.Priority > 65000 {
		return domain.FirewallRule{}, fmt.Errorf("firewall priority must be between 1 and 65000")
	}
	rule.Chain = strings.ToLower(strings.TrimSpace(rule.Chain))
	if rule.Chain == "" {
		rule.Chain = "input"
	}
	if !in(rule.Chain, "input", "forward", "output") {
		return domain.FirewallRule{}, fmt.Errorf("firewall chain must be input, forward or output")
	}
	rule.Action = strings.ToLower(strings.TrimSpace(rule.Action))
	if !in(rule.Action, "accept", "drop", "reject") {
		return domain.FirewallRule{}, fmt.Errorf("firewall action must be accept, drop or reject")
	}
	rule.Direction = strings.ToLower(strings.TrimSpace(rule.Direction))
	if rule.Direction == "" {
		rule.Direction = directionFromFirewallChain(rule.Chain)
	}
	if !in(rule.Direction, "in", "out", "forward") {
		return domain.FirewallRule{}, fmt.Errorf("firewall direction must be in, out or forward")
	}
	rule.Protocol = strings.ToLower(strings.TrimSpace(rule.Protocol))
	if rule.Protocol == "" {
		rule.Protocol = "any"
	}
	if !in(rule.Protocol, "any", "tcp", "udp", "icmp", "icmpv6") {
		return domain.FirewallRule{}, fmt.Errorf("firewall protocol must be any, tcp, udp, icmp or icmpv6")
	}
	var err error
	rule.SrcCIDR, err = normalizeFirewallCIDR(rule.SrcCIDR)
	if err != nil {
		return domain.FirewallRule{}, fmt.Errorf("source CIDR: %w", err)
	}
	rule.DstCIDR, err = normalizeFirewallCIDR(rule.DstCIDR)
	if err != nil {
		return domain.FirewallRule{}, fmt.Errorf("destination CIDR: %w", err)
	}
	rule.SrcPorts, err = normalizeFirewallPorts(rule.SrcPorts)
	if err != nil {
		return domain.FirewallRule{}, fmt.Errorf("source ports: %w", err)
	}
	rule.DstPorts, err = normalizeFirewallPorts(rule.DstPorts)
	if err != nil {
		return domain.FirewallRule{}, fmt.Errorf("destination ports: %w", err)
	}
	if (rule.SrcPorts != "" || rule.DstPorts != "") && rule.Protocol != "tcp" && rule.Protocol != "udp" {
		return domain.FirewallRule{}, fmt.Errorf("ports require tcp or udp protocol")
	}
	rule.StateMatch = normalizeFirewallStateMatch(rule.StateMatch)
	rule.Comment = strings.TrimSpace(rule.Comment)
	rule.Status = normalizeFirewallStatus(rule.Status)
	if input.Metadata == nil {
		rule.Metadata = map[string]any{}
	}
	return rule, nil
}

func firewallRulesPayload(rules []domain.FirewallRule) []any {
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority == rules[j].Priority {
			return rules[i].ID < rules[j].ID
		}
		return rules[i].Priority < rules[j].Priority
	})
	out := make([]any, 0, len(rules))
	for _, rule := range rules {
		if rule.Status == "deleted" {
			continue
		}
		out = append(out, map[string]any{
			"id":           rule.ID,
			"priority":     rule.Priority,
			"chain":        rule.Chain,
			"action":       rule.Action,
			"direction":    rule.Direction,
			"protocol":     rule.Protocol,
			"src_list_id":  ruleStringPtr(rule.SrcListID),
			"src_list_key": rule.SrcListKey,
			"dst_list_id":  ruleStringPtr(rule.DstListID),
			"dst_list_key": rule.DstListKey,
			"src_cidr":     rule.SrcCIDR,
			"dst_cidr":     rule.DstCIDR,
			"src_ports":    rule.SrcPorts,
			"dst_ports":    rule.DstPorts,
			"state_match":  rule.StateMatch,
			"comment":      rule.Comment,
			"enabled":      rule.Enabled,
			"log":          rule.Log,
			"status":       rule.Status,
		})
	}
	return out
}

func firewallFirst(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func ruleStringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func normalizeFirewallScope(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if in(value, "global", "control_plane", "node", "service", "client", "template") {
		return value
	}
	return fallback
}

func normalizeFirewallPolicyScope(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if in(value, "control_plane", "node", "template") {
		return value
	}
	return "node"
}

func normalizeFirewallDefaultPolicy(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if in(value, "accept", "drop", "reject") {
		return value, nil
	}
	return "", fmt.Errorf("must be accept, drop or reject")
}

func normalizeFirewallStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if in(value, "active", "disabled", "deleted") {
		return value
	}
	return "active"
}

func normalizeFirewallAddressEntry(value, valueType string) (string, string, error) {
	value = strings.TrimSpace(value)
	valueType = strings.ToLower(strings.TrimSpace(valueType))
	if value == "" {
		return "", "", fmt.Errorf("address value is required")
	}
	if valueType == "" {
		if strings.Contains(value, "/") {
			valueType = "cidr"
		} else if _, err := netip.ParseAddr(value); err == nil {
			valueType = "address"
		} else {
			valueType = "dns"
		}
	}
	switch valueType {
	case "cidr":
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return "", "", fmt.Errorf("CIDR is invalid")
		}
		return prefix.Masked().String(), "cidr", nil
	case "address":
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return "", "", fmt.Errorf("address is invalid")
		}
		return addr.String(), "address", nil
	case "range":
		left, right, ok := strings.Cut(value, "-")
		if !ok {
			return "", "", fmt.Errorf("address range must use start-end format")
		}
		start, err := netip.ParseAddr(strings.TrimSpace(left))
		if err != nil {
			return "", "", fmt.Errorf("range start is invalid")
		}
		end, err := netip.ParseAddr(strings.TrimSpace(right))
		if err != nil {
			return "", "", fmt.Errorf("range end is invalid")
		}
		return start.String() + "-" + end.String(), "range", nil
	case "dns":
		if strings.ContainsAny(value, " \t\r\n;{}") || len(value) > 253 {
			return "", "", fmt.Errorf("DNS value contains unsupported characters")
		}
		return strings.ToLower(value), "dns", nil
	default:
		return "", "", fmt.Errorf("address value type must be cidr, address, range or dns")
	}
}

func normalizeFirewallCIDR(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "any" || value == "*" {
		return "", nil
	}
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return prefix.Masked().String(), nil
	}
	if addr, err := netip.ParseAddr(value); err == nil {
		if addr.Is4() {
			return addr.String() + "/32", nil
		}
		return addr.String() + "/128", nil
	}
	return "", fmt.Errorf("must be a valid IP address or CIDR")
}

func normalizeFirewallPorts(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "any" || value == "*" {
		return "", nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			left, right, _ := strings.Cut(part, "-")
			from, err := parseFirewallPort(left)
			if err != nil {
				return "", err
			}
			to, err := parseFirewallPort(right)
			if err != nil {
				return "", err
			}
			if from > to {
				return "", fmt.Errorf("port range %q is reversed", part)
			}
			out = append(out, strconv.Itoa(from)+"-"+strconv.Itoa(to))
			continue
		}
		port, err := parseFirewallPort(part)
		if err != nil {
			return "", err
		}
		out = append(out, strconv.Itoa(port))
	}
	return strings.Join(out, ","), nil
}

func parseFirewallPort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("port %q must be between 1 and 65535", value)
	}
	return port, nil
}

func normalizeFirewallStateMatch(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if !in(value, "new", "established", "related", "invalid") || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func directionFromFirewallChain(chain string) string {
	switch chain {
	case "output":
		return "out"
	case "forward":
		return "forward"
	default:
		return "in"
	}
}

func nullableFirewallID(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func nullableFirewallLookupUUID(value string) any {
	value = strings.TrimSpace(value)
	if value == "" || !firewallUUIDRE.MatchString(value) {
		return nil
	}
	return value
}

func archivedFirewallKey(key, idValue string) string {
	key = strings.TrimSpace(key)
	idValue = strings.ReplaceAll(strings.TrimSpace(idValue), "-", "")
	if idValue == "" {
		idValue = "deleted"
	}
	suffix := "_deleted_" + idValue
	if len(suffix) > 24 {
		suffix = suffix[:24]
	}
	limit := 63 - len(suffix)
	if limit < 1 {
		limit = 1
	}
	if len(key) > limit {
		key = key[:limit]
	}
	key = strings.Trim(key, "_.-")
	if key == "" {
		key = "deleted"
	}
	return key + suffix
}

func slugifyFirewallKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastSep := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastSep = false
			continue
		}
		if !lastSep {
			b.WriteByte('_')
			lastSep = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if len(out) > 63 {
		out = out[:63]
	}
	return out
}

func isFirewallCatalogUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "firewall_") && strings.Contains(strings.ToLower(err.Error()), "does not exist")
}
