package postgres

import (
	"context"
	"encoding/json"
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
	items, total, err := s.queryGlobalVLESSGroupAvailableClients(ctx, search, assignment, limit, offset)
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

func (s *Store) ListVLESSGroupMembers(ctx context.Context, groupKey, search, status string, limit, offset int) (domain.VLESSGroupMembersPage, error) {
	group, err := s.getActiveVLESSGroupTemplate(ctx, groupKey)
	if err != nil {
		return domain.VLESSGroupMembersPage{}, err
	}
	accessGroup, err := s.ensureClientAccessGroupFromVLESSTemplate(ctx, group)
	if err != nil {
		return domain.VLESSGroupMembersPage{}, err
	}
	page, err := s.ListClientAccessGroupMembers(ctx, accessGroup.ID, search, status, limit, offset)
	if err != nil {
		return domain.VLESSGroupMembersPage{}, err
	}
	return domain.VLESSGroupMembersPage{
		GroupKey: group.Key,
		Items:    clientAccessGroupMembersToVLESS(page.Items),
		Total:    page.Total,
		Limit:    page.Limit,
		Offset:   page.Offset,
	}, nil
}

func (s *Store) ListVLESSGroupAvailableClients(ctx context.Context, search, assignment string, limit, offset int) (domain.VLESSGroupAvailableClientsPage, error) {
	assignment = strings.ToLower(strings.TrimSpace(assignment))
	if assignment == "" {
		assignment = "unassigned"
	}
	if !in(assignment, "unassigned", "assigned", "all") {
		return domain.VLESSGroupAvailableClientsPage{}, fmt.Errorf("unsupported assignment filter %q", assignment)
	}
	page, err := s.ListClientAccessGroupAvailableClients(ctx, "vless", search, assignment, "", limit, offset)
	if err != nil {
		return domain.VLESSGroupAvailableClientsPage{}, err
	}
	return domain.VLESSGroupAvailableClientsPage{
		Assignment: assignment,
		Items:      clientAccessGroupMembersToVLESS(page.Items),
		Total:      page.Total,
		Limit:      page.Limit,
		Offset:     page.Offset,
	}, nil
}

func (s *Store) AddVLESSGroupMembers(ctx context.Context, groupKey string, req domain.VLESSGroupMembershipRequest) (domain.VLESSGroupMembershipResult, error) {
	group, err := s.getActiveVLESSGroupTemplate(ctx, groupKey)
	if err != nil {
		return domain.VLESSGroupMembershipResult{}, err
	}
	accessGroup, err := s.ensureClientAccessGroupFromVLESSTemplate(ctx, group)
	if err != nil {
		return domain.VLESSGroupMembershipResult{}, err
	}
	genericReq := domain.ClientAccessGroupMembershipRequest{
		ClientIDs:        req.ClientIDs,
		ClientRefs:       req.ClientRefs,
		Mode:             firstString(req.Mode, "add_or_move"),
		QueueApply:       req.QueueApply,
		BuildArtifacts:   req.BuildArtifacts,
		DryRun:           req.DryRun,
		AllFiltered:      req.AllFiltered,
		FilterSearch:     req.FilterSearch,
		FilterAssignment: req.FilterAssignment,
		FilterStatus:     req.FilterStatus,
	}
	if req.DryRun {
		result, err := s.PreviewClientAccessGroupMembers(ctx, accessGroup.ID, genericReq)
		if err != nil {
			return domain.VLESSGroupMembershipResult{}, err
		}
		return clientAccessGroupResultToVLESS(result), nil
	}
	var result domain.ClientAccessGroupMembershipResult
	if strings.EqualFold(strings.TrimSpace(genericReq.Mode), "add_only") {
		result, err = s.BulkAddClientAccessGroupMembers(ctx, accessGroup.ID, genericReq)
	} else {
		result, err = s.BulkMoveClientAccessGroupMembers(ctx, accessGroup.ID, genericReq)
	}
	if err != nil {
		return domain.VLESSGroupMembershipResult{}, err
	}
	return clientAccessGroupResultToVLESS(result), nil
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

	result, err := s.AddVLESSGroupMembers(ctx, group.Key, req)
	if err != nil {
		return result, err
	}
	result.InstanceID = instance.ID
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

func (s *Store) queryGlobalVLESSGroupMemberClients(ctx context.Context, groupKey, search, status string, limit, offset int) ([]domain.VLESSGroupMemberClient, int, error) {
	where := []string{
		`vgm.group_key=$1`,
		`vgm.status in ('active','disabled')`,
	}
	args := []any{groupKey}
	if q := strings.TrimSpace(search); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, fmt.Sprintf(`(lower(ca.username) like $%d or lower(coalesce(ca.display_name,'')) like $%d or lower(coalesce(ca.email,'')) like $%d or lower(ca.id::text) like $%d)`, len(args), len(args), len(args), len(args)))
	}
	if st := strings.ToLower(strings.TrimSpace(status)); st != "" && st != "all" {
		args = append(args, st)
		where = append(where, fmt.Sprintf(`(lower(ca.status)=$%d or lower(vgm.status)=$%d)`, len(args), len(args)))
	}
	countSQL := `select count(*)::int
		from vless_group_memberships vgm
		join client_accounts ca on ca.id=vgm.client_account_id
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
			vgm.id::text,
			vgm.status,
			vgm.group_key,
			'',
			vgm.updated_at
		from vless_group_memberships vgm
		join client_accounts ca on ca.id=vgm.client_account_id
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

func (s *Store) queryGlobalVLESSGroupAvailableClients(ctx context.Context, search, assignment string, limit, offset int) ([]domain.VLESSGroupMemberClient, int, error) {
	where := []string{`ca.status <> 'deleted'`}
	args := []any{}
	switch strings.ToLower(strings.TrimSpace(assignment)) {
	case "", "unassigned":
		where = append(where, `vgm.id is null`)
	case "assigned":
		where = append(where, `vgm.id is not null`)
	case "all":
	default:
		return nil, 0, fmt.Errorf("unsupported assignment filter %q", assignment)
	}
	if q := strings.TrimSpace(search); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, fmt.Sprintf(`(lower(ca.username) like $%d or lower(coalesce(ca.display_name,'')) like $%d or lower(coalesce(ca.email,'')) like $%d or lower(ca.id::text) like $%d)`, len(args), len(args), len(args), len(args)))
	}
	base := ` from client_accounts ca
		left join vless_group_memberships vgm on vgm.client_account_id=ca.id
			and vgm.status in ('active','disabled')
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
			coalesce(vgm.id::text,''),
			coalesce(vgm.status,''),
			coalesce(vgm.group_key,''),
			'',
			vgm.updated_at
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

func (s *Store) queryGlobalVLESSGroupAvailableClientIDs(ctx context.Context, search, assignment string) ([]string, error) {
	where := []string{`ca.status <> 'deleted'`}
	args := []any{}
	switch strings.ToLower(strings.TrimSpace(assignment)) {
	case "", "unassigned":
		where = append(where, `vgm.id is null`)
	case "assigned":
		where = append(where, `vgm.id is not null`)
	case "all":
	default:
		return nil, fmt.Errorf("unsupported assignment filter %q", assignment)
	}
	if q := strings.TrimSpace(search); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, fmt.Sprintf(`(lower(ca.username) like $%d or lower(coalesce(ca.display_name,'')) like $%d or lower(coalesce(ca.email,'')) like $%d or lower(ca.id::text) like $%d)`, len(args), len(args), len(args), len(args)))
	}
	rows, err := s.db.Query(ctx, `select ca.id::text
		from client_accounts ca
		left join vless_group_memberships vgm on vgm.client_account_id=ca.id
			and vgm.status in ('active','disabled')
		where `+strings.Join(where, " and ")+`
		order by ca.username asc, ca.created_at asc`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var clientID string
		if err := rows.Scan(&clientID); err != nil {
			return nil, err
		}
		out = append(out, clientID)
	}
	return out, rows.Err()
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

func (s *Store) previewGlobalVLESSGroupMember(ctx context.Context, clientID, groupKey, mode string) (string, bool, domain.VLESSGroupMemberClient, error) {
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
	existing, err := s.existingGlobalVLESSGroupMember(ctx, client.ID)
	if err != nil {
		return "", false, domain.VLESSGroupMemberClient{}, err
	}
	if existing.ClientID != "" {
		existingGroup := normalizeXrayVLESSGroupKey(existing.GroupKey)
		if existingGroup == groupKey && in(existing.AccessStatus, "active", "disabled") {
			return "", true, existing, nil
		}
		if existingGroup != "" && existingGroup != groupKey && mode == "add_only" {
			return "", true, existing, nil
		}
		existing.GroupKey = groupKey
		existing.AccessStatus = "active"
		return "updated", false, existing, nil
	}
	return "created", false, domain.VLESSGroupMemberClient{
		ClientID:     client.ID,
		Username:     client.Username,
		DisplayName:  client.DisplayName,
		Email:        client.Email,
		ClientStatus: client.Status,
		GroupKey:     groupKey,
		AccessStatus: "active",
	}, nil
}

func (s *Store) upsertGlobalVLESSGroupMember(ctx context.Context, clientID, groupKey, mode string) (string, bool, domain.VLESSGroupMemberClient, error) {
	change, skipped, preview, err := s.previewGlobalVLESSGroupMember(ctx, clientID, groupKey, mode)
	if err != nil || skipped {
		return change, skipped, preview, err
	}
	metadata := map[string]any{"source": "operator:global-vless-group-membership"}
	var membershipID string
	err = s.db.QueryRow(ctx, `insert into vless_group_memberships(id,group_key,client_account_id,status,metadata_json,created_at,updated_at)
		values($1,$2,$3,'active',$4,now(),now())
		on conflict(client_account_id) do update set
			group_key=excluded.group_key,
			status='active',
			metadata_json=vless_group_memberships.metadata_json || excluded.metadata_json,
			updated_at=now()
		returning id::text`, id.New(), groupKey, clientID, mustJSON(metadata)).Scan(&membershipID)
	if err != nil {
		return "", false, domain.VLESSGroupMemberClient{}, err
	}
	updated, err := s.existingGlobalVLESSGroupMember(ctx, clientID)
	if err != nil {
		return "", false, domain.VLESSGroupMemberClient{}, err
	}
	if updated.ServiceAccessID == "" {
		updated.ServiceAccessID = membershipID
	}
	if change == "" {
		change = "updated"
	}
	return change, false, updated, nil
}

func (s *Store) existingGlobalVLESSGroupMember(ctx context.Context, clientID string) (domain.VLESSGroupMemberClient, error) {
	var item domain.VLESSGroupMemberClient
	err := s.db.QueryRow(ctx, `select
			ca.id::text,
			ca.username,
			coalesce(ca.display_name,''),
			coalesce(ca.email,''),
			ca.status,
			vgm.id::text,
			vgm.status,
			vgm.group_key,
			'',
			vgm.updated_at
		from vless_group_memberships vgm
		join client_accounts ca on ca.id=vgm.client_account_id
		where vgm.client_account_id=$1
		  and vgm.status in ('active','disabled')`, clientID).Scan(
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

func (s *Store) getActiveVLESSGroupTemplate(ctx context.Context, groupKey string) (domain.VLESSGroupTemplate, error) {
	groupKey = normalizeXrayVLESSGroupKey(groupKey)
	if groupKey == "" {
		return domain.VLESSGroupTemplate{}, fmt.Errorf("vless group key is required")
	}
	row := s.db.QueryRow(ctx, `select key,label,description,access_mode,egress_mode,
		coalesce(egress_node_id::text,''),coalesce(target_instance_id::text,''),outbound_tag,ad_block,
		rules_json,extra_rules_json,status,source,version,display_order
		from vless_group_templates
		where key=$1`, groupKey)
	template, err := scanVLESSGroupTemplate(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.VLESSGroupTemplate{}, domain.ErrVLESSGroupTemplateNotFound
	}
	if err != nil {
		return domain.VLESSGroupTemplate{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(template.Status), "active") {
		return domain.VLESSGroupTemplate{}, fmt.Errorf("vless group %q is not active", template.Key)
	}
	return template, nil
}

func (s *Store) vlessMembershipTargetInstances(ctx context.Context, groupKey string) ([]domain.Instance, error) {
	groupKey = normalizeXrayVLESSGroupKey(groupKey)
	instances, err := s.listXrayVLESSGroupSyncInstances(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Instance, 0, len(instances))
	for _, instance := range instances {
		spec, err := s.ensureXrayProvisioningGroups(ctx, instance.ID, instance.Spec)
		if err != nil {
			return nil, err
		}
		instance.Spec = spec
		groups := xrayVLESSGroups(spec)
		for _, group := range groups {
			if group.Key == groupKey && vlessGroupUsableForNewMembership(group) {
				out = append(out, instance)
				break
			}
		}
	}
	return out, nil
}

func (s *Store) materializeGlobalVLESSGroupMemberships(ctx context.Context, groupKey string, clientIDs []string, targets []domain.Instance) (int, []domain.VLESSGroupMembershipFailure, []string) {
	materialized := 0
	failures := []domain.VLESSGroupMembershipFailure{}
	warnings := []string{}
	if len(targets) == 0 {
		warnings = append(warnings, "no active Xray/VLESS instances currently expose this group; membership is stored and will be materialized when an instance is created or synced")
		return 0, failures, warnings
	}
	for _, instance := range targets {
		for _, clientID := range clientIDs {
			change, skipped, _, err := s.upsertInstanceVLESSGroupMember(ctx, instance.ID, clientID, groupKey, "add_or_move")
			if err != nil {
				failures = append(failures, domain.VLESSGroupMembershipFailure{ClientID: clientID, Error: fmt.Sprintf("instance %s: %s", instance.ID, err.Error())})
				continue
			}
			if !skipped && (change == "created" || change == "updated") {
				materialized++
			}
		}
	}
	return materialized, failures, warnings
}

func (s *Store) materializeGlobalVLESSGroupMembershipsForInstance(ctx context.Context, instanceID string) (int, error) {
	instance, groups, err := s.loadInstanceVLESSGroups(ctx, instanceID)
	if err != nil {
		return 0, err
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return 0, nil
	}
	rows, err := s.db.Query(ctx, `select
			m.client_account_id::text,
			cag.group_key,
			cag.id::text,
			cag.display_name,
			cag.policy_json,
			cag.scope_mode,
			cag.auto_apply_new_instances
		from client_access_group_memberships m
		join client_access_groups cag on cag.id=m.group_id
		where m.status='active'
		  and m.service_code='vless'
		  and cag.status='active'
		  and cag.deleted_at is null
		  and cag.group_key = any($1)`, keys)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	materialized := 0
	for rows.Next() {
		var clientID string
		var groupKey string
		var group domain.ClientAccessGroup
		var policyRaw []byte
		if err := rows.Scan(&clientID, &groupKey, &group.ID, &group.DisplayName, &policyRaw, &group.ScopeMode, &group.AutoApplyNewInstances); err != nil {
			return materialized, err
		}
		group.ServiceCode = "vless"
		group.Status = "active"
		group.GroupKey = groupKey
		group.PolicyJSON = json.RawMessage(policyRaw)
		allowed, err := s.clientAccessGroupAllowsInstance(ctx, group, instance.ID)
		if err != nil {
			return materialized, err
		}
		if !allowed {
			continue
		}
		hash, hashErr := s.clientAccessGroupDesiredHash(ctx, group, []domain.Instance{instance})
		if hashErr != nil {
			hash = ""
		}
		change, skipped, member, err := s.upsertInstanceVLESSGroupMember(ctx, instance.ID, clientID, groupKey, "add_or_move")
		if err != nil {
			return materialized, err
		}
		if member.ServiceAccessID != "" {
			_ = s.markServiceAccessClientAccessGroup(ctx, member.ServiceAccessID, group, hash)
		}
		if !skipped && (change == "created" || change == "updated") {
			materialized++
		}
	}
	return materialized, rows.Err()
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

func mergeVLESSClientIDs(primary, extra []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(primary)+len(extra))
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
	for _, clientID := range primary {
		add(clientID)
	}
	for _, clientID := range extra {
		add(clientID)
	}
	return out
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

func clientAccessGroupMembersToVLESS(items []domain.ClientAccessGroupMember) []domain.VLESSGroupMemberClient {
	out := make([]domain.VLESSGroupMemberClient, 0, len(items))
	for _, item := range items {
		out = append(out, domain.VLESSGroupMemberClient{
			ClientID:        item.ClientID,
			Username:        item.Username,
			DisplayName:     item.DisplayName,
			Email:           item.Email,
			ClientStatus:    item.ClientStatus,
			ServiceAccessID: firstString(item.ServiceAccessID, item.MembershipID),
			AccessStatus:    firstString(item.AccessStatus, item.MembershipStatus),
			GroupKey:        normalizeXrayVLESSGroupKey(item.GroupKey),
			XrayUUID:        item.XrayUUID,
			UpdatedAt:       item.UpdatedAt,
		})
	}
	return out
}

func clientAccessGroupFailuresToVLESS(items []domain.ClientAccessGroupMembershipFailure) []domain.VLESSGroupMembershipFailure {
	out := make([]domain.VLESSGroupMembershipFailure, 0, len(items))
	for _, item := range items {
		out = append(out, domain.VLESSGroupMembershipFailure{
			ClientID: item.ClientID,
			Ref:      item.Ref,
			Error:    item.Error,
		})
	}
	return out
}

func clientAccessGroupResultToVLESS(result domain.ClientAccessGroupMembershipResult) domain.VLESSGroupMembershipResult {
	out := domain.VLESSGroupMembershipResult{
		InstanceIDs:       []string{},
		GroupKey:          result.GroupKey,
		DryRun:            result.DryRun,
		AllFiltered:       result.AllFiltered,
		Created:           result.CreatedMemberships,
		Updated:           result.MovedMemberships,
		Skipped:           result.SkippedExisting,
		Failed:            clientAccessGroupFailuresToVLESS(result.Failed),
		Materialized:      result.MaterializedCreated + result.MaterializedUpdated,
		AffectedInstances: result.AffectedInstances,
		ApplyJobCount:     result.ApplyJobCount,
		ApplyJobIDs:       result.ApplyJobIDs,
		Warnings:          result.Warnings,
		Clients:           clientAccessGroupMembersToVLESS(result.Clients),
	}
	if len(out.ApplyJobIDs) > 0 {
		out.ApplyJobID = out.ApplyJobIDs[0]
	}
	for _, conflict := range result.Conflicts {
		if conflict.Reason == "" {
			continue
		}
		out.Warnings = append(out.Warnings, fmt.Sprintf("client %s: %s (%s -> %s)", conflict.ClientID, conflict.Reason, conflict.ExistingGroup, conflict.TargetGroup))
	}
	return out
}
