package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

const defaultClientAccessGroupMembershipLimit = 50

func (s *Store) ListClientAccessGroups(ctx context.Context, serviceCode string) ([]domain.ClientAccessGroup, error) {
	serviceCode = normalizeClientAccessGroupServiceCode(serviceCode)
	where := []string{`cag.deleted_at is null`}
	args := []any{}
	if serviceCode != "" {
		args = append(args, serviceCode)
		where = append(where, fmt.Sprintf(`cag.service_code=$%d`, len(args)))
	}
	rows, err := s.db.Query(ctx, `select
			cag.id::text,
			cag.service_code,
			cag.group_key,
			cag.display_name,
			coalesce(cag.description,''),
			cag.status,
			cag.policy_json,
			cag.scope_mode,
			cag.auto_apply_new_instances,
			coalesce(count(m.id) filter(where m.status in ('active','disabled')),0)::int,
			coalesce(count(m.id) filter(where m.status='active'),0)::int,
			coalesce(count(m.id) filter(where m.status='disabled'),0)::int,
			coalesce(count(ss.instance_id),0)::int,
			coalesce(count(ss.instance_id) filter(where ss.status in ('pending','queued')),0)::int,
			coalesce(count(ss.instance_id) filter(where ss.status='failed'),0)::int,
			coalesce(count(ss.instance_id) filter(where ss.status='applied'),0)::int,
			cag.created_at,
			cag.updated_at,
			cag.deleted_at
		from client_access_groups cag
		left join client_access_group_memberships m on m.group_id=cag.id and m.status in ('active','disabled')
		left join client_access_group_sync_state ss on ss.group_id=cag.id
		where `+strings.Join(where, " and ")+`
		group by cag.id
		order by case cag.status when 'active' then 0 when 'disabled' then 1 else 2 end,
			cag.service_code asc, cag.display_name asc, cag.group_key asc`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups, err := scanClientAccessGroupRows(rows)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		instances, resolveErr := s.resolveClientAccessGroupInstances(ctx, groups[i])
		if resolveErr == nil {
			groups[i].AffectedInstances = len(instances)
		}
	}
	return groups, nil
}

func (s *Store) GetClientAccessGroup(ctx context.Context, groupID string) (domain.ClientAccessGroup, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return domain.ClientAccessGroup{}, domain.ErrClientAccessGroupNotFound
	}
	group, err := s.getClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	instances, resolveErr := s.resolveClientAccessGroupInstances(ctx, group)
	if resolveErr == nil {
		group.AffectedInstances = len(instances)
	}
	return group, nil
}

func (s *Store) CreateClientAccessGroup(ctx context.Context, input domain.ClientAccessGroupInput, userID *string) (domain.ClientAccessGroup, error) {
	normalized, err := normalizeClientAccessGroupInput(input, true)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	groupID := id.New()
	row := s.db.QueryRow(ctx, `insert into client_access_groups(
			id,service_code,group_key,display_name,description,status,policy_json,
			scope_mode,auto_apply_new_instances,created_by,updated_by,created_at,updated_at
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,nullif($10,'')::uuid,nullif($10,'')::uuid,now(),now())
		returning id::text,service_code,group_key,display_name,coalesce(description,''),status,policy_json,
			scope_mode,auto_apply_new_instances,0::int,0::int,0::int,0::int,0::int,0::int,0::int,created_at,updated_at,deleted_at`,
		groupID,
		normalized.ServiceCode,
		normalized.GroupKey,
		normalized.DisplayName,
		normalized.Description,
		normalized.Status,
		normalized.PolicyJSON,
		normalized.ScopeMode,
		normalized.autoApplyValue(),
		derefUserID(userID),
	)
	group, err := scanClientAccessGroup(row)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if err := s.mirrorClientAccessGroupToLegacyVLESSCatalog(ctx, group); err != nil {
		return domain.ClientAccessGroup{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "client_access_group.created", "access_group", &group.ID, "client access group created: "+group.GroupKey)
	return group, nil
}

func (s *Store) UpdateClientAccessGroup(ctx context.Context, groupID string, input domain.ClientAccessGroupInput, userID *string) (domain.ClientAccessGroup, error) {
	existing, err := s.getClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if strings.EqualFold(existing.Status, "deleted") || existing.DeletedAt != nil {
		return domain.ClientAccessGroup{}, fmt.Errorf("client access group is deleted")
	}
	previousInstances, err := s.resolveClientAccessGroupInstances(ctx, existing)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if strings.TrimSpace(input.ServiceCode) == "" {
		input.ServiceCode = existing.ServiceCode
	}
	if strings.TrimSpace(input.GroupKey) == "" {
		input.GroupKey = existing.GroupKey
	}
	if strings.TrimSpace(input.Status) == "" {
		input.Status = existing.Status
	}
	if strings.TrimSpace(input.ScopeMode) == "" {
		input.ScopeMode = existing.ScopeMode
	}
	if input.AutoApplyNewInstances == nil {
		value := existing.AutoApplyNewInstances
		input.AutoApplyNewInstances = &value
	}
	if len(strings.TrimSpace(string(input.PolicyJSON))) == 0 {
		input.PolicyJSON = existing.PolicyJSON
	}
	normalized, err := normalizeClientAccessGroupInput(input, false)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	runtimeChanged := !strings.EqualFold(existing.Status, normalized.Status) ||
		!strings.EqualFold(existing.ScopeMode, normalized.ScopeMode) ||
		existing.AutoApplyNewInstances != normalized.autoApplyValue() ||
		strings.TrimSpace(string(existing.PolicyJSON)) != strings.TrimSpace(string(normalized.PolicyJSON))
	row := s.db.QueryRow(ctx, `update client_access_groups
		set display_name=$2,
			description=$3,
			status=$4,
			policy_json=$5,
			scope_mode=$6,
			auto_apply_new_instances=$7,
			updated_by=nullif($8,'')::uuid,
			updated_at=now()
		where id=$1 and deleted_at is null
		returning id::text,service_code,group_key,display_name,coalesce(description,''),status,policy_json,
			scope_mode,auto_apply_new_instances,
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status in ('active','disabled')),
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status='active'),
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status='disabled'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status in ('pending','queued')),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status='failed'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status='applied'),
			created_at,updated_at,deleted_at`,
		existing.ID,
		normalized.DisplayName,
		normalized.Description,
		normalized.Status,
		normalized.PolicyJSON,
		normalized.ScopeMode,
		normalized.autoApplyValue(),
		derefUserID(userID),
	)
	group, err := scanClientAccessGroup(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClientAccessGroup{}, domain.ErrClientAccessGroupNotFound
	}
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if err := s.mirrorClientAccessGroupToLegacyVLESSCatalog(ctx, group); err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if runtimeChanged {
		if _, err := s.syncClientAccessGroupRuntime(ctx, group, previousInstances, !strings.EqualFold(group.Status, "active"), true, "client access group policy updated"); err != nil {
			return group, err
		}
	}
	_, _ = s.CreateAudit(ctx, "system", "client_access_group.updated", "access_group", &group.ID, "client access group updated: "+group.GroupKey)
	return group, nil
}

func (s *Store) SetClientAccessGroupStatus(ctx context.Context, groupID, status string, userID *string) (domain.ClientAccessGroup, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if !in(status, "active", "disabled") {
		return domain.ClientAccessGroup{}, fmt.Errorf("invalid client access group status %q", status)
	}
	existing, err := s.getClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	previousInstances, err := s.resolveClientAccessGroupInstances(ctx, existing)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	row := s.db.QueryRow(ctx, `update client_access_groups
		set status=$2, updated_by=nullif($3,'')::uuid, updated_at=now()
		where id=$1 and deleted_at is null
		returning id::text,service_code,group_key,display_name,coalesce(description,''),status,policy_json,
			scope_mode,auto_apply_new_instances,
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status in ('active','disabled')),
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status='active'),
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status='disabled'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status in ('pending','queued')),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status='failed'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status='applied'),
			created_at,updated_at,deleted_at`,
		strings.TrimSpace(groupID), status, derefUserID(userID))
	group, err := scanClientAccessGroup(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClientAccessGroup{}, domain.ErrClientAccessGroupNotFound
	}
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if err := s.mirrorClientAccessGroupToLegacyVLESSCatalog(ctx, group); err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if _, err := s.syncClientAccessGroupRuntime(ctx, group, previousInstances, !strings.EqualFold(group.Status, "active"), true, "client access group status changed"); err != nil {
		return group, err
	}
	action := "client_access_group.disabled"
	if status == "active" {
		action = "client_access_group.enabled"
	}
	_, _ = s.CreateAudit(ctx, "system", action, "access_group", &group.ID, "client access group status changed: "+group.GroupKey)
	return group, nil
}

func (s *Store) DeleteClientAccessGroupSoft(ctx context.Context, groupID string, userID *string) (domain.ClientAccessGroup, error) {
	existing, err := s.getClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	previousInstances, err := s.resolveClientAccessGroupInstances(ctx, existing)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	row := s.db.QueryRow(ctx, `update client_access_groups
		set status='deleted', deleted_at=coalesce(deleted_at, now()), updated_by=nullif($2,'')::uuid, updated_at=now()
		where id=$1
		returning id::text,service_code,group_key,display_name,coalesce(description,''),status,policy_json,
			scope_mode,auto_apply_new_instances,
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status in ('active','disabled')),
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status='active'),
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status='disabled'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status in ('pending','queued')),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status='failed'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status='applied'),
			created_at,updated_at,deleted_at`,
		strings.TrimSpace(groupID), derefUserID(userID))
	group, err := scanClientAccessGroup(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClientAccessGroup{}, domain.ErrClientAccessGroupNotFound
	}
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if err := s.mirrorClientAccessGroupToLegacyVLESSCatalog(ctx, group); err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if _, err := s.syncClientAccessGroupRuntime(ctx, group, previousInstances, true, true, "client access group deleted"); err != nil {
		return group, err
	}
	_, _ = s.CreateAudit(ctx, "system", "client_access_group.deleted", "access_group", &group.ID, "client access group deleted: "+group.GroupKey)
	return group, nil
}

func (s *Store) ListClientAccessGroupMembers(ctx context.Context, groupID, search, status string, limit, offset int) (domain.ClientAccessGroupMembersPage, error) {
	group, err := s.getClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroupMembersPage{}, err
	}
	limit, offset = normalizeClientAccessGroupPage(limit, offset)
	where := []string{`m.group_id=$1`, `m.status in ('active','disabled','revoked')`}
	args := []any{group.ID}
	if q := strings.TrimSpace(search); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, fmt.Sprintf(`(lower(ca.username) like $%d or lower(coalesce(ca.display_name,'')) like $%d or lower(coalesce(ca.email,'')) like $%d or lower(ca.id::text) like $%d)`, len(args), len(args), len(args), len(args)))
	}
	if st := strings.ToLower(strings.TrimSpace(status)); st != "" && st != "all" {
		args = append(args, st)
		where = append(where, fmt.Sprintf(`(lower(ca.status)=$%d or lower(m.status)=$%d)`, len(args), len(args)))
	}
	var total int
	if err := s.db.QueryRow(ctx, `select count(*)::int
		from client_access_group_memberships m
		join client_accounts ca on ca.id=m.client_account_id
		where `+strings.Join(where, " and "), args...).Scan(&total); err != nil {
		return domain.ClientAccessGroupMembersPage{}, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query(ctx, `select
			ca.id::text,
			ca.username,
			coalesce(ca.display_name,''),
			coalesce(ca.email,''),
			ca.status,
			m.id::text,
			m.status,
			cag.id::text,
			cag.group_key,
			cag.display_name,
			coalesce(sa.id::text,''),
			coalesce(sa.status,''),
			coalesce(nullif(sa.metadata_json->>'xray_uuid',''), nullif(sa.metadata_json->>'uuid',''), ''),
			m.updated_at
		from client_access_group_memberships m
		join client_access_groups cag on cag.id=m.group_id
		join client_accounts ca on ca.id=m.client_account_id
		left join lateral (
			select sa.id, sa.status, sa.metadata_json
			from service_accesses sa
			where sa.client_account_id=ca.id
			  and coalesce(sa.metadata_json->>'access_group_id','') = cag.id::text
			  and sa.status in ('pending','active','disabled')
			order by sa.updated_at desc
			limit 1
		) sa on true
		where `+strings.Join(where, " and ")+`
		order by ca.username asc, ca.created_at asc
		limit $`+fmt.Sprint(len(args)-1)+` offset $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return domain.ClientAccessGroupMembersPage{}, err
	}
	defer rows.Close()
	items, err := scanClientAccessGroupMemberRows(rows)
	if err != nil {
		return domain.ClientAccessGroupMembersPage{}, err
	}
	return domain.ClientAccessGroupMembersPage{
		GroupID:     group.ID,
		ServiceCode: group.ServiceCode,
		GroupKey:    group.GroupKey,
		Status:      group.Status,
		Items:       items,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
	}, nil
}

func (s *Store) ListClientAccessGroupAvailableClients(ctx context.Context, serviceCode, search, assignment, clientStatus string, limit, offset int) (domain.ClientAccessGroupAvailableClientsPage, error) {
	serviceCode = normalizeClientAccessGroupServiceCode(serviceCode)
	if serviceCode == "" {
		serviceCode = "vless"
	}
	assignment = strings.ToLower(strings.TrimSpace(assignment))
	if assignment == "" {
		assignment = "unassigned"
	}
	if !in(assignment, "unassigned", "assigned", "all") {
		return domain.ClientAccessGroupAvailableClientsPage{}, fmt.Errorf("unsupported assignment filter %q", assignment)
	}
	limit, offset = normalizeClientAccessGroupPage(limit, offset)
	items, total, err := s.queryClientAccessGroupAvailableClients(ctx, serviceCode, search, assignment, clientStatus, limit, offset)
	if err != nil {
		return domain.ClientAccessGroupAvailableClientsPage{}, err
	}
	return domain.ClientAccessGroupAvailableClientsPage{
		ServiceCode: serviceCode,
		Assignment:  assignment,
		Items:       items,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
	}, nil
}

func (s *Store) PreviewClientAccessGroupMembers(ctx context.Context, groupID string, req domain.ClientAccessGroupMembershipRequest) (domain.ClientAccessGroupMembershipResult, error) {
	req.DryRun = true
	return s.bulkClientAccessGroupMembers(ctx, groupID, req, false)
}

func (s *Store) BulkAddClientAccessGroupMembers(ctx context.Context, groupID string, req domain.ClientAccessGroupMembershipRequest) (domain.ClientAccessGroupMembershipResult, error) {
	if strings.TrimSpace(req.Mode) == "" {
		req.Mode = "add_only"
	}
	return s.bulkClientAccessGroupMembers(ctx, groupID, req, false)
}

func (s *Store) BulkMoveClientAccessGroupMembers(ctx context.Context, groupID string, req domain.ClientAccessGroupMembershipRequest) (domain.ClientAccessGroupMembershipResult, error) {
	req.Mode = "add_or_move"
	return s.bulkClientAccessGroupMembers(ctx, groupID, req, true)
}

func (s *Store) RemoveClientAccessGroupMember(ctx context.Context, groupID, clientID string) (domain.ClientAccessGroupMembershipResult, error) {
	group, err := s.getMutableClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroupMembershipResult{}, err
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return domain.ClientAccessGroupMembershipResult{}, fmt.Errorf("client id is required")
	}
	tag, err := s.db.Exec(ctx, `update client_access_group_memberships
		set status='removed', removed_at=now(), updated_at=now()
		where group_id=$1 and client_account_id=$2 and status='active'`, group.ID, clientID)
	if err != nil {
		return domain.ClientAccessGroupMembershipResult{}, err
	}
	result := domain.ClientAccessGroupMembershipResult{
		GroupID:     group.ID,
		GroupKey:    group.GroupKey,
		ServiceCode: group.ServiceCode,
	}
	if tag.RowsAffected() == 0 {
		result.SkippedExisting = 1
		return result, nil
	}
	instances, err := s.resolveClientAccessGroupInstances(ctx, group)
	if err != nil {
		return domain.ClientAccessGroupMembershipResult{}, err
	}
	hash, _ := s.clientAccessGroupDesiredHash(ctx, group, instances)
	disabled, disableErr := s.disableClientAccessGroupMaterialization(ctx, group, []string{clientID}, nil, hash)
	if disableErr != nil {
		result.Warnings = append(result.Warnings, disableErr.Error())
	}
	result.MaterializedDisabled = disabled
	for _, instance := range instances {
		job, err := s.UpdateInstanceStatus(ctx, instance.ID, "apply")
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("instance %s apply was not queued: %s", instance.ID, err.Error()))
			continue
		}
		result.ApplyJobIDs = append(result.ApplyJobIDs, job.ID)
	}
	result.ApplyJobCount = len(result.ApplyJobIDs)
	_, _ = s.CreateAudit(ctx, "system", "client_access_group.members.removed", "access_group", &group.ID, "client access group member removed")
	return result, nil
}

func (s *Store) GetClientAccessGroupScope(ctx context.Context, groupID string) (domain.ClientAccessGroupScope, error) {
	group, err := s.getClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	scope := domain.ClientAccessGroupScope{
		GroupID:               group.ID,
		ScopeMode:             group.ScopeMode,
		AutoApplyNewInstances: group.AutoApplyNewInstances,
	}
	rows, err := s.db.Query(ctx, `select instance_id::text, mode
		from client_access_group_instance_scopes
		where group_id=$1
		order by created_at asc`, group.ID)
	if err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var instanceID string
		var mode string
		if err := rows.Scan(&instanceID, &mode); err != nil {
			return domain.ClientAccessGroupScope{}, err
		}
		if mode == "include" {
			scope.IncludeInstanceIDs = append(scope.IncludeInstanceIDs, instanceID)
		} else if mode == "exclude" {
			scope.ExcludeInstanceIDs = append(scope.ExcludeInstanceIDs, instanceID)
		}
	}
	if err := rows.Err(); err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	instances, resolveErr := s.resolveClientAccessGroupInstances(ctx, group)
	if resolveErr == nil {
		scope.AffectedInstances = len(instances)
	}
	return scope, nil
}

func (s *Store) UpdateClientAccessGroupScope(ctx context.Context, groupID string, scope domain.ClientAccessGroupScope, userID *string) (domain.ClientAccessGroupScope, error) {
	group, err := s.getMutableClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	previousInstances, err := s.resolveClientAccessGroupInstances(ctx, group)
	if err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	scope.ScopeMode = normalizeClientAccessGroupScopeMode(firstString(scope.ScopeMode, group.ScopeMode))
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `update client_access_groups
		set scope_mode=$2, auto_apply_new_instances=$3, updated_by=nullif($4,'')::uuid, updated_at=now()
		where id=$1`, group.ID, scope.ScopeMode, scope.AutoApplyNewInstances, derefUserID(userID)); err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	if _, err := tx.Exec(ctx, `delete from client_access_group_instance_scopes where group_id=$1`, group.ID); err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	for _, instanceID := range uniqueStrings(scope.IncludeInstanceIDs) {
		if strings.TrimSpace(instanceID) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `insert into client_access_group_instance_scopes(group_id,instance_id,mode,created_at)
			values($1,$2,'include',now())
			on conflict do nothing`, group.ID, instanceID); err != nil {
			return domain.ClientAccessGroupScope{}, err
		}
	}
	for _, instanceID := range uniqueStrings(scope.ExcludeInstanceIDs) {
		if strings.TrimSpace(instanceID) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `insert into client_access_group_instance_scopes(group_id,instance_id,mode,created_at)
			values($1,$2,'exclude',now())
			on conflict do nothing`, group.ID, instanceID); err != nil {
			return domain.ClientAccessGroupScope{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	updatedGroup, err := s.getMutableClientAccessGroup(ctx, group.ID)
	if err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	updatedScope, err := s.GetClientAccessGroupScope(ctx, group.ID)
	if err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	runtime, err := s.syncClientAccessGroupRuntime(ctx, updatedGroup, previousInstances, false, false, "client access group scope materialized")
	if err != nil {
		return domain.ClientAccessGroupScope{}, err
	}
	updatedScope.MaterializedCreated = runtime.MaterializedCreated
	updatedScope.MaterializedUpdated = runtime.MaterializedUpdated
	updatedScope.MaterializedDisabled = runtime.MaterializedDisabled
	updatedScope.ApplyJobIDs = runtime.ApplyJobIDs
	updatedScope.ApplyJobCount = runtime.ApplyJobCount
	updatedScope.Warnings = append(updatedScope.Warnings, runtime.Warnings...)
	for _, failure := range runtime.Failed {
		updatedScope.Warnings = append(updatedScope.Warnings, failure.Error)
	}
	_, _ = s.CreateAudit(ctx, "system", "client_access_group.scope.updated", "access_group", &group.ID, fmt.Sprintf("client access group scope updated: %s created=%d updated=%d disabled=%d apply_jobs=%d", group.GroupKey, updatedScope.MaterializedCreated, updatedScope.MaterializedUpdated, updatedScope.MaterializedDisabled, updatedScope.ApplyJobCount))
	return updatedScope, nil
}

func (s *Store) PreviewClientAccessGroupSync(ctx context.Context, groupID string) (domain.ClientAccessGroupSyncPreview, error) {
	group, err := s.getClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroupSyncPreview{}, err
	}
	instances, err := s.resolveClientAccessGroupInstances(ctx, group)
	if err != nil {
		return domain.ClientAccessGroupSyncPreview{}, err
	}
	hash, err := s.clientAccessGroupDesiredHash(ctx, group, instances)
	if err != nil {
		return domain.ClientAccessGroupSyncPreview{}, err
	}
	var memberCount int
	if err := s.db.QueryRow(ctx, `select count(*)::int from client_access_group_memberships where group_id=$1 and status='active'`, group.ID).Scan(&memberCount); err != nil {
		return domain.ClientAccessGroupSyncPreview{}, err
	}
	preview := domain.ClientAccessGroupSyncPreview{
		GroupID:           group.ID,
		GroupKey:          group.GroupKey,
		ServiceCode:       group.ServiceCode,
		DesiredHash:       hash,
		AffectedInstances: len(instances),
		MemberCount:       memberCount,
	}
	for _, instance := range instances {
		preview.InstanceIDs = append(preview.InstanceIDs, instance.ID)
	}
	sort.Strings(preview.InstanceIDs)
	rows, err := s.db.Query(ctx, `select status, count(*)::int
		from client_access_group_sync_state
		where group_id=$1
		group by status`, group.ID)
	if err != nil {
		return domain.ClientAccessGroupSyncPreview{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return domain.ClientAccessGroupSyncPreview{}, err
		}
		switch status {
		case "applied":
			preview.AppliedInstances += count
		case "failed":
			preview.FailedInstances += count
		default:
			preview.PendingInstances += count
		}
	}
	return preview, rows.Err()
}

func (s *Store) ApplyClientAccessGroupSync(ctx context.Context, groupID string) (domain.ClientAccessGroupMembershipResult, error) {
	group, err := s.getMutableClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroupMembershipResult{}, err
	}
	if group.ServiceCode != "vless" {
		return domain.ClientAccessGroupMembershipResult{}, fmt.Errorf("client access group sync is not implemented for service %q yet", group.ServiceCode)
	}
	result, err := s.syncClientAccessGroupRuntime(ctx, group, nil, false, true, "client access group sync queued")
	if err != nil {
		return domain.ClientAccessGroupMembershipResult{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "client_access_group.sync.queued", "access_group", &group.ID, fmt.Sprintf("client access group sync queued: %s instances=%d", group.GroupKey, result.AffectedInstances))
	return result, nil
}

func (s *Store) ListClientAccessGroupSyncState(ctx context.Context, groupID string) ([]domain.ClientAccessGroupSyncState, error) {
	group, err := s.getClientAccessGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `select
			group_id::text,
			instance_id::text,
			desired_hash,
			coalesce(last_applied_hash,''),
			status,
			coalesce(last_job_id::text,''),
			coalesce(last_error,''),
			updated_at
		from client_access_group_sync_state
		where group_id=$1
		order by updated_at desc`, group.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ClientAccessGroupSyncState{}
	for rows.Next() {
		var item domain.ClientAccessGroupSyncState
		if err := rows.Scan(&item.GroupID, &item.InstanceID, &item.DesiredHash, &item.LastAppliedHash, &item.Status, &item.LastJobID, &item.LastError, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListClientAccessGroupMigrationConflicts(ctx context.Context, limit int) ([]domain.ClientAccessGroupMigrationConflict, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `select
			id::text,
			client_account_id::text,
			coalesce(instance_id::text,''),
			coalesce(service_access_id::text,''),
			service_code,
			group_key,
			reason,
			created_at
		from client_access_group_migration_conflicts
		order by created_at desc
		limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ClientAccessGroupMigrationConflict{}
	for rows.Next() {
		var item domain.ClientAccessGroupMigrationConflict
		if err := rows.Scan(&item.ID, &item.ClientID, &item.InstanceID, &item.ServiceAccessID, &item.ServiceCode, &item.GroupKey, &item.Reason, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListClientAccessGroupsForClient(ctx context.Context, clientID string) ([]domain.ClientAccessGroup, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, fmt.Errorf("client id is required")
	}
	rows, err := s.db.Query(ctx, `select
			cag.id::text,
			cag.service_code,
			cag.group_key,
			cag.display_name,
			coalesce(cag.description,''),
			cag.status,
			cag.policy_json,
			cag.scope_mode,
			cag.auto_apply_new_instances,
			(select count(*)::int from client_access_group_memberships m where m.group_id=cag.id and m.status in ('active','disabled')),
			(select count(*)::int from client_access_group_memberships m where m.group_id=cag.id and m.status='active'),
			(select count(*)::int from client_access_group_memberships m where m.group_id=cag.id and m.status='disabled'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=cag.id),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=cag.id and ss.status in ('pending','queued')),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=cag.id and ss.status='failed'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=cag.id and ss.status='applied'),
			cag.created_at,
			cag.updated_at,
			cag.deleted_at
		from client_access_group_memberships m
		join client_access_groups cag on cag.id=m.group_id
		where m.client_account_id=$1
		  and m.status='active'
		  and cag.deleted_at is null
		order by cag.service_code asc, cag.display_name asc`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanClientAccessGroupRows(rows)
}

func (s *Store) SetClientAccessGroupForClient(ctx context.Context, clientID, serviceCode, groupID string, userID *string) (domain.ClientAccessGroupMembershipResult, error) {
	serviceCode = normalizeClientAccessGroupServiceCode(serviceCode)
	if serviceCode == "" {
		return domain.ClientAccessGroupMembershipResult{}, fmt.Errorf("service code is required")
	}
	group, err := s.getMutableClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroupMembershipResult{}, err
	}
	if group.ServiceCode != serviceCode {
		return domain.ClientAccessGroupMembershipResult{}, fmt.Errorf("group %s belongs to service %q, not %q", group.GroupKey, group.ServiceCode, serviceCode)
	}
	req := domain.ClientAccessGroupMembershipRequest{
		ClientIDs:  []string{clientID},
		Mode:       "add_or_move",
		QueueApply: true,
	}
	return s.BulkMoveClientAccessGroupMembers(ctx, group.ID, req)
}

func (s *Store) bulkClientAccessGroupMembers(ctx context.Context, groupID string, req domain.ClientAccessGroupMembershipRequest, forceMove bool) (domain.ClientAccessGroupMembershipResult, error) {
	group, err := s.getMutableClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroupMembershipResult{}, err
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "add_only"
	}
	if forceMove {
		mode = "add_or_move"
	}
	if !in(mode, "add_only", "add_or_move") {
		return domain.ClientAccessGroupMembershipResult{}, fmt.Errorf("unsupported membership mode %q", mode)
	}
	clientIDs, failures, err := s.resolveAccessGroupClientRefs(ctx, req.ClientIDs, req.ClientRefs)
	if err != nil {
		return domain.ClientAccessGroupMembershipResult{}, err
	}
	if req.AllFiltered {
		filteredIDs, err := s.queryClientAccessGroupAvailableClientIDs(ctx, group.ServiceCode, req.FilterSearch, req.FilterAssignment, req.FilterStatus)
		if err != nil {
			return domain.ClientAccessGroupMembershipResult{}, err
		}
		clientIDs = mergeVLESSClientIDs(clientIDs, filteredIDs)
	}
	result := domain.ClientAccessGroupMembershipResult{
		GroupID:     group.ID,
		GroupKey:    group.GroupKey,
		ServiceCode: group.ServiceCode,
		DryRun:      req.DryRun,
		AllFiltered: req.AllFiltered,
		Failed:      failures,
	}
	if len(clientIDs) == 0 {
		if len(failures) > 0 {
			result.Warnings = append(result.Warnings, "no matching clients were resolved")
			return result, nil
		}
		return domain.ClientAccessGroupMembershipResult{}, fmt.Errorf("at least one client is required")
	}
	instances, err := s.resolveClientAccessGroupInstances(ctx, group)
	if err != nil {
		return domain.ClientAccessGroupMembershipResult{}, err
	}
	result.AffectedInstances = len(instances)
	if req.DryRun {
		for _, clientID := range clientIDs {
			change, skipped, client, conflict, err := s.previewClientAccessGroupMember(ctx, clientID, group, mode)
			if err != nil {
				result.Failed = append(result.Failed, domain.ClientAccessGroupMembershipFailure{ClientID: clientID, Error: err.Error()})
				continue
			}
			if conflict.Reason != "" {
				result.Conflicts = append(result.Conflicts, conflict)
			}
			if skipped {
				result.SkippedExisting++
			} else if change == "created" {
				result.CreatedMemberships++
			} else if change == "moved" {
				result.MovedMemberships++
			}
			if client.ClientID != "" {
				result.Clients = append(result.Clients, client)
			}
		}
		if req.QueueApply && (result.CreatedMemberships > 0 || result.MovedMemberships > 0) {
			result.ApplyJobCount = len(instances)
		}
		_, _ = s.CreateAudit(ctx, "system", "client_access_group.members.previewed", "access_group", &group.ID, "client access group membership previewed")
		return result, nil
	}
	changedClientIDs := make([]string, 0, len(clientIDs))
	for _, clientID := range clientIDs {
		change, skipped, client, conflict, err := s.upsertClientAccessGroupMember(ctx, clientID, group, mode)
		if err != nil {
			result.Failed = append(result.Failed, domain.ClientAccessGroupMembershipFailure{ClientID: clientID, Error: err.Error()})
			continue
		}
		if conflict.Reason != "" {
			result.Conflicts = append(result.Conflicts, conflict)
		}
		if skipped {
			result.SkippedExisting++
		} else {
			changedClientIDs = append(changedClientIDs, clientID)
			if change == "created" {
				result.CreatedMemberships++
			} else if change == "moved" {
				result.MovedMemberships++
			}
		}
		if client.ClientID != "" {
			result.Clients = append(result.Clients, client)
		}
	}
	hash, hashErr := s.clientAccessGroupDesiredHash(ctx, group, instances)
	if hashErr != nil {
		result.Warnings = append(result.Warnings, hashErr.Error())
	}
	if len(changedClientIDs) > 0 && group.ServiceCode == "vless" {
		created, updated, materializeFailures := s.materializeClientAccessGroupVLESS(ctx, group, changedClientIDs, instances, hash)
		result.MaterializedCreated = created
		result.MaterializedUpdated = updated
		for _, failure := range materializeFailures {
			result.Failed = append(result.Failed, domain.ClientAccessGroupMembershipFailure{ClientID: failure.ClientID, Error: failure.Error})
		}
	} else if len(changedClientIDs) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("materialization for %s groups is not implemented yet", group.ServiceCode))
	}
	if req.BuildArtifacts {
		result.Warnings = append(result.Warnings, "artifact rebuild is not queued per-client from group membership; build configs or use subscriptions after instance apply")
	}
	if req.QueueApply && (result.MaterializedCreated > 0 || result.MaterializedUpdated > 0) {
		for _, instance := range instances {
			job, err := s.UpdateInstanceStatus(ctx, instance.ID, "apply")
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("instance %s apply was not queued: %s", instance.ID, err.Error()))
				if hash != "" {
					_ = s.upsertClientAccessGroupSyncState(ctx, group.ID, instance.ID, hash, "", "failed", err.Error())
				}
				continue
			}
			result.ApplyJobIDs = append(result.ApplyJobIDs, job.ID)
			if hash != "" {
				_ = s.upsertClientAccessGroupSyncState(ctx, group.ID, instance.ID, hash, job.ID, "queued", "")
			}
			_ = s.AddJobLog(ctx, job.ID, "info", "client access group materialized", map[string]any{
				"group_id":     group.ID,
				"group_key":    group.GroupKey,
				"service":      group.ServiceCode,
				"created":      result.CreatedMemberships,
				"moved":        result.MovedMemberships,
				"skipped":      result.SkippedExisting,
				"failed":       len(result.Failed),
				"member_count": len(changedClientIDs),
			})
		}
	}
	result.ApplyJobCount = len(result.ApplyJobIDs)
	action := "client_access_group.members.bulk_added"
	if mode == "add_or_move" {
		action = "client_access_group.members.bulk_moved"
	}
	_, _ = s.CreateAudit(ctx, "system", action, "access_group", &group.ID, fmt.Sprintf("client access group membership updated: created=%d moved=%d skipped=%d failed=%d", result.CreatedMemberships, result.MovedMemberships, result.SkippedExisting, len(result.Failed)))
	return result, nil
}

func (s *Store) previewClientAccessGroupMember(ctx context.Context, clientID string, group domain.ClientAccessGroup, mode string) (string, bool, domain.ClientAccessGroupMember, domain.ClientAccessGroupMembershipConflict, error) {
	client, err := s.loadClientAccessGroupClient(ctx, clientID)
	if err != nil {
		return "", false, domain.ClientAccessGroupMember{}, domain.ClientAccessGroupMembershipConflict{}, err
	}
	if strings.EqualFold(client.Status, "deleted") {
		return "", false, domain.ClientAccessGroupMember{}, domain.ClientAccessGroupMembershipConflict{}, fmt.Errorf("client is deleted")
	}
	existing, err := s.existingClientAccessGroupMember(ctx, client.ID, group.ServiceCode)
	if err != nil {
		return "", false, domain.ClientAccessGroupMember{}, domain.ClientAccessGroupMembershipConflict{}, err
	}
	if existing.ClientID != "" {
		if existing.GroupID == group.ID && in(existing.MembershipStatus, "active", "disabled") {
			return "", true, existing, domain.ClientAccessGroupMembershipConflict{}, nil
		}
		conflict := domain.ClientAccessGroupMembershipConflict{
			ClientID:      client.ID,
			ExistingGroup: existing.GroupKey,
			TargetGroup:   group.GroupKey,
			Reason:        "client already belongs to another active group for this service",
		}
		if mode == "add_only" {
			return "", true, existing, conflict, nil
		}
		existing.GroupID = group.ID
		existing.GroupKey = group.GroupKey
		existing.GroupName = group.DisplayName
		existing.MembershipStatus = "active"
		return "moved", false, existing, conflict, nil
	}
	return "created", false, domain.ClientAccessGroupMember{
		ClientID:         client.ID,
		Username:         client.Username,
		DisplayName:      client.DisplayName,
		Email:            client.Email,
		ClientStatus:     client.Status,
		GroupID:          group.ID,
		GroupKey:         group.GroupKey,
		GroupName:        group.DisplayName,
		MembershipStatus: "active",
	}, domain.ClientAccessGroupMembershipConflict{}, nil
}

func (s *Store) upsertClientAccessGroupMember(ctx context.Context, clientID string, group domain.ClientAccessGroup, mode string) (string, bool, domain.ClientAccessGroupMember, domain.ClientAccessGroupMembershipConflict, error) {
	change, skipped, preview, conflict, err := s.previewClientAccessGroupMember(ctx, clientID, group, mode)
	if err != nil || skipped {
		return change, skipped, preview, conflict, err
	}
	var membershipID string
	err = s.db.QueryRow(ctx, `insert into client_access_group_memberships(
			id,client_account_id,service_code,group_id,status,source,metadata_json,created_at,updated_at
		) values($1,$2,$3,$4,'active','manual',$5,now(),now())
		on conflict(client_account_id, service_code) where status='active' do update set
			group_id=excluded.group_id,
			status='active',
			source='manual',
			metadata_json=client_access_group_memberships.metadata_json || excluded.metadata_json,
			updated_at=now()
		returning id::text`,
		id.New(),
		clientID,
		group.ServiceCode,
		group.ID,
		mustJSON(map[string]any{"source": "operator:client-access-group"}),
	).Scan(&membershipID)
	if err != nil {
		return "", false, domain.ClientAccessGroupMember{}, domain.ClientAccessGroupMembershipConflict{}, err
	}
	updated, err := s.existingClientAccessGroupMember(ctx, clientID, group.ServiceCode)
	if err != nil {
		return "", false, domain.ClientAccessGroupMember{}, domain.ClientAccessGroupMembershipConflict{}, err
	}
	if updated.MembershipID == "" {
		updated.MembershipID = membershipID
	}
	return change, false, updated, conflict, nil
}

func (s *Store) materializeClientAccessGroupVLESS(ctx context.Context, group domain.ClientAccessGroup, clientIDs []string, targets []domain.Instance, desiredHash string) (int, int, []domain.VLESSGroupMembershipFailure) {
	created := 0
	updated := 0
	failures := []domain.VLESSGroupMembershipFailure{}
	if len(targets) == 0 || len(clientIDs) == 0 {
		return created, updated, failures
	}
	for _, instance := range targets {
		for _, clientID := range clientIDs {
			change, skipped, member, err := s.upsertInstanceVLESSGroupMember(ctx, instance.ID, clientID, group.GroupKey, "add_or_move")
			if err != nil {
				failures = append(failures, domain.VLESSGroupMembershipFailure{ClientID: clientID, Error: fmt.Sprintf("instance %s: %s", instance.ID, err.Error())})
				continue
			}
			if member.ServiceAccessID != "" {
				if err := s.markServiceAccessClientAccessGroup(ctx, member.ServiceAccessID, group, desiredHash); err != nil {
					failures = append(failures, domain.VLESSGroupMembershipFailure{ClientID: clientID, Error: fmt.Sprintf("instance %s metadata: %s", instance.ID, err.Error())})
					continue
				}
			}
			if skipped {
				continue
			}
			if change == "created" {
				created++
			} else if change == "updated" {
				updated++
			}
		}
	}
	return created, updated, failures
}

func (s *Store) markServiceAccessClientAccessGroup(ctx context.Context, accessID string, group domain.ClientAccessGroup, desiredHash string) error {
	if strings.TrimSpace(accessID) == "" {
		return nil
	}
	patch := map[string]any{
		"source":           "client_access_group",
		"access_group_id":  group.ID,
		"access_group_key": group.GroupKey,
	}
	if desiredHash != "" {
		patch["desired_hash"] = desiredHash
	}
	_, err := s.db.Exec(ctx, `update service_accesses
		set metadata_json = metadata_json || $2::jsonb,
			updated_at=now()
		where id=$1`, accessID, mustJSON(patch))
	return err
}

func (s *Store) disableClientAccessGroupMaterialization(ctx context.Context, group domain.ClientAccessGroup, clientIDs []string, keepInstances []domain.Instance, desiredHash string) (int, error) {
	args := []any{group.ID}
	where := []string{`coalesce(sa.metadata_json->>'access_group_id','')=$1`, `sa.status in ('pending','active')`}
	if len(clientIDs) > 0 {
		args = append(args, uniqueStrings(clientIDs))
		where = append(where, fmt.Sprintf(`sa.client_account_id::text = any($%d)`, len(args)))
	}
	if len(keepInstances) > 0 {
		instanceIDs := make([]string, 0, len(keepInstances))
		for _, instance := range keepInstances {
			instanceIDs = append(instanceIDs, instance.ID)
		}
		args = append(args, uniqueStrings(instanceIDs))
		where = append(where, fmt.Sprintf(`not (sa.instance_id::text = any($%d))`, len(args)))
	}
	patch := map[string]any{"desired_hash": desiredHash}
	args = append(args, mustJSON(patch))
	tag, err := s.db.Exec(ctx, `update service_accesses sa
		set status='disabled',
			metadata_json=metadata_json || $`+fmt.Sprint(len(args))+`::jsonb,
			updated_at=now()
		where `+strings.Join(where, " and "), args...)
	return int(tag.RowsAffected()), err
}

func (s *Store) resolveClientAccessGroupInstances(ctx context.Context, group domain.ClientAccessGroup) ([]domain.Instance, error) {
	if strings.EqualFold(group.Status, "deleted") || group.DeletedAt != nil {
		return nil, nil
	}
	switch normalizeClientAccessGroupServiceCode(group.ServiceCode) {
	case "vless":
		return s.resolveVLESSAccessGroupInstances(ctx, group)
	default:
		return nil, nil
	}
}

func (s *Store) resolveVLESSAccessGroupInstances(ctx context.Context, group domain.ClientAccessGroup) ([]domain.Instance, error) {
	candidates, err := s.listXrayVLESSGroupSyncInstances(ctx)
	if err != nil {
		return nil, err
	}
	include, exclude, err := s.clientAccessGroupScopeSets(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	mode := normalizeClientAccessGroupScopeMode(group.ScopeMode)
	out := make([]domain.Instance, 0, len(candidates))
	for _, instance := range candidates {
		if mode == "selected_instances" {
			if _, ok := include[instance.ID]; !ok {
				continue
			}
		}
		if mode == "all_except_selected" {
			if _, ok := exclude[instance.ID]; ok {
				continue
			}
		}
		spec, err := s.ensureXrayProvisioningGroups(ctx, instance.ID, instance.Spec)
		if err != nil {
			return nil, err
		}
		instance.Spec = spec
		for _, candidate := range xrayVLESSGroups(spec) {
			if candidate.Key == group.GroupKey && vlessGroupUsableForNewMembership(candidate) {
				out = append(out, instance)
				break
			}
		}
	}
	return out, nil
}

func (s *Store) clientAccessGroupScopeSets(ctx context.Context, groupID string) (map[string]struct{}, map[string]struct{}, error) {
	rows, err := s.db.Query(ctx, `select instance_id::text, mode from client_access_group_instance_scopes where group_id=$1`, groupID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	include := map[string]struct{}{}
	exclude := map[string]struct{}{}
	for rows.Next() {
		var instanceID string
		var mode string
		if err := rows.Scan(&instanceID, &mode); err != nil {
			return nil, nil, err
		}
		if mode == "include" {
			include[instanceID] = struct{}{}
		} else if mode == "exclude" {
			exclude[instanceID] = struct{}{}
		}
	}
	return include, exclude, rows.Err()
}

func (s *Store) clientAccessGroupAllowsInstance(ctx context.Context, group domain.ClientAccessGroup, instanceID string) (bool, error) {
	mode := normalizeClientAccessGroupScopeMode(group.ScopeMode)
	if mode == "all_active_instances" {
		return true, nil
	}
	include, exclude, err := s.clientAccessGroupScopeSets(ctx, group.ID)
	if err != nil {
		return false, err
	}
	switch mode {
	case "selected_instances":
		_, ok := include[instanceID]
		return ok, nil
	case "all_except_selected":
		_, blocked := exclude[instanceID]
		return !blocked, nil
	default:
		return true, nil
	}
}

func (s *Store) syncClientAccessGroupRuntime(ctx context.Context, group domain.ClientAccessGroup, previousInstances []domain.Instance, forceDisable, forceApply bool, reason string) (domain.ClientAccessGroupMembershipResult, error) {
	result := domain.ClientAccessGroupMembershipResult{
		GroupID:     group.ID,
		GroupKey:    group.GroupKey,
		ServiceCode: group.ServiceCode,
	}
	if forceDisable || !strings.EqualFold(group.Status, "active") || group.DeletedAt != nil {
		materializedInstances, err := s.clientAccessGroupMaterializedInstances(ctx, group.ID)
		if err != nil {
			return result, err
		}
		applyTargets := uniqueClientAccessGroupInstances(previousInstances, materializedInstances)
		result.AffectedInstances = len(applyTargets)
		desiredHash := clientAccessGroupDisabledDesiredHash(group)
		disabled, err := s.disableClientAccessGroupMaterialization(ctx, group, nil, nil, desiredHash)
		if err != nil {
			return result, err
		}
		result.MaterializedDisabled = disabled
		if forceApply || disabled > 0 {
			s.queueClientAccessGroupApplyJobs(ctx, group, applyTargets, desiredHash, reason, &result)
		}
		return result, nil
	}

	instances, err := s.resolveClientAccessGroupInstances(ctx, group)
	if err != nil {
		return result, err
	}
	result.AffectedInstances = len(instances)
	desiredHash, err := s.clientAccessGroupDesiredHash(ctx, group, instances)
	if err != nil {
		return result, err
	}
	clientIDs, err := s.activeClientAccessGroupMemberIDs(ctx, group.ID)
	if err != nil {
		return result, err
	}
	if normalizeClientAccessGroupServiceCode(group.ServiceCode) == "vless" && len(clientIDs) > 0 {
		created, updated, failures := s.materializeClientAccessGroupVLESS(ctx, group, clientIDs, instances, desiredHash)
		result.MaterializedCreated = created
		result.MaterializedUpdated = updated
		for _, failure := range failures {
			result.Failed = append(result.Failed, domain.ClientAccessGroupMembershipFailure{ClientID: failure.ClientID, Error: failure.Error})
		}
	} else if len(clientIDs) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("materialization for %s groups is not implemented yet", group.ServiceCode))
	}
	disabled, err := s.disableClientAccessGroupMaterialization(ctx, group, nil, instances, desiredHash)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	} else {
		result.MaterializedDisabled = disabled
	}
	if forceApply || result.MaterializedCreated > 0 || result.MaterializedUpdated > 0 || result.MaterializedDisabled > 0 {
		applyTargets := uniqueClientAccessGroupInstances(previousInstances, instances)
		s.queueClientAccessGroupApplyJobs(ctx, group, applyTargets, desiredHash, reason, &result)
	}
	return result, nil
}

func (s *Store) clientAccessGroupMaterializedInstances(ctx context.Context, groupID string) ([]domain.Instance, error) {
	rows, err := s.db.Query(ctx, `select distinct sa.instance_id::text
		from service_accesses sa
		join instances i on i.id=sa.instance_id
		where coalesce(sa.metadata_json->>'access_group_id','')=$1
		  and sa.status in ('pending','active')
		  and i.status <> 'deleted'
		order by sa.instance_id::text asc`, strings.TrimSpace(groupID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Instance{}
	for rows.Next() {
		var instanceID string
		if err := rows.Scan(&instanceID); err != nil {
			return nil, err
		}
		out = append(out, domain.Instance{ID: instanceID})
	}
	return out, rows.Err()
}

func (s *Store) queueClientAccessGroupApplyJobs(ctx context.Context, group domain.ClientAccessGroup, instances []domain.Instance, desiredHash, reason string, result *domain.ClientAccessGroupMembershipResult) {
	for _, instance := range uniqueClientAccessGroupInstances(instances) {
		job, err := s.UpdateInstanceStatus(ctx, instance.ID, "apply")
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("instance %s apply was not queued: %s", instance.ID, err.Error()))
			if desiredHash != "" {
				_ = s.upsertClientAccessGroupSyncState(ctx, group.ID, instance.ID, desiredHash, "", "failed", err.Error())
			}
			continue
		}
		result.ApplyJobIDs = append(result.ApplyJobIDs, job.ID)
		if desiredHash != "" {
			_ = s.upsertClientAccessGroupSyncState(ctx, group.ID, instance.ID, desiredHash, job.ID, "queued", "")
		}
		_ = s.AddJobLog(ctx, job.ID, "info", firstString(reason, "client access group runtime synchronized"), map[string]any{
			"group_id":              group.ID,
			"group_key":             group.GroupKey,
			"service":               group.ServiceCode,
			"materialized_created":  result.MaterializedCreated,
			"materialized_updated":  result.MaterializedUpdated,
			"materialized_disabled": result.MaterializedDisabled,
			"failed":                len(result.Failed),
		})
	}
	result.ApplyJobCount = len(result.ApplyJobIDs)
}

func uniqueClientAccessGroupInstances(lists ...[]domain.Instance) []domain.Instance {
	byID := map[string]domain.Instance{}
	ids := []string{}
	for _, list := range lists {
		for _, instance := range list {
			instanceID := strings.TrimSpace(instance.ID)
			if instanceID == "" {
				continue
			}
			if _, ok := byID[instanceID]; ok {
				continue
			}
			byID[instanceID] = instance
			ids = append(ids, instanceID)
		}
	}
	sort.Strings(ids)
	out := make([]domain.Instance, 0, len(ids))
	for _, instanceID := range ids {
		out = append(out, byID[instanceID])
	}
	return out
}

func clientAccessGroupDisabledDesiredHash(group domain.ClientAccessGroup) string {
	payload := map[string]any{
		"group_id":     group.ID,
		"service_code": group.ServiceCode,
		"group_key":    group.GroupKey,
		"status":       group.Status,
		"deleted_at":   group.DeletedAt,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Store) clientAccessGroupDesiredHash(ctx context.Context, group domain.ClientAccessGroup, instances []domain.Instance) (string, error) {
	memberIDs, err := s.activeClientAccessGroupMemberIDs(ctx, group.ID)
	if err != nil {
		return "", err
	}
	instanceIDs := make([]string, 0, len(instances))
	for _, instance := range instances {
		instanceIDs = append(instanceIDs, instance.ID)
	}
	sort.Strings(instanceIDs)
	scope, err := s.GetClientAccessGroupScope(ctx, group.ID)
	if err != nil {
		return "", err
	}
	payload := map[string]any{
		"group_id":     group.ID,
		"service_code": group.ServiceCode,
		"group_key":    group.GroupKey,
		"status":       group.Status,
		"policy_json":  json.RawMessage(group.PolicyJSON),
		"scope_mode":   group.ScopeMode,
		"auto_apply":   group.AutoApplyNewInstances,
		"include":      scope.IncludeInstanceIDs,
		"exclude":      scope.ExcludeInstanceIDs,
		"members":      memberIDs,
		"instances":    instanceIDs,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Store) activeClientAccessGroupMemberIDs(ctx context.Context, groupID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `select client_account_id::text
		from client_access_group_memberships
		where group_id=$1 and status='active'
		order by client_account_id::text asc`, groupID)
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

func (s *Store) upsertClientAccessGroupSyncState(ctx context.Context, groupID, instanceID, desiredHash, jobID, status, lastError string) error {
	_, err := s.db.Exec(ctx, `insert into client_access_group_sync_state(
			group_id,instance_id,desired_hash,last_job_id,status,last_error,updated_at
		) values($1,$2,$3,nullif($4,'')::uuid,$5,$6,now())
		on conflict(group_id, instance_id) do update set
			desired_hash=excluded.desired_hash,
			last_job_id=excluded.last_job_id,
			status=excluded.status,
			last_error=excluded.last_error,
			updated_at=now()`,
		groupID, instanceID, desiredHash, strings.TrimSpace(jobID), status, strings.TrimSpace(lastError))
	return err
}

func (s *Store) queryClientAccessGroupAvailableClients(ctx context.Context, serviceCode, search, assignment, clientStatus string, limit, offset int) ([]domain.ClientAccessGroupMember, int, error) {
	where := []string{`ca.status <> 'deleted'`}
	args := []any{serviceCode}
	switch strings.ToLower(strings.TrimSpace(assignment)) {
	case "", "unassigned":
		where = append(where, `m.id is null`)
	case "assigned":
		where = append(where, `m.id is not null`)
	case "all":
	default:
		return nil, 0, fmt.Errorf("unsupported assignment filter %q", assignment)
	}
	if q := strings.TrimSpace(search); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, fmt.Sprintf(`(lower(ca.username) like $%d or lower(coalesce(ca.display_name,'')) like $%d or lower(coalesce(ca.email,'')) like $%d or lower(ca.id::text) like $%d)`, len(args), len(args), len(args), len(args)))
	}
	if st := strings.ToLower(strings.TrimSpace(clientStatus)); st != "" && st != "all" {
		args = append(args, st)
		where = append(where, fmt.Sprintf(`lower(ca.status)=$%d`, len(args)))
	}
	base := ` from client_accounts ca
		left join client_access_group_memberships m on m.client_account_id=ca.id
			and m.service_code=$1
			and m.status in ('active','disabled')
		left join client_access_groups cag on cag.id=m.group_id
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
			coalesce(m.id::text,''),
			coalesce(m.status,''),
			coalesce(cag.id::text,''),
			coalesce(cag.group_key,''),
			coalesce(cag.display_name,''),
			''::text,
			''::text,
			''::text,
			m.updated_at
		`+base+`
		order by ca.username asc, ca.created_at asc
		limit $`+fmt.Sprint(len(args)-1)+` offset $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanClientAccessGroupMemberRows(rows)
	return items, total, err
}

func (s *Store) queryClientAccessGroupAvailableClientIDs(ctx context.Context, serviceCode, search, assignment, clientStatus string) ([]string, error) {
	where := []string{`ca.status <> 'deleted'`}
	args := []any{serviceCode}
	switch strings.ToLower(strings.TrimSpace(assignment)) {
	case "", "unassigned":
		where = append(where, `m.id is null`)
	case "assigned":
		where = append(where, `m.id is not null`)
	case "all":
	default:
		return nil, fmt.Errorf("unsupported assignment filter %q", assignment)
	}
	if q := strings.TrimSpace(search); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		where = append(where, fmt.Sprintf(`(lower(ca.username) like $%d or lower(coalesce(ca.display_name,'')) like $%d or lower(coalesce(ca.email,'')) like $%d or lower(ca.id::text) like $%d)`, len(args), len(args), len(args), len(args)))
	}
	if st := strings.ToLower(strings.TrimSpace(clientStatus)); st != "" && st != "all" {
		args = append(args, st)
		where = append(where, fmt.Sprintf(`lower(ca.status)=$%d`, len(args)))
	}
	rows, err := s.db.Query(ctx, `select ca.id::text
		from client_accounts ca
		left join client_access_group_memberships m on m.client_account_id=ca.id
			and m.service_code=$1
			and m.status in ('active','disabled')
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

func (s *Store) resolveAccessGroupClientRefs(ctx context.Context, clientIDs, refs []string) ([]string, []domain.ClientAccessGroupMembershipFailure, error) {
	ids, failures, err := s.resolveVLESSMembershipClientRefs(ctx, clientIDs, refs)
	if err != nil {
		return nil, nil, err
	}
	outFailures := make([]domain.ClientAccessGroupMembershipFailure, 0, len(failures))
	for _, failure := range failures {
		outFailures = append(outFailures, domain.ClientAccessGroupMembershipFailure{
			ClientID: failure.ClientID,
			Ref:      failure.Ref,
			Error:    failure.Error,
		})
	}
	return ids, outFailures, nil
}

func (s *Store) loadClientAccessGroupClient(ctx context.Context, clientID string) (domain.Client, error) {
	var client domain.Client
	err := s.db.QueryRow(ctx, `select id::text,username,coalesce(display_name,''),coalesce(email,''),status,coalesce(notes,''),expires_at,created_at,updated_at
		from client_accounts
		where id=$1`, strings.TrimSpace(clientID)).Scan(&client.ID, &client.Username, &client.DisplayName, &client.Email, &client.Status, &client.Notes, &client.ExpiresAt, &client.CreatedAt, &client.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Client{}, fmt.Errorf("client not found")
	}
	return client, err
}

func (s *Store) existingClientAccessGroupMember(ctx context.Context, clientID, serviceCode string) (domain.ClientAccessGroupMember, error) {
	var item domain.ClientAccessGroupMember
	err := s.db.QueryRow(ctx, `select
			ca.id::text,
			ca.username,
			coalesce(ca.display_name,''),
			coalesce(ca.email,''),
			ca.status,
			m.id::text,
			m.status,
			cag.id::text,
			cag.group_key,
			cag.display_name,
			''::text,
			''::text,
			''::text,
			m.updated_at
		from client_access_group_memberships m
		join client_access_groups cag on cag.id=m.group_id
		join client_accounts ca on ca.id=m.client_account_id
		where m.client_account_id=$1
		  and m.service_code=$2
		  and m.status in ('active','disabled')`, strings.TrimSpace(clientID), normalizeClientAccessGroupServiceCode(serviceCode)).Scan(
		&item.ClientID,
		&item.Username,
		&item.DisplayName,
		&item.Email,
		&item.ClientStatus,
		&item.MembershipID,
		&item.MembershipStatus,
		&item.GroupID,
		&item.GroupKey,
		&item.GroupName,
		&item.ServiceAccessID,
		&item.AccessStatus,
		&item.XrayUUID,
		&item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClientAccessGroupMember{}, nil
	}
	return item, err
}

func (s *Store) getMutableClientAccessGroup(ctx context.Context, groupID string) (domain.ClientAccessGroup, error) {
	group, err := s.getClientAccessGroup(ctx, groupID)
	if err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if strings.EqualFold(group.Status, "deleted") || group.DeletedAt != nil {
		return domain.ClientAccessGroup{}, domain.ErrClientAccessGroupNotFound
	}
	if !strings.EqualFold(group.Status, "active") {
		return domain.ClientAccessGroup{}, fmt.Errorf("client access group %q is not active", group.GroupKey)
	}
	return group, nil
}

func (s *Store) getClientAccessGroup(ctx context.Context, groupID string) (domain.ClientAccessGroup, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return domain.ClientAccessGroup{}, domain.ErrClientAccessGroupNotFound
	}
	row := s.db.QueryRow(ctx, `select
			id::text,service_code,group_key,display_name,coalesce(description,''),status,policy_json,
			scope_mode,auto_apply_new_instances,
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status in ('active','disabled')),
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status='active'),
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status='disabled'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status in ('pending','queued')),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status='failed'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status='applied'),
			created_at,updated_at,deleted_at
		from client_access_groups
		where id=$1`, groupID)
	group, err := scanClientAccessGroup(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClientAccessGroup{}, domain.ErrClientAccessGroupNotFound
	}
	return group, err
}

func (s *Store) getClientAccessGroupByServiceKey(ctx context.Context, serviceCode, groupKey string) (domain.ClientAccessGroup, error) {
	serviceCode = normalizeClientAccessGroupServiceCode(serviceCode)
	groupKey = normalizeVLESSGroupTemplateKey(groupKey)
	row := s.db.QueryRow(ctx, `select
			id::text,service_code,group_key,display_name,coalesce(description,''),status,policy_json,
			scope_mode,auto_apply_new_instances,
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status in ('active','disabled')),
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status='active'),
			(select count(*)::int from client_access_group_memberships m where m.group_id=client_access_groups.id and m.status='disabled'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status in ('pending','queued')),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status='failed'),
			(select count(*)::int from client_access_group_sync_state ss where ss.group_id=client_access_groups.id and ss.status='applied'),
			created_at,updated_at,deleted_at
		from client_access_groups
		where service_code=$1 and group_key=$2 and deleted_at is null`, serviceCode, groupKey)
	group, err := scanClientAccessGroup(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ClientAccessGroup{}, domain.ErrClientAccessGroupNotFound
	}
	return group, err
}

func (s *Store) ensureClientAccessGroupFromVLESSTemplate(ctx context.Context, template domain.VLESSGroupTemplate) (domain.ClientAccessGroup, error) {
	group, err := s.getClientAccessGroupByServiceKey(ctx, "vless", template.Key)
	if err == nil {
		return group, nil
	}
	if !errors.Is(err, domain.ErrClientAccessGroupNotFound) {
		return domain.ClientAccessGroup{}, err
	}
	policy := vlessTemplatePolicyJSON(template)
	auto := true
	return s.CreateClientAccessGroup(ctx, domain.ClientAccessGroupInput{
		ServiceCode:           "vless",
		GroupKey:              template.Key,
		DisplayName:           template.Label,
		Description:           template.Description,
		Status:                firstString(template.Status, "active"),
		PolicyJSON:            policy,
		ScopeMode:             "all_active_instances",
		AutoApplyNewInstances: &auto,
	}, nil)
}

func (s *Store) mirrorClientAccessGroupToLegacyVLESSCatalog(ctx context.Context, group domain.ClientAccessGroup) error {
	if normalizeClientAccessGroupServiceCode(group.ServiceCode) != "vless" {
		return nil
	}
	template := clientAccessGroupToVLESSTemplate(group)
	_, err := s.UpsertVLESSGroupTemplate(ctx, template)
	return err
}

type clientAccessGroupRowScanner interface {
	Scan(dest ...any) error
}

type clientAccessGroupRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanClientAccessGroupRows(rows clientAccessGroupRows) ([]domain.ClientAccessGroup, error) {
	out := []domain.ClientAccessGroup{}
	for rows.Next() {
		group, err := scanClientAccessGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, group)
	}
	return out, rows.Err()
}

func scanClientAccessGroup(row clientAccessGroupRowScanner) (domain.ClientAccessGroup, error) {
	var group domain.ClientAccessGroup
	var policyRaw []byte
	if err := row.Scan(
		&group.ID,
		&group.ServiceCode,
		&group.GroupKey,
		&group.DisplayName,
		&group.Description,
		&group.Status,
		&policyRaw,
		&group.ScopeMode,
		&group.AutoApplyNewInstances,
		&group.MemberCount,
		&group.ActiveMemberCount,
		&group.DisabledMemberCount,
		&group.AffectedInstances,
		&group.PendingSyncCount,
		&group.FailedSyncCount,
		&group.AppliedSyncCount,
		&group.CreatedAt,
		&group.UpdatedAt,
		&group.DeletedAt,
	); err != nil {
		return domain.ClientAccessGroup{}, err
	}
	if len(strings.TrimSpace(string(policyRaw))) == 0 {
		policyRaw = []byte(`{}`)
	}
	group.PolicyJSON = json.RawMessage(policyRaw)
	return group, nil
}

type clientAccessGroupMemberRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanClientAccessGroupMemberRows(rows clientAccessGroupMemberRows) ([]domain.ClientAccessGroupMember, error) {
	out := []domain.ClientAccessGroupMember{}
	for rows.Next() {
		var item domain.ClientAccessGroupMember
		if err := rows.Scan(
			&item.ClientID,
			&item.Username,
			&item.DisplayName,
			&item.Email,
			&item.ClientStatus,
			&item.MembershipID,
			&item.MembershipStatus,
			&item.GroupID,
			&item.GroupKey,
			&item.GroupName,
			&item.ServiceAccessID,
			&item.AccessStatus,
			&item.XrayUUID,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type normalizedClientAccessGroupInput struct {
	domain.ClientAccessGroupInput
	autoApply bool
}

func (in normalizedClientAccessGroupInput) autoApplyValue() bool {
	return in.autoApply
}

func normalizeClientAccessGroupInput(input domain.ClientAccessGroupInput, create bool) (normalizedClientAccessGroupInput, error) {
	out := normalizedClientAccessGroupInput{ClientAccessGroupInput: input}
	out.ServiceCode = normalizeClientAccessGroupServiceCode(out.ServiceCode)
	out.GroupKey = normalizeVLESSGroupTemplateKey(out.GroupKey)
	out.DisplayName = strings.TrimSpace(out.DisplayName)
	out.Description = strings.TrimSpace(out.Description)
	out.Status = strings.ToLower(strings.TrimSpace(out.Status))
	out.ScopeMode = normalizeClientAccessGroupScopeMode(out.ScopeMode)
	if out.ServiceCode == "" {
		return out, fmt.Errorf("service_code is required")
	}
	if out.GroupKey == "" {
		return out, fmt.Errorf("group_key is required")
	}
	if out.DisplayName == "" {
		return out, fmt.Errorf("display_name is required")
	}
	if out.Status == "" {
		out.Status = "active"
	}
	if !in(out.Status, "active", "disabled", "deleted") {
		return out, fmt.Errorf("invalid status %q", out.Status)
	}
	if out.ScopeMode == "" {
		out.ScopeMode = "all_active_instances"
	}
	out.autoApply = true
	if out.AutoApplyNewInstances != nil {
		out.autoApply = *out.AutoApplyNewInstances
	}
	if len(strings.TrimSpace(string(out.PolicyJSON))) == 0 {
		out.PolicyJSON = []byte(`{}`)
	}
	var policy map[string]any
	if err := json.Unmarshal(out.PolicyJSON, &policy); err != nil {
		return out, fmt.Errorf("invalid policy_json: %w", err)
	}
	if policy == nil {
		out.PolicyJSON = []byte(`{}`)
	}
	if create && out.Status == "deleted" {
		return out, fmt.Errorf("new access group cannot be created as deleted")
	}
	return out, nil
}

func normalizeClientAccessGroupServiceCode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return ""
	case "xray", "xray-core", "vless":
		return "vless"
	case "ipsec", "l2tpd", "xl2tpd", "l2tp":
		return "l2tp"
	case "wg", "wireguard":
		return "wireguard"
	case "ss", "shadowsocks":
		return "shadowsocks"
	case "http", "http-proxy", "http_proxy":
		return "http_proxy"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeClientAccessGroupScopeMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all", "all_active", "all_active_instances":
		return "all_active_instances"
	case "selected", "selected_instances":
		return "selected_instances"
	case "all_except", "all_except_selected":
		return "all_except_selected"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func vlessTemplatePolicyJSON(template domain.VLESSGroupTemplate) json.RawMessage {
	raw := mustJSON(map[string]any{
		"access_mode":        template.AccessMode,
		"egress_mode":        template.EgressMode,
		"egress_node_id":     template.EgressNodeID,
		"target_instance_id": template.TargetInstanceID,
		"outbound_tag":       template.OutboundTag,
		"ad_block":           template.AdBlock,
		"rules":              template.Rules,
		"extra_rules":        template.ExtraRules,
	})
	return json.RawMessage(raw)
}

func clientAccessGroupToVLESSTemplate(group domain.ClientAccessGroup) domain.VLESSGroupTemplate {
	var policy map[string]any
	_ = json.Unmarshal(group.PolicyJSON, &policy)
	if policy == nil {
		policy = map[string]any{}
	}
	template := domain.VLESSGroupTemplate{
		Key:              group.GroupKey,
		Label:            firstString(group.DisplayName, group.GroupKey),
		Description:      group.Description,
		AccessMode:       firstString(policy["access_mode"], "instance_default"),
		EgressMode:       firstString(policy["egress_mode"], "default"),
		EgressNodeID:     firstString(policy["egress_node_id"]),
		TargetInstanceID: firstString(policy["target_instance_id"]),
		OutboundTag:      firstString(policy["outbound_tag"], "direct"),
		AdBlock:          boolFromAny(policy["ad_block"]),
		Status:           firstString(group.Status, "active"),
		Source:           "client_access_group",
		Version:          1,
		DisplayOrder:     100,
	}
	template.Rules = mapSliceFromAny(policy["rules"])
	template.ExtraRules = mapSliceFromAny(policy["extra_rules"])
	return template
}

func mapSliceFromAny(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]map[string]any); ok {
			return typed
		}
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func derefUserID(userID *string) string {
	if userID == nil {
		return ""
	}
	return strings.TrimSpace(*userID)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeClientAccessGroupPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultClientAccessGroupMembershipLimit
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func boolFromAny(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true") || strings.TrimSpace(v) == "1"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}
