package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/rtis-emc2/megavpn/internal/accesspolicy"
	"github.com/rtis-emc2/megavpn/internal/domain"
)

// availableVLESSAccessPolicies adapts canonical client access groups to the
// Xray policy shape used while creating an instance.
func (s *Server) availableVLESSAccessPolicies(ctx context.Context) ([]domain.VLESSAccessPolicy, error) {
	groups, err := s.store.ListClientAccessGroups(ctx, "vless")
	if err != nil {
		return nil, err
	}
	policies := make([]domain.VLESSAccessPolicy, 0, len(groups))
	for _, group := range groups {
		if !strings.EqualFold(strings.TrimSpace(group.Status), "active") {
			continue
		}
		policy, err := accesspolicy.DecodeVLESS(group)
		if err != nil {
			return nil, fmt.Errorf("client access group %q: %w", group.GroupKey, err)
		}
		policies = append(policies, policy)
	}
	if len(policies) == 0 {
		return nil, fmt.Errorf("no active VLESS client access groups are configured")
	}
	return policies, nil
}

func cloneRulesHTTP(in []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, item := range in {
		out = append(out, cloneMapHTTP(item))
	}
	return out
}

func vlessAccessPoliciesAsSpec(policies []domain.VLESSAccessPolicy) []any {
	out := make([]any, 0, len(policies))
	for _, policy := range policies {
		if strings.TrimSpace(policy.Key) == "" || !strings.EqualFold(strings.TrimSpace(policy.Status), "active") {
			continue
		}
		group := map[string]any{
			"key":          policy.Key,
			"label":        policy.Label,
			"access_mode":  policy.AccessMode,
			"egress_mode":  policy.EgressMode,
			"outbound_tag": policy.OutboundTag,
		}
		if policy.Description != "" {
			group["description"] = policy.Description
		}
		if policy.EgressNodeID != "" {
			group["egress_node_id"] = policy.EgressNodeID
		}
		if policy.TargetInstanceID != "" {
			group["target_instance_id"] = policy.TargetInstanceID
		}
		if policy.AdBlock {
			group["ad_block"] = true
		}
		rules := cloneRulesHTTP(policy.Rules)
		if len(policy.ExtraRules) > 0 {
			rules = append(rules, cloneRulesHTTP(policy.ExtraRules)...)
			group["extra_rules"] = cloneRulesHTTP(policy.ExtraRules)
		}
		if len(rules) > 0 {
			group["rules"] = rules
		}
		out = append(out, group)
	}
	return out
}

func ensureVLESSDefaultGroup(spec map[string]any, policies []domain.VLESSAccessPolicy) error {
	if spec == nil {
		return fmt.Errorf("instance spec is required")
	}
	groups := vlessAccessPoliciesAsSpec(policies)
	if len(groups) == 0 {
		return fmt.Errorf("no active VLESS access policies are available")
	}
	spec["vless_groups"] = groups
	current := strings.TrimSpace(firstStringHTTP(spec["default_vless_group"], spec["default_xray_group"], spec["default_outbound_group"]))
	for _, item := range groups {
		group, _ := item.(map[string]any)
		key := firstStringHTTP(group["key"])
		if current != "" && key == current {
			spec["default_vless_group"] = current
			return nil
		}
	}
	first, _ := groups[0].(map[string]any)
	spec["default_vless_group"] = firstStringHTTP(first["key"])
	return nil
}
