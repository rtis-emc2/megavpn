package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

const defaultRemoteAccessPoolKey = "remote_access_v4"
const importedRemoteAccessPoolKey = "imported_remote_access_v4"

func (s *Store) EnsureDefaultAddressPoolSpaces(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `insert into address_pool_spaces(
		key,label,description,family,base_cidr,start_cidr,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at
	) values (
		'remote_access_v4',
		'Remote Access IPv4',
		'Default IPv4 supernet for WireGuard, OpenVPN and L2TP client pools.',
		'ipv4',
		'172.16.0.0/12',
		'172.16.112.0/24',
		24,
		'remote_access',
		false,
		'active',
		10,
		now(),
		now()
	) on conflict(key) do nothing;
	insert into address_pool_spaces(
		key,label,description,family,base_cidr,start_cidr,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at
	) values (
		'imported_remote_access_v4',
		'Imported Remote Access IPv4',
		'Observed pools imported from existing instance revisions. The allocator does not use this space for new automatic allocations.',
		'ipv4',
		'10.0.0.0/8',
		'10.0.0.0/24',
		24,
		'imported',
		false,
		'active',
		900,
		now(),
		now()
	) on conflict(key) do nothing`)
	return err
}

func (s *Store) AddressPoolInventory(ctx context.Context) (domain.AddressPoolInventory, error) {
	if err := s.EnsureDefaultAddressPoolSpaces(ctx); err != nil {
		return domain.AddressPoolInventory{}, err
	}
	if err := s.backfillAddressPoolAllocationsFromInstances(ctx); err != nil && !isAddressPoolCatalogUnavailable(err) {
		return domain.AddressPoolInventory{}, err
	}
	spaces, err := s.listAddressPoolSpaces(ctx)
	if err != nil {
		return domain.AddressPoolInventory{}, err
	}
	allocations, err := s.ListAddressPoolAllocations(ctx)
	if err != nil {
		return domain.AddressPoolInventory{}, err
	}
	usedBySpace := map[string]int{}
	for _, allocation := range allocations {
		if allocation.Status == "reserved" || allocation.Status == "active" {
			usedBySpace[allocation.PoolSpaceID]++
		}
	}
	for i := range spaces {
		spaces[i].Capacity = addressPoolCapacity(spaces[i].BaseCIDR, spaces[i].StartCIDR, spaces[i].AllocationPrefix)
		spaces[i].Used = usedBySpace[spaces[i].ID]
		spaces[i].Free = spaces[i].Capacity - spaces[i].Used
		if spaces[i].Free < 0 {
			spaces[i].Free = 0
		}
	}
	return domain.AddressPoolInventory{Spaces: spaces, Allocations: allocations}, nil
}

func (s *Store) CreateAddressPoolSpace(ctx context.Context, space domain.AddressPoolSpace) (domain.AddressPoolSpace, error) {
	space.Key = strings.TrimSpace(space.Key)
	if space.Key == "" {
		space.Key = slugify(space.Label)
	}
	space.Label = strings.TrimSpace(space.Label)
	if space.Label == "" {
		return domain.AddressPoolSpace{}, fmt.Errorf("address pool label is required")
	}
	if strings.TrimSpace(space.Family) == "" {
		space.Family = "ipv4"
	}
	if strings.TrimSpace(space.ServiceScope) == "" {
		space.ServiceScope = "remote_access"
	}
	if strings.TrimSpace(space.Status) == "" {
		space.Status = "active"
	}
	if space.DisplayOrder == 0 {
		space.DisplayOrder = 1000
	}
	if err := validateAddressPoolSpaceCIDRs(space); err != nil {
		return domain.AddressPoolSpace{}, err
	}
	if space.AllocationPrefix <= 0 {
		space.AllocationPrefix = 24
	}
	row := s.db.QueryRow(ctx, `insert into address_pool_spaces(
		key,label,description,family,base_cidr,start_cidr,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at
	) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now(),now())
	returning id::text,key,label,description,family,base_cidr::text,start_cidr::text,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at`,
		space.Key,
		space.Label,
		space.Description,
		space.Family,
		space.BaseCIDR,
		space.StartCIDR,
		space.AllocationPrefix,
		space.ServiceScope,
		space.RoutingEnabled,
		space.Status,
		space.DisplayOrder,
	)
	created, err := scanAddressPoolSpace(row)
	if err != nil {
		return domain.AddressPoolSpace{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "address_pool.create", "address_pool", &created.ID, "address pool space created")
	return created, nil
}

func (s *Store) UpdateAddressPoolSpace(ctx context.Context, poolID string, patch domain.AddressPoolSpace) (domain.AddressPoolSpace, error) {
	poolID = strings.TrimSpace(poolID)
	if poolID == "" {
		return domain.AddressPoolSpace{}, fmt.Errorf("address pool id is required")
	}
	existing, err := s.getAddressPoolSpace(ctx, poolID)
	if err != nil {
		return domain.AddressPoolSpace{}, err
	}
	updated := existing
	if strings.TrimSpace(patch.Label) != "" {
		updated.Label = strings.TrimSpace(patch.Label)
	}
	updated.Description = strings.TrimSpace(patch.Description)
	if strings.TrimSpace(patch.Family) != "" {
		updated.Family = strings.TrimSpace(patch.Family)
	}
	if strings.TrimSpace(patch.BaseCIDR) != "" {
		updated.BaseCIDR = strings.TrimSpace(patch.BaseCIDR)
	}
	if strings.TrimSpace(patch.StartCIDR) != "" {
		updated.StartCIDR = strings.TrimSpace(patch.StartCIDR)
	}
	if patch.AllocationPrefix > 0 {
		updated.AllocationPrefix = patch.AllocationPrefix
	}
	if strings.TrimSpace(patch.ServiceScope) != "" {
		updated.ServiceScope = strings.TrimSpace(patch.ServiceScope)
	}
	if strings.TrimSpace(patch.Status) != "" {
		updated.Status = strings.TrimSpace(patch.Status)
	}
	if patch.DisplayOrder != 0 {
		updated.DisplayOrder = patch.DisplayOrder
	}
	updated.RoutingEnabled = patch.RoutingEnabled
	if err := validateAddressPoolSpaceCIDRs(updated); err != nil {
		return domain.AddressPoolSpace{}, err
	}
	activeAllocations, err := s.countActiveAddressPoolAllocations(ctx, existing.ID)
	if err != nil {
		return domain.AddressPoolSpace{}, err
	}
	structuralChange := existing.Family != updated.Family ||
		existing.BaseCIDR != updated.BaseCIDR ||
		existing.StartCIDR != updated.StartCIDR ||
		existing.AllocationPrefix != updated.AllocationPrefix ||
		existing.ServiceScope != updated.ServiceScope
	if activeAllocations > 0 && structuralChange {
		return domain.AddressPoolSpace{}, fmt.Errorf("address pool has %d active allocations; only label, description, status, routing and display order can be changed", activeAllocations)
	}
	row := s.db.QueryRow(ctx, `update address_pool_spaces
		set label=$2,description=$3,family=$4,base_cidr=$5,start_cidr=$6,allocation_prefix=$7,
		    service_scope=$8,routing_enabled=$9,status=$10,display_order=$11,updated_at=now()
		where id::text=$1 or key=$1
		returning id::text,key,label,description,family,base_cidr::text,start_cidr::text,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at`,
		poolID,
		updated.Label,
		updated.Description,
		updated.Family,
		updated.BaseCIDR,
		updated.StartCIDR,
		updated.AllocationPrefix,
		updated.ServiceScope,
		updated.RoutingEnabled,
		updated.Status,
		updated.DisplayOrder,
	)
	out, err := scanAddressPoolSpace(row)
	if err != nil {
		return domain.AddressPoolSpace{}, err
	}
	_, _ = s.db.Exec(ctx, `update address_pool_allocations set route_export=$2,updated_at=now() where pool_space_id=$1 and status in ('reserved','active')`, out.ID, out.RoutingEnabled)
	_, _ = s.CreateAudit(ctx, "system", "address_pool.update", "address_pool", &out.ID, "address pool space updated")
	return out, nil
}

func (s *Store) DeleteAddressPoolSpace(ctx context.Context, poolID string) (domain.AddressPoolSpace, error) {
	poolID = strings.TrimSpace(poolID)
	if poolID == "" {
		return domain.AddressPoolSpace{}, fmt.Errorf("address pool id is required")
	}
	existing, err := s.getAddressPoolSpace(ctx, poolID)
	if err != nil {
		return domain.AddressPoolSpace{}, err
	}
	activeAllocations, err := s.countActiveAddressPoolAllocations(ctx, existing.ID)
	if err != nil {
		return domain.AddressPoolSpace{}, err
	}
	if activeAllocations > 0 {
		return domain.AddressPoolSpace{}, fmt.Errorf("address pool has %d active allocations; delete dependent instances or release allocations first", activeAllocations)
	}
	row := s.db.QueryRow(ctx, `update address_pool_spaces
		set status='deleted',updated_at=now()
		where id::text=$1
		returning id::text,key,label,description,family,base_cidr::text,start_cidr::text,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at`, existing.ID)
	deleted, err := scanAddressPoolSpace(row)
	if err != nil {
		return domain.AddressPoolSpace{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "address_pool.delete", "address_pool", &deleted.ID, "address pool space deleted")
	return deleted, nil
}

func (s *Store) SetAddressPoolRouting(ctx context.Context, poolID string, enabled bool) (domain.AddressPoolSpace, error) {
	poolID = strings.TrimSpace(poolID)
	if poolID == "" {
		return domain.AddressPoolSpace{}, fmt.Errorf("address pool id is required")
	}
	row := s.db.QueryRow(ctx, `update address_pool_spaces
		set routing_enabled=$2,updated_at=now()
		where id::text=$1 or key=$1
		returning id::text,key,label,description,family,base_cidr::text,start_cidr::text,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at`,
		poolID, enabled)
	updated, err := scanAddressPoolSpace(row)
	if err != nil {
		return domain.AddressPoolSpace{}, err
	}
	_, _ = s.db.Exec(ctx, `update address_pool_allocations set route_export=$2,updated_at=now() where pool_space_id=$1 and status in ('reserved','active')`, updated.ID, enabled)
	_, _ = s.CreateAudit(ctx, "system", "address_pool.routing", "address_pool", &updated.ID, "address pool routing flag updated")
	return updated, nil
}

func (s *Store) getAddressPoolSpace(ctx context.Context, poolID string) (domain.AddressPoolSpace, error) {
	row := s.db.QueryRow(ctx, `select id::text,key,label,description,family,base_cidr::text,start_cidr::text,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at
		from address_pool_spaces
		where (id::text=$1 or key=$1) and status <> 'deleted'`, poolID)
	space, err := scanAddressPoolSpace(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.AddressPoolSpace{}, fmt.Errorf("address pool not found")
		}
		return domain.AddressPoolSpace{}, err
	}
	return space, nil
}

func (s *Store) countActiveAddressPoolAllocations(ctx context.Context, poolSpaceID string) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `select count(*) from address_pool_allocations where pool_space_id=$1 and status in ('reserved','active')`, poolSpaceID).Scan(&count)
	return count, err
}

func (s *Store) listAddressPoolSpaces(ctx context.Context) ([]domain.AddressPoolSpace, error) {
	rows, err := s.db.Query(ctx, `select id::text,key,label,description,family,base_cidr::text,start_cidr::text,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at
		from address_pool_spaces
		where status <> 'deleted'
		order by display_order asc,label asc,key asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AddressPoolSpace{}
	for rows.Next() {
		space, err := scanAddressPoolSpace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, space)
	}
	return out, rows.Err()
}

func (s *Store) ListAddressPoolAllocations(ctx context.Context) ([]domain.AddressPoolAllocation, error) {
	rows, err := s.db.Query(ctx, `select
		a.id::text,a.pool_space_id::text,p.key,p.label,a.cidr::text,coalesce(a.node_id::text,''),coalesce(n.name,''),
		coalesce(a.instance_id::text,''),coalesce(i.name,''),a.service_code,a.purpose,a.status,a.route_export,a.metadata_json,a.created_at,a.updated_at
	from address_pool_allocations a
	join address_pool_spaces p on p.id=a.pool_space_id
	left join nodes n on n.id=a.node_id
	left join instances i on i.id=a.instance_id
	where a.status <> 'released'
	order by p.display_order asc,a.cidr asc,a.created_at asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AddressPoolAllocation{}
	for rows.Next() {
		allocation, err := scanAddressPoolAllocation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, allocation)
	}
	return out, rows.Err()
}

func (s *Store) ensureInstanceAddressPoolAllocation(ctx context.Context, instance domain.Instance, purpose string) (domain.AddressPoolAllocation, bool, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		purpose = "remote_access"
	}
	existing, found, err := s.getInstanceAddressPoolAllocation(ctx, instance.ID, purpose)
	if err != nil {
		if isAddressPoolCatalogUnavailable(err) {
			return domain.AddressPoolAllocation{}, false, nil
		}
		return domain.AddressPoolAllocation{}, false, err
	}
	if found {
		return existing, true, nil
	}
	if err := s.EnsureDefaultAddressPoolSpaces(ctx); err != nil {
		if isAddressPoolCatalogUnavailable(err) {
			return domain.AddressPoolAllocation{}, false, nil
		}
		return domain.AddressPoolAllocation{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.AddressPoolAllocation{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var space domain.AddressPoolSpace
	if err := tx.QueryRow(ctx, `select id::text,key,label,description,family,base_cidr::text,start_cidr::text,allocation_prefix,service_scope,routing_enabled,status,display_order,created_at,updated_at
		from address_pool_spaces
		where status='active' and service_scope in ('remote_access','generic')
		order by case key when $1 then 0 else 1 end, display_order asc,label asc
		limit 1
		for update`, defaultRemoteAccessPoolKey).Scan(
		&space.ID,
		&space.Key,
		&space.Label,
		&space.Description,
		&space.Family,
		&space.BaseCIDR,
		&space.StartCIDR,
		&space.AllocationPrefix,
		&space.ServiceScope,
		&space.RoutingEnabled,
		&space.Status,
		&space.DisplayOrder,
		&space.CreatedAt,
		&space.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return domain.AddressPoolAllocation{}, false, fmt.Errorf("no active address pool space is available; add a new address pool supernet")
		}
		return domain.AddressPoolAllocation{}, false, err
	}
	usedRows, err := tx.Query(ctx, `select cidr::text from address_pool_allocations where pool_space_id=$1 and status in ('reserved','active')`, space.ID)
	if err != nil {
		return domain.AddressPoolAllocation{}, false, err
	}
	used := map[string]bool{}
	for usedRows.Next() {
		var cidr string
		if err := usedRows.Scan(&cidr); err != nil {
			usedRows.Close()
			return domain.AddressPoolAllocation{}, false, err
		}
		used[cidr] = true
	}
	usedRows.Close()
	if err := usedRows.Err(); err != nil {
		return domain.AddressPoolAllocation{}, false, err
	}
	cidr, err := nextAvailableIPv4Subnet(space.BaseCIDR, space.StartCIDR, space.AllocationPrefix, used)
	if err != nil {
		return domain.AddressPoolAllocation{}, false, err
	}
	allocationID := id.New()
	metadata := map[string]any{"allocated_by": "instance_planner"}
	row := tx.QueryRow(ctx, `insert into address_pool_allocations(
		id,pool_space_id,cidr,node_id,instance_id,service_code,purpose,status,route_export,metadata_json,created_at,updated_at
	) values($1,$2,$3,$4,$5,$6,$7,'active',$8,$9,now(),now())
	returning id::text,pool_space_id::text,cidr::text,coalesce(node_id::text,''),coalesce(instance_id::text,''),service_code,purpose,status,route_export,metadata_json,created_at,updated_at`,
		allocationID,
		space.ID,
		cidr.String(),
		nullIfEmpty(instance.NodeID),
		nullIfEmpty(instance.ID),
		instance.ServiceCode,
		purpose,
		space.RoutingEnabled,
		mustJSON(metadata),
	)
	var allocation domain.AddressPoolAllocation
	var nodeID, instanceID string
	var rawMeta []byte
	if err := row.Scan(
		&allocation.ID,
		&allocation.PoolSpaceID,
		&allocation.CIDR,
		&nodeID,
		&instanceID,
		&allocation.ServiceCode,
		&allocation.Purpose,
		&allocation.Status,
		&allocation.RouteExport,
		&rawMeta,
		&allocation.CreatedAt,
		&allocation.UpdatedAt,
	); err != nil {
		return domain.AddressPoolAllocation{}, false, err
	}
	allocation.PoolSpaceKey = space.Key
	allocation.PoolSpaceLabel = space.Label
	allocation.NodeID = stringPtrIfNotEmpty(nodeID)
	allocation.InstanceID = stringPtrIfNotEmpty(instanceID)
	_ = json.Unmarshal(rawMeta, &allocation.Metadata)
	if allocation.Metadata == nil {
		allocation.Metadata = map[string]any{}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AddressPoolAllocation{}, false, err
	}
	_, _ = s.CreateAudit(ctx, "system", "address_pool.allocate", "instance", &instance.ID, "address pool allocated")
	return allocation, true, nil
}

func (s *Store) getInstanceAddressPoolAllocation(ctx context.Context, instanceID, purpose string) (domain.AddressPoolAllocation, bool, error) {
	row := s.db.QueryRow(ctx, `select
		a.id::text,a.pool_space_id::text,p.key,p.label,a.cidr::text,coalesce(a.node_id::text,''),coalesce(n.name,''),
		coalesce(a.instance_id::text,''),coalesce(i.name,''),a.service_code,a.purpose,a.status,a.route_export,a.metadata_json,a.created_at,a.updated_at
	from address_pool_allocations a
	join address_pool_spaces p on p.id=a.pool_space_id
	left join nodes n on n.id=a.node_id
	left join instances i on i.id=a.instance_id
	where a.instance_id=$1 and a.purpose=$2 and a.status in ('reserved','active')
	order by a.created_at asc
	limit 1`, instanceID, purpose)
	allocation, err := scanAddressPoolAllocation(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.AddressPoolAllocation{}, false, nil
		}
		return domain.AddressPoolAllocation{}, false, err
	}
	return allocation, true, nil
}

func (s *Store) releaseInstanceAddressPoolAllocations(ctx context.Context, instanceID string) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return
	}
	err := releaseInstanceAddressPoolAllocationsTx(ctx, s.db, instanceID)
	if err != nil && !isAddressPoolCatalogUnavailable(err) {
		_, _ = s.CreateAudit(ctx, "system", "address_pool.release_failed", "instance", &instanceID, err.Error())
	}
}

func releaseInstanceAddressPoolAllocationsTx(ctx context.Context, exec interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, instanceID string) error {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil
	}
	_, err := exec.Exec(ctx, `update address_pool_allocations
		set status='released',updated_at=now()
		where instance_id=$1 and status in ('reserved','active')`, instanceID)
	return err
}

func (s *Store) backfillAddressPoolAllocationsFromInstances(ctx context.Context) error {
	var importedSpaceID string
	if err := s.db.QueryRow(ctx, `select id::text from address_pool_spaces where key=$1`, importedRemoteAccessPoolKey).Scan(&importedSpaceID); err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `select
		i.id::text,i.node_id::text,sd.code,i.name,
		coalesce((select spec_json from instance_revisions where instance_id=i.id order by revision_no desc limit 1), '{}'::jsonb)
	from instances i
	join service_definitions sd on sd.id=i.service_definition_id
	where i.status <> 'deleted'
	  and sd.code in ('wireguard','openvpn','xl2tpd')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var instanceID string
		var nodeID string
		var serviceCode string
		var instanceName string
		var rawSpec []byte
		if err := rows.Scan(&instanceID, &nodeID, &serviceCode, &instanceName, &rawSpec); err != nil {
			return err
		}
		spec := map[string]any{}
		_ = json.Unmarshal(rawSpec, &spec)
		cidr, purpose := observedPoolCIDRFromInstanceSpec(serviceCode, spec)
		if cidr == "" || purpose == "" {
			continue
		}
		metadata := map[string]any{"imported_from": "instance_revision", "instance_name": instanceName}
		_, err := s.db.Exec(ctx, `insert into address_pool_allocations(
			id,pool_space_id,cidr,node_id,instance_id,service_code,purpose,status,route_export,metadata_json,created_at,updated_at
		) values($1,$2,$3,$4,$5,$6,$7,'active',false,$8,now(),now())
		on conflict(instance_id,purpose) where instance_id is not null and status in ('reserved','active') do nothing`,
			id.New(),
			importedSpaceID,
			cidr,
			nullIfEmpty(nodeID),
			nullIfEmpty(instanceID),
			normalizeInstanceRuntimeCode(serviceCode),
			purpose,
			mustJSON(metadata),
		)
		if err != nil {
			return err
		}
	}
	return rows.Err()
}

func observedPoolCIDRFromInstanceSpec(serviceCode string, spec map[string]any) (string, string) {
	switch normalizeInstanceRuntimeCode(serviceCode) {
	case "wireguard":
		cidr := firstString(spec["network_cidr"], spec["address_pool_cidr"])
		if _, err := netip.ParsePrefix(cidr); err == nil {
			return cidr, "wireguard"
		}
	case "openvpn":
		cidr := firstString(spec["address_pool_cidr"])
		if _, err := netip.ParsePrefix(cidr); err == nil {
			return cidr, "openvpn"
		}
		cidr, err := cidrFromIPv4NetworkAndNetmask(firstString(spec["server_network"]), firstString(spec["server_netmask"]))
		if err == nil {
			return cidr, "openvpn"
		}
	case "xl2tpd":
		cidr := firstString(spec["address_pool_cidr"])
		if _, err := netip.ParsePrefix(cidr); err == nil {
			return cidr, "xl2tpd"
		}
		localIP := firstString(spec["local_ip"])
		if localIP == "" {
			return "", ""
		}
		addr, err := netip.ParseAddr(localIP)
		if err != nil || !addr.Is4() {
			return "", ""
		}
		network := alignIPv4ToPrefix(ipv4ToUint32(addr), 24)
		return netip.PrefixFrom(uint32ToIPv4(network), 24).String(), "xl2tpd"
	}
	return "", ""
}

type addressPoolScanner interface {
	Scan(dest ...any) error
}

func scanAddressPoolSpace(row addressPoolScanner) (domain.AddressPoolSpace, error) {
	var space domain.AddressPoolSpace
	if err := row.Scan(
		&space.ID,
		&space.Key,
		&space.Label,
		&space.Description,
		&space.Family,
		&space.BaseCIDR,
		&space.StartCIDR,
		&space.AllocationPrefix,
		&space.ServiceScope,
		&space.RoutingEnabled,
		&space.Status,
		&space.DisplayOrder,
		&space.CreatedAt,
		&space.UpdatedAt,
	); err != nil {
		return domain.AddressPoolSpace{}, err
	}
	return space, nil
}

func scanAddressPoolAllocation(row addressPoolScanner) (domain.AddressPoolAllocation, error) {
	var allocation domain.AddressPoolAllocation
	var nodeID string
	var instanceID string
	var rawMeta []byte
	if err := row.Scan(
		&allocation.ID,
		&allocation.PoolSpaceID,
		&allocation.PoolSpaceKey,
		&allocation.PoolSpaceLabel,
		&allocation.CIDR,
		&nodeID,
		&allocation.NodeName,
		&instanceID,
		&allocation.InstanceName,
		&allocation.ServiceCode,
		&allocation.Purpose,
		&allocation.Status,
		&allocation.RouteExport,
		&rawMeta,
		&allocation.CreatedAt,
		&allocation.UpdatedAt,
	); err != nil {
		return domain.AddressPoolAllocation{}, err
	}
	allocation.NodeID = stringPtrIfNotEmpty(nodeID)
	allocation.InstanceID = stringPtrIfNotEmpty(instanceID)
	_ = json.Unmarshal(rawMeta, &allocation.Metadata)
	if allocation.Metadata == nil {
		allocation.Metadata = map[string]any{}
	}
	return allocation, nil
}

func validateAddressPoolSpaceCIDRs(space domain.AddressPoolSpace) error {
	base, err := netip.ParsePrefix(strings.TrimSpace(space.BaseCIDR))
	if err != nil {
		return fmt.Errorf("base_cidr is invalid: %w", err)
	}
	start, err := netip.ParsePrefix(strings.TrimSpace(space.StartCIDR))
	if err != nil {
		return fmt.Errorf("start_cidr is invalid: %w", err)
	}
	if !base.Addr().Is4() || !start.Addr().Is4() {
		return fmt.Errorf("only IPv4 address pools are supported")
	}
	if !base.Contains(start.Masked().Addr()) {
		return fmt.Errorf("start_cidr must be inside base_cidr")
	}
	prefix := space.AllocationPrefix
	if prefix <= 0 {
		prefix = 24
	}
	if prefix < base.Bits() || prefix > 32 {
		return fmt.Errorf("allocation_prefix /%d must be between base prefix /%d and /32", prefix, base.Bits())
	}
	return nil
}

func addressPoolCapacity(baseCIDR, startCIDR string, allocationPrefix int) int {
	base, err := netip.ParsePrefix(baseCIDR)
	if err != nil || !base.Addr().Is4() {
		return 0
	}
	start, err := netip.ParsePrefix(startCIDR)
	if err != nil || !start.Addr().Is4() {
		return 0
	}
	if allocationPrefix < base.Bits() || allocationPrefix > 32 {
		return 0
	}
	block := uint32(1) << uint32(32-allocationPrefix)
	baseStart := ipv4ToUint32(base.Masked().Addr())
	baseEnd := uint32(uint64(baseStart) + (uint64(1) << uint32(32-base.Bits())) - 1)
	startValue := alignIPv4ToPrefix(ipv4ToUint32(start.Masked().Addr()), allocationPrefix)
	if startValue < baseStart {
		startValue = baseStart
	}
	if startValue > baseEnd {
		return 0
	}
	return int(((baseEnd - startValue) / block) + 1)
}

func nextAvailableIPv4Subnet(baseCIDR, startCIDR string, allocationPrefix int, used map[string]bool) (netip.Prefix, error) {
	base, err := netip.ParsePrefix(baseCIDR)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("base_cidr is invalid: %w", err)
	}
	start, err := netip.ParsePrefix(startCIDR)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("start_cidr is invalid: %w", err)
	}
	if !base.Addr().Is4() || !start.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("only IPv4 address pools are supported")
	}
	if allocationPrefix < base.Bits() || allocationPrefix > 32 {
		return netip.Prefix{}, fmt.Errorf("allocation prefix /%d is outside base %s", allocationPrefix, base.String())
	}
	block := uint32(1) << uint32(32-allocationPrefix)
	baseStart := ipv4ToUint32(base.Masked().Addr())
	baseEnd := uint32(uint64(baseStart) + (uint64(1) << uint32(32-base.Bits())) - 1)
	next := alignIPv4ToPrefix(ipv4ToUint32(start.Masked().Addr()), allocationPrefix)
	if next < baseStart {
		next = baseStart
	}
	for next <= baseEnd && next+block-1 <= baseEnd {
		candidate := netip.PrefixFrom(uint32ToIPv4(next), allocationPrefix).Masked()
		if !used[candidate.String()] {
			return candidate, nil
		}
		if math.MaxUint32-next < block {
			break
		}
		next += block
	}
	return netip.Prefix{}, fmt.Errorf("no free /%d subnet left in %s from %s; add a new address pool supernet", allocationPrefix, base.String(), start.String())
}

func firstUsableIPv4Address(prefix string, hostIndex uint32) (string, error) {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return "", err
	}
	if !p.Addr().Is4() {
		return "", fmt.Errorf("prefix must be IPv4")
	}
	if hostIndex == 0 {
		hostIndex = 1
	}
	base := ipv4ToUint32(p.Masked().Addr())
	addr := uint32ToIPv4(base + hostIndex)
	if !p.Contains(addr) {
		return "", fmt.Errorf("host index %d is outside prefix %s", hostIndex, p.String())
	}
	return fmt.Sprintf("%s/%d", addr.String(), p.Bits()), nil
}

func ipv4NetworkAndNetmask(prefix string) (string, string, error) {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return "", "", err
	}
	if !p.Addr().Is4() {
		return "", "", fmt.Errorf("prefix must be IPv4")
	}
	mask := uint32(0)
	if p.Bits() > 0 {
		mask = ^uint32(0) << uint32(32-p.Bits())
	}
	return p.Masked().Addr().String(), uint32ToIPv4(mask).String(), nil
}

func cidrFromIPv4NetworkAndNetmask(network string, netmask string) (string, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(network))
	if err != nil {
		return "", err
	}
	if !addr.Is4() {
		return "", fmt.Errorf("network must be IPv4")
	}
	maskAddr, err := netip.ParseAddr(strings.TrimSpace(netmask))
	if err != nil {
		return "", err
	}
	if !maskAddr.Is4() {
		return "", fmt.Errorf("netmask must be IPv4")
	}
	mask := ipv4ToUint32(maskAddr)
	bits := 0
	for i := 31; i >= 0; i-- {
		if mask&(1<<uint(i)) == 0 {
			break
		}
		bits++
	}
	prefix := netip.PrefixFrom(addr, bits).Masked()
	return prefix.String(), nil
}

func l2tpPoolFromPrefix(prefix string) (localIP, rangeStart, rangeEnd string, err error) {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return "", "", "", err
	}
	if !p.Addr().Is4() {
		return "", "", "", fmt.Errorf("prefix must be IPv4")
	}
	base := ipv4ToUint32(p.Masked().Addr())
	size := uint32(1) << uint32(32-p.Bits())
	if size < 32 {
		return "", "", "", fmt.Errorf("prefix %s is too small for L2TP pool", p.String())
	}
	localIP = uint32ToIPv4(base + 1).String()
	rangeStart = uint32ToIPv4(base + 10).String()
	endOffset := uint32(200)
	if endOffset >= size-1 {
		endOffset = size - 2
	}
	rangeEnd = uint32ToIPv4(base + endOffset).String()
	return localIP, rangeStart, rangeEnd, nil
}

func ipv4ToUint32(addr netip.Addr) uint32 {
	raw := addr.As4()
	return uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])
}

func uint32ToIPv4(value uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
}

func alignIPv4ToPrefix(value uint32, bits int) uint32 {
	if bits >= 32 {
		return value
	}
	mask := ^uint32(0) << uint32(32-bits)
	return value & mask
}

func stringPtrIfNotEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func isAddressPoolCatalogUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "42P01", "42703":
			return true
		}
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "address_pool_") && (strings.Contains(text, "does not exist") || strings.Contains(text, "undefined"))
}
