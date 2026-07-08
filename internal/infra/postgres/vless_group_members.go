package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

const defaultVLESSGroupMembershipLimit = 50

func (s *Store) ListInstanceVLESSGroupMembers(ctx context.Context, instanceID string) (domain.VLESSGroupMembersOverview, error) {
	instance, groups, err := s.loadInstanceVLESSGroups(ctx, instanceID)
	if err != nil {
		return domain.VLESSGroupMembersOverview{}, err
	}
	counts, err := s.countInstanceVLESSGroupMembers(ctx, instance.ID)
	if err != nil {
		return domain.VLESSGroupMembersOverview{}, err
	}

	out := domain.VLESSGroupMembersOverview{
		InstanceID:   instance.ID,
		InstanceName: instance.Name,
		InstanceSlug: instance.Slug,
		ServiceCode:  instance.ServiceCode,
		Groups:       make([]domain.VLESSGroupMemberSummary, 0, len(groups)),
	}
	for _, group := range groups {
		summary := domain.VLESSGroupMemberSummary{
			Key:              group.Key,
			Label:            group.Label,
			Description:      group.Description,
			AccessMode:       group.AccessMode,
			EgressMode:       group.EgressMode,
			EgressNodeID:     group.EgressNodeID,
			TargetInstanceID: group.TargetID,
			OutboundTag:      group.OutboundTag,
			AdBlock:          group.AdBlock,
			Status:           firstString(group.Status, "active"),
		}
		if c, ok := counts[group.Key]; ok {
			summary.MemberCount = c.total
			summary.PendingCount = c.pending
			summary.ActiveCount = c.active
			summary.DisabledCount = c.disabled
		}
		out.Groups = append(out.Groups, summary)
		out.AvailableKeys = append(out.AvailableKeys, group.Key)
	}
	return out, nil
}

func (s *Store) ListInstanceVLESSGroupMemberClients(ctx context.Context, instanceID, groupKey, search, status string, limit, offset int) (domain.VLESSGroupMembersPage, error) {
	instance, groups, err := s.loadInstanceVLESSGroups(ctx, instanceID)
	if err != nil {
		return domain.VLESSGroupMembersPage{}, err
	}
	group, ok := groups[normalizeXrayVLESSGroupKey(groupKey)]
	if !ok {
		return domain.VLESSGroupMembersPage{}, unknownVLESSGroupError(groupKey, groups)
	}
	limit, offset = normalizeVLESSMembershipPage(limit, offset)
	items, total, err := s.queryInstanceVLESSGroupMemberClients(ctx, instance.ID, group.Key, search, status, limit, offset)
	if err != nil {
		return domain.VLESSGroupMembersPage{}, err
	}
	return domain.VLESSGroupMembersPage{
		InstanceID: instance.ID,
		GroupKey:   group.Key,
		Items:      items,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (s *Store) ListInstanceVLESSGroupAvailableClients(ctx context.Context, instanceID, search, assignment string, limit, offset int) (domain.VLESSGroupAvailableClientsPage, error) {
	instance, _, err := s.loadInstanceVLESSGroups(ctx, instanceID)
	if err != nil {
		return domain.VLESSGroupAvailableClientsPage{}, err
	}
	assignment = strings.ToLower(strings.TrimSpace(assignment))
	if assignment == "" {
		assignment = "unassigned"
	}
	if !in(assignment, "unassigned", "assigned", "all") {
		return domain.VLESSGroupAvailableClientsPage{}, fmt.Errorf("unsupported assignment filter %q", assignment)
	}
	limit, offset = normalizeVLESSMembershipPage(limit, offset)
	items, total, err := s.queryInstanceVLESSGroupAvailableClients(ctx, instance.ID, search, assignment, limit, offset)
	if err != nil {
		return domain.VLESSGroupAvailableClientsPage{}, err
	}
	return domain.VLESSGroupAvailableClientsPage{
		InstanceID: instance.ID,
		Assignment: assignment,
		Items:      items,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (s *Store) AddInstanceVLESSGroupMembers(ctx context.Context, instanceID, groupKey string, req domain.VLESSGroupMembershipRequest) (domain.VLESSGroupMembershipResult, error) {
	instance, groups, err := s.loadInstanceVLESSGroups(ctx, instanceID)
	if err != nil {
		return domain.VLESSGroupMembershipResult{}, err
	}
	group, ok := groups[normalizeXrayVLESSGroupKey(groupKey)]
	if !ok {
		return domain.VLESSGroupMembershipResult{}, unknownVLESSGroupError(groupKey, groups)
	}
	if !vlessGroupUsableForNewMembership(group) {
		return domain.VLESSGroupMembershipResult{}, fmt.Errorf("vless group %q is not active", group.Key)
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "add_or_move"
	}
	if !in(mode, "add_or_move", "add_only") {
		return domain.VLESSGroupMembershipResult{}, fmt.Errorf("unsupported membership mode %q", mode)
	}

	clientIDs, failures, err := s.resolveVLESSMembershipClientRefs(ctx, req.ClientIDs, req.ClientRefs)
	if err != nil {
		return domain.VLESSGroupMembershipResult{}, err
	}

	result := domain.VLESSGroupMembershipResult{
		InstanceID: instance.ID,
		GroupKey:   group.Key,
		Failed:     failures,
	}
	if len(clientIDs) == 0 {
		if len(failures) > 0 {
			result.Warnings = append(result.Warnings, "no matching clients were resolved")
			_, _ = s.CreateAudit(ctx, "system", "vless_group_members.bulk_add", "instance", &instance.ID, fmt.Sprintf("vless group %s membership failed: failed=%d", group.Key, len(result.Failed)))
			return result, nil
		}
		return domain.VLESSGroupMembershipResult{}, fmt.Errorf("at least one client is required")
	}
	for _, clientID := range clientIDs {
		changed, skipped, client, err := s.upsertInstanceVLESSGroupMember(ctx, instance.ID, clientID, group.Key, mode)
		if err != nil {
			result.Failed = append(result.Failed, domain.VLESSGroupMembershipFailure{ClientID: clientID, Error: err.Error()})
			continue
		}
		if skipped {
			result.Skipped++
		} else if changed == "created" {
			result.Created++
		} else if changed == "updated" {
			result.Updated++
		}
		if client.ClientID != "" {
			result.Clients = append(result.Clients, client)
		}
	}

	if req.BuildArtifacts {
		result.Warnings = append(result.Warnings, "artifact rebuild is not queued per-client from bulk VLESS membership; build configs or use subscriptions after instance apply")
	}
	if req.QueueApply || result.Created > 0 || result.Updated > 0 {
		if result.Created > 0 || result.Updated > 0 {
			apply, err := s.UpdateInstanceStatus(ctx, instance.ID, "apply")
			if err != nil {
				result.Warnings = append(result.Warnings, "instance apply was not queued: "+err.Error())
			} else {
				result.ApplyJobID = apply.ID
				_ = s.AddJobLog(ctx, apply.ID, "info", "vless group membership updated", map[string]any{
					"instance_id": instance.ID,
					"group_key":   group.Key,
					"created":     result.Created,
					"updated":     result.Updated,
					"skipped":     result.Skipped,
					"failed":      len(result.Failed),
				})
			}
		}
	}
	_, _ = s.CreateAudit(ctx, "system", "vless_group_members.bulk_add", "instance", &instance.ID, fmt.Sprintf("vless group %s membership updated: created=%d updated=%d skipped=%d failed=%d", group.Key, result.Created, result.Updated, result.Skipped, len(result.Failed)))
	return result, nil
}

func (s *Store) MoveInstanceVLESSGroupMember(ctx context.Context, instanceID, clientID, groupKey string) (domain.VLESSGroupMembershipResult, error) {
	req := domain.VLESSGroupMembershipRequest{
		ClientIDs:  []string{clientID},
		Mode:       "add_or_move",
		QueueApply: true,
	}
	result, err := s.AddInstanceVLESSGroupMembers(ctx, instanceID, groupKey, req)
	if err != nil {
		return result, err
	}
	_, _ = s.CreateAudit(ctx, "system", "vless_group_members.move", "instance", &result.InstanceID, fmt.Sprintf("client %s moved to vless group %s", strings.TrimSpace(clientID), result.GroupKey))
	return result, nil
}

func (s *Store) RemoveInstanceVLESSGroupMember(ctx context.Context, instanceID, clientID string) (domain.VLESSGroupMembershipResult, error) {
	instance, _, err := s.loadInstanceVLESSGroups(ctx, instanceID)
	if err != nil {
		return domain.VLESSGroupMembershipResult{}, err
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return domain.VLESSGroupMembershipResult{}, fmt.Errorf("client id is required")
	}

	existing, err := s.existingInstanceVLESSGroupMember(ctx, instance.ID, clientID)
	if err != nil {
		return domain.VLESSGroupMembershipResult{}, err
	}
	if existing.ServiceAccessID == "" {
		return domain.VLESSGroupMembershipResult{
			InstanceID: instance.ID,
			Skipped:    1,
			Warnings:   []string{"client did not have service access on this instance"},
		}, nil
	}
	cleanup, err := s.DeleteClientServiceAccess(ctx, clientID, existing.ServiceAccessID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.VLESSGroupMembershipResult{
			InstanceID: instance.ID,
			Skipped:    1,
			Warnings:   []string{"client did not have service access on this instance"},
		}, nil
	}
	if err != nil {
		return domain.VLESSGroupMembershipResult{}, err
	}

	result := domain.VLESSGroupMembershipResult{InstanceID: instance.ID, Removed: int(cleanup.ServiceAccessesDeleted)}
	if cleanup.InstanceApplyJobsQueued > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("instance apply jobs queued: %d", cleanup.InstanceApplyJobsQueued))
	}
	if len(cleanup.QueueErrors) > 0 {
		result.Warnings = append(result.Warnings, cleanup.QueueErrors...)
	}
	_, _ = s.CreateAudit(ctx, "system", "vless_group_members.remove", "instance", &instance.ID, "vless group member removed")
	return result, nil
}

type vlessGroupMemberCounts struct {
	total    int
	pending  int
	active   int
	disabled int
}

func (s *Store) countInstanceVLESSGroupMembers(ctx context.Context, instanceID string) (map[string]vlessGroupMemberCounts, error) {
	rows, err := s.db.Query(ctx, `select
			coalesce(nullif(metadata_json->>'vless_group',''), nullif(metadata_json->>'xray_group',''), nullif(metadata_json->>'outbound_group',''), nullif(metadata_json->'inbound_service'->>'vless_group',''), '') as group_key,
			count(*)::int,
			count(*) filter(where status='pending')::int,
			count(*) filter(where status='active')::int,
			count(*) filter(where status='disabled')::int
		from service_accesses
		where instance_id=$1
		  and status in ('pending','active','disabled')
		group by group_key`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]vlessGroupMemberCounts{}
	for rows.Next() {
		var key string
		var c vlessGroupMemberCounts
		if err := rows.Scan(&key, &c.total, &c.pending, &c.active, &c.disabled); err != nil {
			return nil, err
		}
		key = normalizeXrayVLESSGroupKey(key)
		if key != "" {
			out[key] = c
		}
	}
	return out, rows.Err()
}

func (s *Store) queryInstanceVLESSGroupMemberClients(ctx context.Context, instanceID, groupKey, search, status string, limit, offset int) ([]domain.VLESSGroupMemberClient, int, error) {
	where := []string{
		`sa.instance_id=$1`,
		`sa.status in ('pending','active','disabled')`,
		`coalesce(nullif(sa.metadata_json->>'vless_group',''), nullif(sa.metadata_json->>'xray_group',''), nullif(sa.metadata_json->>'outbound_group',''), nullif(sa.metadata_json->'inbound_service'->>'vless_group',''), '')=$2`,
	}
	args := []any{instanceID, groupKey}
	if q := strings.TrimSpace(search); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, fmt.Sprintf(`(lower(ca.username) like $%d or lower(coalesce(ca.display_name,'')) like $%d or lower(coalesce(ca.email,'')) like $%d or lower(ca.id::text) like $%d)`, len(args), len(args), len(args), len(args)))
	}
	if st := strings.ToLower(strings.TrimSpace(status)); st != "" && st != "all" {
		args = append(args, st)
		where = append(where, fmt.Sprintf(`(lower(ca.status)=$%d or lower(sa.status)=$%d)`, len(args), len(args)))
	}
	countSQL := `select count(*)::int
		from service_accesses sa
		join client_accounts ca on ca.id=sa.client_account_id
		where ` + strings.Join(where, " and ")
	var total int
	if err := s.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query(ctx, `select
			ca.id::text,
			ca.username,
			coalesce(ca.display_name,''),
			coalesce(ca.email,''),
			ca.status,
			sa.id::text,
			sa.status,
			coalesce(nullif(sa.metadata_json->>'vless_group',''), nullif(sa.metadata_json->>'xray_group',''), nullif(sa.metadata_json->>'outbound_group',''), nullif(sa.metadata_json->'inbound_service'->>'vless_group',''), ''),
			coalesce(nullif(sa.metadata_json->>'xray_uuid',''), nullif(sa.metadata_json->>'uuid',''), ''),
			sa.updated_at
		from service_accesses sa
		join client_accounts ca on ca.id=sa.client_account_id
		where `+strings.Join(where, " and ")+`
		order by ca.username asc, ca.created_at asc
		limit $`+fmt.Sprint(len(args)-1)+` offset $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanVLESSGroupMemberClientRows(rows)
	return items, total, err
}

func (s *Store) queryInstanceVLESSGroupAvailableClients(ctx context.Context, instanceID, search, assignment string, limit, offset int) ([]domain.VLESSGroupMemberClient, int, error) {
	where := []string{`ca.status <> 'deleted'`}
	args := []any{instanceID}
	switch assignment {
	case "unassigned":
		where = append(where, `sa.id is null`)
	case "assigned":
		where = append(where, `sa.id is not null`)
	}
	if q := strings.TrimSpace(search); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, fmt.Sprintf(`(lower(ca.username) like $%d or lower(coalesce(ca.display_name,'')) like $%d or lower(coalesce(ca.email,'')) like $%d or lower(ca.id::text) like $%d)`, len(args), len(args), len(args), len(args)))
	}
	base := ` from client_accounts ca
		left join service_accesses sa on sa.client_account_id=ca.id
			and sa.instance_id=$1
			and sa.status in ('pending','active','disabled')
		where ` + strings.Join(where, " and ")
	var total int
	if err := s.db.QueryRow(ctx, `select count(*)::int`+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query(ctx, `select
			ca.id::text,
			ca.username,
			coalesce(ca.display_name,''),
			coalesce(ca.email,''),
			ca.status,
			coalesce(sa.id::text,''),
			coalesce(sa.status,''),
			coalesce(nullif(sa.metadata_json->>'vless_group',''), nullif(sa.metadata_json->>'xray_group',''), nullif(sa.metadata_json->>'outbound_group',''), nullif(sa.metadata_json->'inbound_service'->>'vless_group',''), ''),
			coalesce(nullif(sa.metadata_json->>'xray_uuid',''), nullif(sa.metadata_json->>'uuid',''), ''),
			sa.updated_at
		`+base+`
		order by ca.username asc, ca.created_at asc
		limit $`+fmt.Sprint(len(args)-1)+` offset $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanVLESSGroupMemberClientRows(rows)
	return items, total, err
}

type vlessMembershipClientRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanVLESSGroupMemberClientRows(rows vlessMembershipClientRows) ([]domain.VLESSGroupMemberClient, error) {
	items := []domain.VLESSGroupMemberClient{}
	for rows.Next() {
		var item domain.VLESSGroupMemberClient
		if err := rows.Scan(
			&item.ClientID,
			&item.Username,
			&item.DisplayName,
			&item.Email,
			&item.ClientStatus,
			&item.ServiceAccessID,
			&item.AccessStatus,
			&item.GroupKey,
			&item.XrayUUID,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.GroupKey = normalizeXrayVLESSGroupKey(item.GroupKey)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) upsertInstanceVLESSGroupMember(ctx context.Context, instanceID, clientID, groupKey, mode string) (string, bool, domain.VLESSGroupMemberClient, error) {
	var client domain.Client
	err := s.db.QueryRow(ctx, `select id::text,username,coalesce(display_name,''),coalesce(email,''),status,coalesce(notes,''),expires_at,created_at,updated_at
		from client_accounts
		where id=$1`, clientID).Scan(&client.ID, &client.Username, &client.DisplayName, &client.Email, &client.Status, &client.Notes, &client.ExpiresAt, &client.CreatedAt, &client.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, domain.VLESSGroupMemberClient{}, fmt.Errorf("client not found")
	}
	if err != nil {
		return "", false, domain.VLESSGroupMemberClient{}, err
	}
	if strings.EqualFold(client.Status, "deleted") {
		return "", false, domain.VLESSGroupMemberClient{}, fmt.Errorf("client is deleted")
	}

	existing, err := s.existingInstanceVLESSGroupMember(ctx, instanceID, client.ID)
	if err != nil {
		return "", false, domain.VLESSGroupMemberClient{}, err
	}
	if existing.ServiceAccessID != "" {
		existingGroup := normalizeXrayVLESSGroupKey(existing.GroupKey)
		if existingGroup == groupKey && in(existing.AccessStatus, "pending", "active") {
			return "", true, existing, nil
		}
		if existingGroup != "" && existingGroup != groupKey && mode == "add_only" {
			return "", true, existing, nil
		}
	}

	metadata, err := s.clientProvisioningServiceMetadata(ctx, client.ID, instanceID, map[string]any{"vless_group": groupKey})
	if err != nil {
		return "", false, domain.VLESSGroupMemberClient{}, err
	}
	changeKind := "created"
	if existing.ServiceAccessID != "" {
		changeKind = "updated"
	}
	var accessID string
	err = s.db.QueryRow(ctx, `insert into service_accesses(id,client_account_id,instance_id,status,provision_mode,metadata_json,created_at,updated_at)
		values($1,$2,$3,'pending','bulk',$4,now(),now())
		on conflict(client_account_id, instance_id) do update set
			status='pending',
			provision_mode=case when service_accesses.provision_mode='' then 'bulk' else service_accesses.provision_mode end,
			metadata_json=service_accesses.metadata_json || excluded.metadata_json,
			updated_at=now()
		returning id::text`, id.New(), client.ID, instanceID, mustJSON(metadata)).Scan(&accessID)
	if err != nil {
		return "", false, domain.VLESSGroupMemberClient{}, err
	}
	if _, err := s.EnsureXrayServiceAccessUUID(ctx, accessID); err != nil {
		return "", false, domain.VLESSGroupMemberClient{}, err
	}
	if err := s.ensureBaselineClientAccessRoute(ctx, client.ID, accessID, instanceID); err != nil {
		return "", false, domain.VLESSGroupMemberClient{}, err
	}
	updated, err := s.existingInstanceVLESSGroupMember(ctx, instanceID, client.ID)
	if err != nil {
		return "", false, domain.VLESSGroupMemberClient{}, err
	}
	return changeKind, false, updated, nil
}

func (s *Store) existingInstanceVLESSGroupMember(ctx context.Context, instanceID, clientID string) (domain.VLESSGroupMemberClient, error) {
	var item domain.VLESSGroupMemberClient
	err := s.db.QueryRow(ctx, `select
			ca.id::text,
			ca.username,
			coalesce(ca.display_name,''),
			coalesce(ca.email,''),
			ca.status,
			sa.id::text,
			sa.status,
			coalesce(nullif(sa.metadata_json->>'vless_group',''), nullif(sa.metadata_json->>'xray_group',''), nullif(sa.metadata_json->>'outbound_group',''), nullif(sa.metadata_json->'inbound_service'->>'vless_group',''), ''),
			coalesce(nullif(sa.metadata_json->>'xray_uuid',''), nullif(sa.metadata_json->>'uuid',''), ''),
			sa.updated_at
		from service_accesses sa
		join client_accounts ca on ca.id=sa.client_account_id
		where sa.client_account_id=$1 and sa.instance_id=$2`, clientID, instanceID).Scan(
		&item.ClientID,
		&item.Username,
		&item.DisplayName,
		&item.Email,
		&item.ClientStatus,
		&item.ServiceAccessID,
		&item.AccessStatus,
		&item.GroupKey,
		&item.XrayUUID,
		&item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.VLESSGroupMemberClient{}, nil
	}
	if err != nil {
		return domain.VLESSGroupMemberClient{}, err
	}
	item.GroupKey = normalizeXrayVLESSGroupKey(item.GroupKey)
	return item, nil
}

func (s *Store) resolveVLESSMembershipClientRefs(ctx context.Context, clientIDs, refs []string) ([]string, []domain.VLESSGroupMembershipFailure, error) {
	seen := map[string]struct{}{}
	out := []string{}
	failures := []domain.VLESSGroupMembershipFailure{}
	add := func(clientID string) {
		clientID = strings.TrimSpace(clientID)
		if clientID == "" {
			return
		}
		if _, ok := seen[clientID]; ok {
			return
		}
		seen[clientID] = struct{}{}
		out = append(out, clientID)
	}
	for _, clientID := range clientIDs {
		add(clientID)
	}
	for _, raw := range refs {
		for _, ref := range splitVLESSClientRefs(raw) {
			clientID, err := s.resolveVLESSMembershipClientRef(ctx, ref)
			if err != nil {
				failures = append(failures, domain.VLESSGroupMembershipFailure{Ref: ref, Error: err.Error()})
				continue
			}
			add(clientID)
		}
	}
	return out, failures, nil
}

func splitVLESSClientRefs(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t' || r == ',' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if value := strings.TrimSpace(field); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func (s *Store) resolveVLESSMembershipClientRef(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty client reference")
	}
	rows, err := s.db.Query(ctx, `select id::text
		from client_accounts
		where status <> 'deleted'
		  and (id::text=$1 or lower(username)=lower($1) or lower(coalesce(email,''))=lower($1))
		order by case when id::text=$1 then 0 when lower(username)=lower($1) then 1 else 2 end
		limit 2`, ref)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var matches []string
	for rows.Next() {
		var clientID string
		if err := rows.Scan(&clientID); err != nil {
			return "", err
		}
		matches = append(matches, clientID)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("client not found")
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("client reference is ambiguous")
	}
	return matches[0], nil
}

func (s *Store) loadInstanceVLESSGroups(ctx context.Context, instanceID string) (domain.Instance, map[string]xrayVLESSGroup, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return domain.Instance{}, nil, fmt.Errorf("instance id is required")
	}
	instance, err := s.GetInstanceWithSpec(ctx, instanceID)
	if err != nil {
		return domain.Instance{}, nil, err
	}
	if normalizeCapabilityCode(instance.ServiceCode) != "xray-core" {
		return domain.Instance{}, nil, fmt.Errorf("service instance %s is not an Xray/VLESS instance", instance.ID)
	}
	if strings.EqualFold(strings.TrimSpace(instance.Status), "deleted") {
		return domain.Instance{}, nil, fmt.Errorf("service instance %s is deleted", instance.ID)
	}
	spec, err := s.ensureXrayProvisioningGroups(ctx, instance.ID, instance.Spec)
	if err != nil {
		return domain.Instance{}, nil, err
	}
	instance.Spec = spec
	groups := xrayVLESSGroups(spec)
	if len(groups) == 0 {
		return domain.Instance{}, nil, fmt.Errorf("service instance %s has no VLESS groups", instance.ID)
	}
	out := make(map[string]xrayVLESSGroup, len(groups))
	for _, group := range groups {
		if group.Key == "" {
			continue
		}
		if !vlessGroupUsableForCatalog(group) {
			continue
		}
		out[group.Key] = group
	}
	if len(out) == 0 {
		return domain.Instance{}, nil, fmt.Errorf("service instance %s has no active VLESS groups", instance.ID)
	}
	return instance, out, nil
}

func vlessGroupUsableForCatalog(group xrayVLESSGroup) bool {
	status := strings.ToLower(strings.TrimSpace(group.Status))
	return status == "" || status == "active"
}

func vlessGroupUsableForNewMembership(group xrayVLESSGroup) bool {
	return vlessGroupUsableForCatalog(group)
}

func unknownVLESSGroupError(groupKey string, groups map[string]xrayVLESSGroup) error {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return fmt.Errorf("vless group %q is not available for service instance; available groups: %s", normalizeXrayVLESSGroupKey(groupKey), strings.Join(keys, ", "))
}

func normalizeVLESSMembershipPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultVLESSGroupMembershipLimit
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
