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

// upsertInstanceVLESSAccessProjection materializes canonical group membership
// into the per-instance service access consumed by the Xray runtime.
func (s *Store) upsertInstanceVLESSAccessProjection(ctx context.Context, instanceID, clientID, groupKey, mode string) (string, bool, domain.ClientAccessGroupMember, error) {
	client, err := s.loadClientAccessGroupClient(ctx, clientID)
	if err != nil {
		return "", false, domain.ClientAccessGroupMember{}, err
	}
	if strings.EqualFold(client.Status, "deleted") {
		return "", false, domain.ClientAccessGroupMember{}, fmt.Errorf("client is deleted")
	}

	existing, err := s.existingInstanceVLESSAccessProjection(ctx, instanceID, client.ID)
	if err != nil {
		return "", false, domain.ClientAccessGroupMember{}, err
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
		return "", false, domain.ClientAccessGroupMember{}, err
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
		return "", false, domain.ClientAccessGroupMember{}, err
	}
	if _, err := s.EnsureXrayServiceAccessUUID(ctx, accessID); err != nil {
		return "", false, domain.ClientAccessGroupMember{}, err
	}
	if err := s.ensureBaselineClientAccessRoute(ctx, client.ID, accessID, instanceID); err != nil {
		return "", false, domain.ClientAccessGroupMember{}, err
	}
	updated, err := s.existingInstanceVLESSAccessProjection(ctx, instanceID, client.ID)
	if err != nil {
		return "", false, domain.ClientAccessGroupMember{}, err
	}
	return changeKind, false, updated, nil
}

func (s *Store) existingInstanceVLESSAccessProjection(ctx context.Context, instanceID, clientID string) (domain.ClientAccessGroupMember, error) {
	var item domain.ClientAccessGroupMember
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
		return domain.ClientAccessGroupMember{}, nil
	}
	if err != nil {
		return domain.ClientAccessGroupMember{}, err
	}
	item.GroupKey = normalizeXrayVLESSGroupKey(item.GroupKey)
	return item, nil
}

func (s *Store) materializeVLESSAccessGroupsForInstance(ctx context.Context, instanceID string) (int, error) {
	instance, policies, err := s.loadInstanceVLESSAccessPolicies(ctx, instanceID)
	if err != nil {
		return 0, err
	}
	keys := make([]string, 0, len(policies))
	for key := range policies {
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
		var clientID, groupKey string
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
		desiredHash, hashErr := s.clientAccessGroupDesiredHash(ctx, group, []domain.Instance{instance})
		if hashErr != nil {
			desiredHash = ""
		}
		change, skipped, member, err := s.upsertInstanceVLESSAccessProjection(ctx, instance.ID, clientID, groupKey, "add_or_move")
		if err != nil {
			return materialized, err
		}
		if member.ServiceAccessID != "" {
			if err := s.markServiceAccessClientAccessGroup(ctx, member.ServiceAccessID, group, desiredHash); err != nil {
				return materialized, fmt.Errorf("mark service access %s for client access group %s: %w", member.ServiceAccessID, group.GroupKey, err)
			}
		}
		if !skipped && (change == "created" || change == "updated") {
			materialized++
		}
	}
	return materialized, rows.Err()
}

func (s *Store) resolveAccessGroupClientRefs(ctx context.Context, clientIDs, refs []string) ([]string, []domain.ClientAccessGroupMembershipFailure, error) {
	seen := map[string]struct{}{}
	resolved := []string{}
	failures := []domain.ClientAccessGroupMembershipFailure{}
	add := func(clientID string) {
		clientID = strings.TrimSpace(clientID)
		if clientID == "" {
			return
		}
		if _, exists := seen[clientID]; exists {
			return
		}
		seen[clientID] = struct{}{}
		resolved = append(resolved, clientID)
	}
	for _, clientID := range clientIDs {
		add(clientID)
	}
	for _, raw := range refs {
		for _, ref := range splitClientAccessGroupRefs(raw) {
			clientID, err := s.resolveClientAccessGroupRef(ctx, ref)
			if err != nil {
				failures = append(failures, domain.ClientAccessGroupMembershipFailure{Ref: ref, Error: err.Error()})
				continue
			}
			add(clientID)
		}
	}
	return resolved, failures, nil
}

func mergeClientIDs(primary, extra []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(primary)+len(extra))
	for _, values := range [][]string{primary, extra} {
		for _, clientID := range values {
			clientID = strings.TrimSpace(clientID)
			if clientID == "" {
				continue
			}
			if _, exists := seen[clientID]; exists {
				continue
			}
			seen[clientID] = struct{}{}
			out = append(out, clientID)
		}
	}
	return out
}

func splitClientAccessGroupRefs(raw string) []string {
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

func (s *Store) resolveClientAccessGroupRef(ctx context.Context, ref string) (string, error) {
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
	matches := []string{}
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

func (s *Store) loadInstanceVLESSAccessPolicies(ctx context.Context, instanceID string) (domain.Instance, map[string]xrayVLESSGroup, error) {
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
	out := make(map[string]xrayVLESSGroup, len(groups))
	for _, group := range groups {
		if group.Key != "" && vlessAccessPolicyIsActive(group) {
			out[group.Key] = group
		}
	}
	if len(out) == 0 {
		return domain.Instance{}, nil, fmt.Errorf("service instance %s has no active VLESS access policies", instance.ID)
	}
	return instance, out, nil
}

func vlessAccessPolicyIsActive(group xrayVLESSGroup) bool {
	status := strings.ToLower(strings.TrimSpace(group.Status))
	return status == "" || status == "active"
}
