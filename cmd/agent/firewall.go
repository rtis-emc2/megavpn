package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	firewallNFTFamily       = "inet"
	firewallNFTTable        = "megavpn_firewall"
	firewallLegacyNFTTable  = "megavpn"
	firewallInputChain      = "firewall_input"
	firewallForwardChain    = "firewall_forward"
	firewallOutputChain     = "firewall_output"
	firewallNFTCommentScope = "megavpn:firewall:"
)

type nodeFirewallRule struct {
	ID         string
	Priority   int
	Chain      string
	Action     string
	Protocol   string
	SrcListID  string
	SrcListKey string
	DstListID  string
	DstListKey string
	SrcCIDR    string
	DstCIDR    string
	SrcPorts   string
	DstPorts   string
	StateMatch []string
	Comment    string
	Enabled    bool
	Status     string
}

type nodeFirewallAddressLists struct {
	ByRef   map[string]nodeFirewallAddressList
	Ordered []nodeFirewallAddressList
}

type nodeFirewallAddressList struct {
	ID     string
	Key    string
	NameV4 string
	NameV6 string
	V4     []string
	V6     []string
}

type nodeFirewallListElement struct {
	Family string
	Value  string
}

type nodeFirewallPlan struct {
	Script                   string
	Count                    int
	SystemRuleCount          int
	Hash                     string
	Warnings                 []string
	DefaultPolicyEnforcement string
}

type nodeFirewallDefaultPolicies struct {
	Input   string
	Forward string
	Output  string
	Enforce bool
}

func (c *client) handleNodeFirewallJob(ctx context.Context, j job, st agentState) (string, map[string]any) {
	nodeID := strings.TrimSpace(stringify(j.Payload["node_id"]))
	if nodeID == "" || nodeID != st.NodeID {
		return "failed", map[string]any{"error": "firewall job node_id does not match agent state", "node_id": nodeID}
	}
	payload := firewallPayloadWithAgentSafetyContext(j.Payload, st)
	switch j.Type {
	case "node.firewall.preview":
		plan, err := renderNodeFirewallPlan(payload)
		if err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "render"}
		}
		return "succeeded", map[string]any{"applied": false, "rule_count": plan.Count, "system_rule_count": plan.SystemRuleCount, "rendered_hash": plan.Hash, "warnings": plan.Warnings, "default_policy_enforcement": plan.DefaultPolicyEnforcement, "script": plan.Script}
	case "node.firewall.observe":
		code, out := runInstallCommand(ctx, "nft", "list", "table", firewallNFTFamily, firewallNFTTable)
		result := map[string]any{"observed": code == 0, "output": truncate(out, 4000)}
		if code != 0 {
			result["error"] = "nft table is not available"
			return "failed", result
		}
		return "succeeded", result
	case "node.firewall.disable":
		result := disableNodeFirewall(ctx)
		if stringify(result["status"]) != "disabled" {
			return "failed", result
		}
		return "succeeded", result
	case "node.firewall.apply":
		plan, err := renderNodeFirewallPlan(payload)
		if err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "render"}
		}
		if err := cleanupLegacyFirewallNFTChains(ctx); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "legacy_cleanup"}
		}
		if err := ensureFirewallNFTChains(ctx); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "ensure_chains"}
		}
		out, err := runNFTScript(ctx, plan.Script)
		result := map[string]any{
			"applied":                    err == nil,
			"rule_count":                 plan.Count,
			"system_rule_count":          plan.SystemRuleCount,
			"rendered_hash":              plan.Hash,
			"warnings":                   plan.Warnings,
			"default_policy_enforcement": plan.DefaultPolicyEnforcement,
			"output":                     truncate(out, 4000),
		}
		if err != nil {
			result["error"] = err.Error()
			return "failed", result
		}
		return "succeeded", result
	default:
		return "failed", map[string]any{"error": "unsupported firewall job type"}
	}
}

func disableNodeFirewall(ctx context.Context) map[string]any {
	result := map[string]any{
		"message":                    "managed firewall disabled",
		"status":                     "disabled",
		"managed_table":              firewallNFTFamily + " " + firewallNFTTable,
		"default_policy_enforcement": "disabled",
		"rule_count":                 0,
		"system_rule_count":          0,
	}
	if err := cleanupLegacyFirewallNFTChains(ctx); err != nil {
		result["legacy_cleanup_warning"] = err.Error()
	}
	if code, out := runInstallCommand(ctx, "nft", "list", "table", firewallNFTFamily, firewallNFTTable); code != 0 {
		result["already_disabled"] = true
		result["output"] = truncate(out, 4000)
		return result
	}
	code, out := runInstallCommand(ctx, "nft", "delete", "table", firewallNFTFamily, firewallNFTTable)
	result["output"] = truncate(out, 4000)
	result["delete_exit_code"] = code
	if code != 0 {
		result["status"] = "failed"
		result["error"] = "managed firewall table delete failed: " + firstLine(out)
	}
	return result
}

func firewallPayloadWithAgentSafetyContext(payload map[string]any, st agentState) map[string]any {
	out := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		out[key] = value
	}
	if strings.TrimSpace(stringify(out["agent_control_plane_url"])) == "" && strings.TrimSpace(st.ControlPlaneURL) != "" {
		out["agent_control_plane_url"] = st.ControlPlaneURL
	}
	return out
}

func ensureFirewallNFTChains(ctx context.Context) error {
	if code, out := runInstallCommand(ctx, "nft", "list", "table", firewallNFTFamily, firewallNFTTable); code != 0 {
		if addCode, addOut := runInstallCommand(ctx, "nft", "add", "table", firewallNFTFamily, firewallNFTTable); addCode != 0 {
			return fmt.Errorf("create nft table failed: %s", firstLine(first(addOut, out)))
		}
	}
	for _, spec := range []struct {
		chain string
		hook  string
	}{
		{firewallInputChain, "input"},
		{firewallForwardChain, "forward"},
		{firewallOutputChain, "output"},
	} {
		if code, _ := runInstallCommand(ctx, "nft", "list", "chain", firewallNFTFamily, firewallNFTTable, spec.chain); code == 0 {
			continue
		}
		args := []string{"add", "chain", firewallNFTFamily, firewallNFTTable, spec.chain, "{", "type", "filter", "hook", spec.hook, "priority", "filter", ";", "policy", "accept", ";", "}"}
		if code, out := runInstallCommand(ctx, "nft", args...); code != 0 {
			return fmt.Errorf("create nft chain %s failed: %s", spec.chain, firstLine(out))
		}
	}
	return nil
}

func cleanupLegacyFirewallNFTChains(ctx context.Context) error {
	for _, chain := range []string{firewallInputChain, firewallForwardChain, firewallOutputChain} {
		if code, _ := runInstallCommand(ctx, "nft", "list", "chain", firewallNFTFamily, firewallLegacyNFTTable, chain); code != 0 {
			continue
		}
		if code, out := runInstallCommand(ctx, "nft", "flush", "chain", firewallNFTFamily, firewallLegacyNFTTable, chain); code != 0 {
			return fmt.Errorf("flush legacy nft chain %s failed: %s", chain, firstLine(out))
		}
		if code, out := runInstallCommand(ctx, "nft", "delete", "chain", firewallNFTFamily, firewallLegacyNFTTable, chain); code != 0 {
			return fmt.Errorf("delete legacy nft chain %s failed: %s", chain, firstLine(out))
		}
	}
	return nil
}

func renderNodeFirewallPlan(payload map[string]any) (nodeFirewallPlan, error) {
	defaults, policyWarnings, err := parseNodeFirewallDefaultPolicies(payload)
	if err != nil {
		return nodeFirewallPlan{}, err
	}
	addressLists, warnings, err := parseNodeFirewallAddressLists(payload["address_lists"])
	if err != nil {
		return nodeFirewallPlan{}, err
	}
	warnings = append(warnings, policyWarnings...)
	rules, ruleWarnings, err := parseNodeFirewallRules(payload["rules"])
	if err != nil {
		return nodeFirewallPlan{}, err
	}
	warnings = append(warnings, ruleWarnings...)
	lines := renderNodeFirewallChainResetLines(defaults)
	setLines := renderNodeFirewallAddressListSets(addressLists)
	lines = append(lines, setLines...)
	safetyLines, safetyCount, safetyWarnings, err := renderNodeFirewallSafetyRules(defaults, payload, rules)
	if err != nil {
		return nodeFirewallPlan{}, err
	}
	lines = append(lines, safetyLines...)
	warnings = append(warnings, safetyWarnings...)
	applied := 0
	for _, rule := range rules {
		if !rule.Enabled || rule.Status != "active" {
			continue
		}
		rendered, err := renderNodeFirewallRule(rule, addressLists)
		if err != nil {
			return nodeFirewallPlan{}, err
		}
		if len(rendered) == 0 {
			continue
		}
		applied += len(rendered)
		lines = append(lines, rendered...)
	}
	rejectLines := renderNodeFirewallDefaultRejectRules(defaults)
	lines = append(lines, rejectLines...)
	script := strings.Join(lines, "\n") + "\n"
	sum := sha256.Sum256([]byte(script))
	enforcement := "observe_only"
	if defaults.Enforce {
		enforcement = "enforced"
	}
	return nodeFirewallPlan{Script: script, Count: applied, SystemRuleCount: safetyCount + len(rejectLines), Hash: hex.EncodeToString(sum[:]), Warnings: warnings, DefaultPolicyEnforcement: enforcement}, nil
}

func parseNodeFirewallDefaultPolicies(payload map[string]any) (nodeFirewallDefaultPolicies, []string, error) {
	input, err := normalizeNodeFirewallDefaultPolicy(stringify(payload["default_input_policy"]))
	if err != nil {
		return nodeFirewallDefaultPolicies{}, nil, fmt.Errorf("default_input_policy: %w", err)
	}
	forward, err := normalizeNodeFirewallDefaultPolicy(stringify(payload["default_forward_policy"]))
	if err != nil {
		return nodeFirewallDefaultPolicies{}, nil, fmt.Errorf("default_forward_policy: %w", err)
	}
	output, err := normalizeNodeFirewallDefaultPolicy(stringify(payload["default_output_policy"]))
	if err != nil {
		return nodeFirewallDefaultPolicies{}, nil, fmt.Errorf("default_output_policy: %w", err)
	}
	defaults := nodeFirewallDefaultPolicies{
		Input:   first(input, "accept"),
		Forward: first(forward, "accept"),
		Output:  first(output, "accept"),
		Enforce: boolFromAny(payload["enforce_default_policy"]),
	}
	if defaults.Enforce {
		return defaults, nil, nil
	}
	warnings := []string{}
	if defaults.Input != "accept" || defaults.Forward != "accept" || defaults.Output != "accept" {
		warnings = append(warnings, "default chain policies are present in policy metadata but not enforced for this apply; enable strict enforcement to apply them")
	}
	return defaults, warnings, nil
}

func normalizeNodeFirewallDefaultPolicy(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "accept", nil
	}
	if !inLocal(value, "accept", "drop", "reject") {
		return "", fmt.Errorf("must be accept, drop or reject")
	}
	return value, nil
}

func renderNodeFirewallChainResetLines(defaults nodeFirewallDefaultPolicies) []string {
	return []string{
		"delete table " + firewallNFTFamily + " " + firewallNFTTable,
		"add table " + firewallNFTFamily + " " + firewallNFTTable,
		renderNodeFirewallChainAddLine(firewallInputChain, "input", nftBasePolicy(defaults.Input, defaults.Enforce)),
		renderNodeFirewallChainAddLine(firewallForwardChain, "forward", nftBasePolicy(defaults.Forward, defaults.Enforce)),
		renderNodeFirewallChainAddLine(firewallOutputChain, "output", nftBasePolicy(defaults.Output, defaults.Enforce)),
	}
}

func renderNodeFirewallChainAddLine(chain, hook, policy string) string {
	return fmt.Sprintf("add chain %s %s %s { type filter hook %s priority filter; policy %s; }", firewallNFTFamily, firewallNFTTable, chain, hook, policy)
}

func nftBasePolicy(policy string, enforce bool) string {
	if !enforce {
		return "accept"
	}
	if policy == "accept" {
		return "accept"
	}
	return "drop"
}

func renderNodeFirewallSafetyRules(defaults nodeFirewallDefaultPolicies, payload map[string]any, rules []nodeFirewallRule) ([]string, int, []string, error) {
	if !defaults.Enforce {
		return nil, 0, nil, nil
	}
	lines := []string{}
	warnings := []string{}
	if defaults.Input != "accept" {
		lines = append(lines,
			fmt.Sprintf("add rule %s %s %s ct state { established, related } accept comment %s", firewallNFTFamily, firewallNFTTable, firewallInputChain, nftQuote(firewallNFTCommentScope+"system-established-input")),
			fmt.Sprintf("add rule %s %s %s iifname %s accept comment %s", firewallNFTFamily, firewallNFTTable, firewallInputChain, nftQuote("lo"), nftQuote(firewallNFTCommentScope+"system-loopback-input")),
		)
	}
	if defaults.Forward != "accept" {
		lines = append(lines,
			fmt.Sprintf("add rule %s %s %s ct state { established, related } accept comment %s", firewallNFTFamily, firewallNFTTable, firewallForwardChain, nftQuote(firewallNFTCommentScope+"system-established-forward")),
		)
	}
	if defaults.Output != "accept" {
		controlPlaneLine, warning, err := renderNodeFirewallControlPlaneOutputRule(payload, rules)
		if err != nil {
			return nil, 0, nil, err
		}
		lines = append(lines,
			fmt.Sprintf("add rule %s %s %s ct state { established, related } accept comment %s", firewallNFTFamily, firewallNFTTable, firewallOutputChain, nftQuote(firewallNFTCommentScope+"system-established-output")),
			fmt.Sprintf("add rule %s %s %s oifname %s accept comment %s", firewallNFTFamily, firewallNFTTable, firewallOutputChain, nftQuote("lo"), nftQuote(firewallNFTCommentScope+"system-loopback-output")),
		)
		if controlPlaneLine != "" {
			lines = append(lines, controlPlaneLine)
		}
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	return lines, len(lines), warnings, nil
}

func renderNodeFirewallControlPlaneOutputRule(payload map[string]any, rules []nodeFirewallRule) (string, string, error) {
	raw := strings.TrimSpace(stringify(payload["agent_control_plane_url"]))
	if raw == "" {
		return "", "", fmt.Errorf("strict default output policy requires agent_control_plane_url to keep control-plane egress reachable")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "", "", fmt.Errorf("strict default output policy requires a valid agent_control_plane_url")
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		switch strings.ToLower(parsed.Scheme) {
		case "http":
			port = "80"
		default:
			port = "443"
		}
	}
	if _, err := parseNodeFirewallPort(port); err != nil {
		return "", "", fmt.Errorf("strict default output policy control-plane URL port: %w", err)
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		if hasExplicitControlPlaneOutputAllow(rules, port) {
			return "", fmt.Sprintf("strict default output policy relies on explicit output allow rule for control-plane TCP port %s because control-plane URL host %q is DNS", port, host), nil
		}
		return "", "", fmt.Errorf("strict default output policy requires control-plane URL host to be an IP address or an explicit active output accept rule for TCP port %s; %q is DNS", port, host)
	}
	family := "ip"
	cidr := addr.String() + "/32"
	if addr.Is6() {
		family = "ip6"
		cidr = addr.String() + "/128"
	}
	line := fmt.Sprintf("add rule %s %s %s %s daddr %s tcp dport %s accept comment %s", firewallNFTFamily, firewallNFTTable, firewallOutputChain, family, cidr, port, nftQuote(firewallNFTCommentScope+"system-control-plane-output"))
	return line, "", nil
}

func hasExplicitControlPlaneOutputAllow(rules []nodeFirewallRule, port string) bool {
	for _, rule := range rules {
		if !rule.Enabled || rule.Status != "active" || rule.Chain != "output" || rule.Action != "accept" {
			continue
		}
		if rule.Protocol != "any" && rule.Protocol != "tcp" {
			continue
		}
		if len(rule.StateMatch) > 0 && !containsNodeFirewallState(rule.StateMatch, "new") {
			continue
		}
		if rule.Protocol == "any" && rule.DstPorts == "" {
			return true
		}
		if rule.Protocol == "tcp" && nodeFirewallPortsContain(rule.DstPorts, port) {
			return true
		}
	}
	return false
}

func containsNodeFirewallState(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func nodeFirewallPortsContain(ports, target string) bool {
	if strings.TrimSpace(ports) == "" {
		return true
	}
	targetPort, err := parseNodeFirewallPort(target)
	if err != nil {
		return false
	}
	for _, part := range strings.Split(ports, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			left, right, _ := strings.Cut(part, "-")
			from, err := parseNodeFirewallPort(left)
			if err != nil {
				return false
			}
			to, err := parseNodeFirewallPort(right)
			if err != nil {
				return false
			}
			if targetPort >= from && targetPort <= to {
				return true
			}
			continue
		}
		port, err := parseNodeFirewallPort(part)
		if err != nil {
			return false
		}
		if port == targetPort {
			return true
		}
	}
	return false
}

func renderNodeFirewallDefaultRejectRules(defaults nodeFirewallDefaultPolicies) []string {
	if !defaults.Enforce {
		return nil
	}
	lines := []string{}
	if defaults.Input == "reject" {
		lines = append(lines, fmt.Sprintf("add rule %s %s %s reject comment %s", firewallNFTFamily, firewallNFTTable, firewallInputChain, nftQuote(firewallNFTCommentScope+"default-reject-input")))
	}
	if defaults.Forward == "reject" {
		lines = append(lines, fmt.Sprintf("add rule %s %s %s reject comment %s", firewallNFTFamily, firewallNFTTable, firewallForwardChain, nftQuote(firewallNFTCommentScope+"default-reject-forward")))
	}
	if defaults.Output == "reject" {
		lines = append(lines, fmt.Sprintf("add rule %s %s %s reject comment %s", firewallNFTFamily, firewallNFTTable, firewallOutputChain, nftQuote(firewallNFTCommentScope+"default-reject-output")))
	}
	return lines
}

func parseNodeFirewallAddressLists(raw any) (nodeFirewallAddressLists, []string, error) {
	out := nodeFirewallAddressLists{ByRef: map[string]nodeFirewallAddressList{}}
	warnings := []string{}
	items, ok := raw.([]any)
	if !ok && raw != nil {
		return nodeFirewallAddressLists{}, nil, fmt.Errorf("firewall address_lists payload must be an array")
	}
	for idx, item := range items {
		m, _ := item.(map[string]any)
		if m == nil {
			return nodeFirewallAddressLists{}, nil, fmt.Errorf("firewall address-list %d must be an object", idx+1)
		}
		status := strings.ToLower(first(stringify(m["status"]), "active"))
		if status != "active" {
			continue
		}
		listID := strings.TrimSpace(stringify(m["id"]))
		listKey := strings.TrimSpace(stringify(m["key"]))
		if listID == "" && listKey == "" {
			warnings = append(warnings, fmt.Sprintf("address-list %d has no id or key and was ignored", idx+1))
			continue
		}
		listName := nodeFirewallSetName(first(listKey, listID))
		list := nodeFirewallAddressList{
			ID:     listID,
			Key:    listKey,
			NameV4: listName + "_v4",
			NameV6: listName + "_v6",
		}
		rawEntries, _ := m["entries"].([]any)
		for entryIdx, rawEntry := range rawEntries {
			entry, _ := rawEntry.(map[string]any)
			if entry == nil {
				return nodeFirewallAddressLists{}, nil, fmt.Errorf("firewall address-list %s entry %d must be an object", first(listKey, listID), entryIdx+1)
			}
			if strings.ToLower(first(stringify(entry["status"]), "active")) != "active" {
				continue
			}
			element, supported, warning, err := normalizeNodeFirewallAddressListEntry(stringify(entry["value"]), stringify(entry["value_type"]))
			if err != nil {
				return nodeFirewallAddressLists{}, nil, fmt.Errorf("firewall address-list %s entry %d: %w", first(listKey, listID), entryIdx+1, err)
			}
			if warning != "" {
				warnings = append(warnings, fmt.Sprintf("address-list %s entry %d: %s", first(listKey, listID), entryIdx+1, warning))
			}
			if !supported {
				continue
			}
			switch element.Family {
			case "ip":
				list.V4 = append(list.V4, element.Value)
			case "ip6":
				list.V6 = append(list.V6, element.Value)
			}
		}
		sort.Strings(list.V4)
		sort.Strings(list.V6)
		if listID != "" {
			out.ByRef[listID] = list
		}
		if listKey != "" {
			out.ByRef[listKey] = list
		}
		out.Ordered = append(out.Ordered, list)
	}
	return out, warnings, nil
}

func parseNodeFirewallRules(raw any) ([]nodeFirewallRule, []string, error) {
	items, ok := raw.([]any)
	if !ok && raw != nil {
		return nil, nil, fmt.Errorf("firewall rules payload must be an array")
	}
	rules := make([]nodeFirewallRule, 0, len(items))
	warnings := []string{}
	for idx, item := range items {
		m, _ := item.(map[string]any)
		if m == nil {
			return nil, nil, fmt.Errorf("firewall rule %d must be an object", idx+1)
		}
		rule, err := parseNodeFirewallRule(m, idx)
		if err != nil {
			return nil, nil, err
		}
		rules = append(rules, rule)
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority == rules[j].Priority {
			return rules[i].ID < rules[j].ID
		}
		return rules[i].Priority < rules[j].Priority
	})
	return rules, warnings, nil
}

func parseNodeFirewallRule(m map[string]any, idx int) (nodeFirewallRule, error) {
	rule := nodeFirewallRule{
		ID:         first(stringify(m["id"]), strconv.Itoa(idx+1)),
		Priority:   intFromAny(m["priority"]),
		Chain:      strings.ToLower(stringify(m["chain"])),
		Action:     strings.ToLower(stringify(m["action"])),
		Protocol:   strings.ToLower(stringify(m["protocol"])),
		SrcListID:  strings.TrimSpace(stringify(m["src_list_id"])),
		SrcListKey: strings.TrimSpace(stringify(m["src_list_key"])),
		DstListID:  strings.TrimSpace(stringify(m["dst_list_id"])),
		DstListKey: strings.TrimSpace(stringify(m["dst_list_key"])),
		SrcCIDR:    stringify(m["src_cidr"]),
		DstCIDR:    stringify(m["dst_cidr"]),
		SrcPorts:   stringify(m["src_ports"]),
		DstPorts:   stringify(m["dst_ports"]),
		Comment:    stringify(m["comment"]),
		Enabled:    true,
		Status:     strings.ToLower(first(stringify(m["status"]), "active")),
	}
	if _, ok := m["enabled"]; ok {
		rule.Enabled = boolFromAny(m["enabled"])
	}
	if rule.Priority <= 0 {
		rule.Priority = 1000 + idx
	}
	if !inLocal(rule.Chain, "input", "forward", "output") {
		return nodeFirewallRule{}, fmt.Errorf("firewall rule %s chain must be input, forward or output", rule.ID)
	}
	if !inLocal(rule.Action, "accept", "drop", "reject") {
		return nodeFirewallRule{}, fmt.Errorf("firewall rule %s action must be accept, drop or reject", rule.ID)
	}
	if rule.Protocol == "" {
		rule.Protocol = "any"
	}
	if !inLocal(rule.Protocol, "any", "tcp", "udp", "icmp", "icmpv6") {
		return nodeFirewallRule{}, fmt.Errorf("firewall rule %s protocol must be any, tcp, udp, icmp or icmpv6", rule.ID)
	}
	var err error
	rule.SrcCIDR, err = normalizeNodeFirewallCIDR(rule.SrcCIDR)
	if err != nil {
		return nodeFirewallRule{}, fmt.Errorf("firewall rule %s source CIDR: %w", rule.ID, err)
	}
	rule.DstCIDR, err = normalizeNodeFirewallCIDR(rule.DstCIDR)
	if err != nil {
		return nodeFirewallRule{}, fmt.Errorf("firewall rule %s destination CIDR: %w", rule.ID, err)
	}
	rule.SrcPorts, err = normalizeNodeFirewallPorts(rule.SrcPorts)
	if err != nil {
		return nodeFirewallRule{}, fmt.Errorf("firewall rule %s source ports: %w", rule.ID, err)
	}
	rule.DstPorts, err = normalizeNodeFirewallPorts(rule.DstPorts)
	if err != nil {
		return nodeFirewallRule{}, fmt.Errorf("firewall rule %s destination ports: %w", rule.ID, err)
	}
	if (rule.SrcPorts != "" || rule.DstPorts != "") && rule.Protocol != "tcp" && rule.Protocol != "udp" {
		return nodeFirewallRule{}, fmt.Errorf("firewall rule %s ports require tcp or udp protocol", rule.ID)
	}
	rule.StateMatch = normalizeNodeFirewallStateMatch(m["state_match"])
	return rule, nil
}

func renderNodeFirewallRule(rule nodeFirewallRule, addressLists nodeFirewallAddressLists) ([]string, error) {
	srcList, hasSrcList, err := resolveNodeFirewallList(addressLists, rule.SrcListID, rule.SrcListKey, "source")
	if err != nil {
		return nil, fmt.Errorf("firewall rule %s: %w", rule.ID, err)
	}
	dstList, hasDstList, err := resolveNodeFirewallList(addressLists, rule.DstListID, rule.DstListKey, "destination")
	if err != nil {
		return nil, fmt.Errorf("firewall rule %s: %w", rule.ID, err)
	}
	families, err := nodeFirewallRuleFamilies(rule, srcList, hasSrcList, dstList, hasDstList)
	if err != nil {
		return nil, fmt.Errorf("firewall rule %s: %w", rule.ID, err)
	}
	lines := make([]string, 0, len(families))
	for _, family := range families {
		srcSet := ""
		dstSet := ""
		if hasSrcList {
			srcSet = srcList.setNameForFamily(family)
		}
		if hasDstList {
			dstSet = dstList.setNameForFamily(family)
		}
		line, err := renderNodeFirewallRuleLine(rule, family, srcSet, dstSet)
		if err != nil {
			return nil, err
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func resolveNodeFirewallList(addressLists nodeFirewallAddressLists, listID, listKey, label string) (nodeFirewallAddressList, bool, error) {
	listID = strings.TrimSpace(listID)
	listKey = strings.TrimSpace(listKey)
	if listID == "" && listKey == "" {
		return nodeFirewallAddressList{}, false, nil
	}
	if listID != "" {
		if list, ok := addressLists.ByRef[listID]; ok {
			return list, true, nil
		}
	}
	if listKey != "" {
		if list, ok := addressLists.ByRef[listKey]; ok {
			return list, true, nil
		}
	}
	return nodeFirewallAddressList{}, false, fmt.Errorf("%s address-list %q is not present in payload", label, first(listKey, listID))
}

func nodeFirewallRuleFamilies(rule nodeFirewallRule, srcList nodeFirewallAddressList, hasSrcList bool, dstList nodeFirewallAddressList, hasDstList bool) ([]string, error) {
	allowed := map[string]bool{"ip": true, "ip6": true}
	if hasSrcList {
		if len(srcList.families()) == 0 {
			return nil, fmt.Errorf("source address-list %q has no active IP/CIDR/range entries", first(srcList.Key, srcList.ID))
		}
		allowed = intersectNodeFirewallFamilies(allowed, srcList.families())
	} else if rule.SrcCIDR != "" {
		allowed = intersectNodeFirewallFamilies(allowed, map[string]bool{nftIPFamily(rule.SrcCIDR): true})
	}
	if hasDstList {
		if len(dstList.families()) == 0 {
			return nil, fmt.Errorf("destination address-list %q has no active IP/CIDR/range entries", first(dstList.Key, dstList.ID))
		}
		allowed = intersectNodeFirewallFamilies(allowed, dstList.families())
	} else if rule.DstCIDR != "" {
		allowed = intersectNodeFirewallFamilies(allowed, map[string]bool{nftIPFamily(rule.DstCIDR): true})
	}
	if rule.Protocol == "icmp" {
		allowed = intersectNodeFirewallFamilies(allowed, map[string]bool{"ip": true})
	}
	if rule.Protocol == "icmpv6" {
		allowed = intersectNodeFirewallFamilies(allowed, map[string]bool{"ip6": true})
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("source and destination address families do not overlap")
	}
	if !hasSrcList && !hasDstList && rule.SrcCIDR == "" && rule.DstCIDR == "" {
		return []string{""}, nil
	}
	out := make([]string, 0, 2)
	for _, family := range []string{"ip", "ip6"} {
		if allowed[family] {
			out = append(out, family)
		}
	}
	return out, nil
}

func intersectNodeFirewallFamilies(left, right map[string]bool) map[string]bool {
	out := map[string]bool{}
	for family := range left {
		if right[family] {
			out[family] = true
		}
	}
	return out
}

func renderNodeFirewallRuleLine(rule nodeFirewallRule, family, srcSet, dstSet string) (string, error) {
	chain := map[string]string{
		"input":   firewallInputChain,
		"forward": firewallForwardChain,
		"output":  firewallOutputChain,
	}[rule.Chain]
	if chain == "" {
		return "", fmt.Errorf("unsupported firewall chain %q", rule.Chain)
	}
	parts := []string{"add", "rule", firewallNFTFamily, firewallNFTTable, chain}
	if len(rule.StateMatch) > 0 {
		parts = append(parts, "ct", "state", nftSet(rule.StateMatch))
	}
	if srcSet != "" {
		parts = append(parts, family, "saddr", "@"+srcSet)
	} else if rule.SrcCIDR != "" {
		parts = append(parts, nftIPFamily(rule.SrcCIDR), "saddr", rule.SrcCIDR)
	}
	if dstSet != "" {
		parts = append(parts, family, "daddr", "@"+dstSet)
	} else if rule.DstCIDR != "" {
		parts = append(parts, nftIPFamily(rule.DstCIDR), "daddr", rule.DstCIDR)
	}
	switch rule.Protocol {
	case "tcp", "udp":
		if rule.SrcPorts != "" {
			parts = append(parts, rule.Protocol, "sport", nftPorts(rule.SrcPorts))
		}
		if rule.DstPorts != "" {
			parts = append(parts, rule.Protocol, "dport", nftPorts(rule.DstPorts))
		}
	case "icmp":
		parts = append(parts, "ip", "protocol", "icmp")
	case "icmpv6":
		parts = append(parts, "ip6", "nexthdr", "icmpv6")
	}
	parts = append(parts, rule.Action, "comment", nftQuote(nodeFirewallRuleComment(rule)))
	return strings.Join(parts, " "), nil
}

func renderNodeFirewallAddressListSets(addressLists nodeFirewallAddressLists) []string {
	lines := []string{}
	for _, list := range addressLists.Ordered {
		if len(list.V4) > 0 {
			lines = append(lines,
				fmt.Sprintf("add set %s %s %s { type ipv4_addr; flags interval; }", firewallNFTFamily, firewallNFTTable, list.NameV4),
				fmt.Sprintf("add element %s %s %s %s", firewallNFTFamily, firewallNFTTable, list.NameV4, nftElementSet(list.V4)),
			)
		}
		if len(list.V6) > 0 {
			lines = append(lines,
				fmt.Sprintf("add set %s %s %s { type ipv6_addr; flags interval; }", firewallNFTFamily, firewallNFTTable, list.NameV6),
				fmt.Sprintf("add element %s %s %s %s", firewallNFTFamily, firewallNFTTable, list.NameV6, nftElementSet(list.V6)),
			)
		}
	}
	return lines
}

func (list nodeFirewallAddressList) families() map[string]bool {
	out := map[string]bool{}
	if len(list.V4) > 0 {
		out["ip"] = true
	}
	if len(list.V6) > 0 {
		out["ip6"] = true
	}
	return out
}

func (list nodeFirewallAddressList) setNameForFamily(family string) string {
	switch family {
	case "ip":
		return list.NameV4
	case "ip6":
		return list.NameV6
	default:
		return ""
	}
}

func nodeFirewallSetName(ref string) string {
	name := strings.ReplaceAll(slugifyLocal(ref), "-", "_")
	if name == "" || name == "instance" {
		name = "list"
	}
	return "fwlist_" + name
}

func normalizeNodeFirewallAddressListEntry(value, valueType string) (nodeFirewallListElement, bool, string, error) {
	value = strings.TrimSpace(value)
	valueType = strings.ToLower(strings.TrimSpace(valueType))
	if value == "" {
		return nodeFirewallListElement{}, false, "", fmt.Errorf("value is required")
	}
	if valueType == "" {
		if strings.Contains(value, "/") {
			valueType = "cidr"
		} else if _, err := netip.ParseAddr(value); err == nil {
			valueType = "address"
		} else if strings.Contains(value, "-") {
			valueType = "range"
		} else {
			valueType = "dns"
		}
	}
	switch valueType {
	case "cidr":
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nodeFirewallListElement{}, false, "", fmt.Errorf("must be a valid CIDR")
		}
		prefix = prefix.Masked()
		return nodeFirewallListElement{Family: nftFamilyForAddr(prefix.Addr()), Value: prefix.String()}, true, "", nil
	case "address":
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return nodeFirewallListElement{}, false, "", fmt.Errorf("must be a valid IP address")
		}
		return nodeFirewallListElement{Family: nftFamilyForAddr(addr), Value: addr.String()}, true, "", nil
	case "range":
		left, right, ok := strings.Cut(value, "-")
		if !ok {
			return nodeFirewallListElement{}, false, "", fmt.Errorf("address range must use start-end format")
		}
		start, err := netip.ParseAddr(strings.TrimSpace(left))
		if err != nil {
			return nodeFirewallListElement{}, false, "", fmt.Errorf("range start must be a valid IP address")
		}
		end, err := netip.ParseAddr(strings.TrimSpace(right))
		if err != nil {
			return nodeFirewallListElement{}, false, "", fmt.Errorf("range end must be a valid IP address")
		}
		if nftFamilyForAddr(start) != nftFamilyForAddr(end) {
			return nodeFirewallListElement{}, false, "", fmt.Errorf("range start and end must use the same IP family")
		}
		if start.Compare(end) > 0 {
			return nodeFirewallListElement{}, false, "", fmt.Errorf("address range is reversed")
		}
		return nodeFirewallListElement{Family: nftFamilyForAddr(start), Value: start.String() + "-" + end.String()}, true, "", nil
	case "dns":
		return nodeFirewallListElement{}, false, "DNS entries are stored but not rendered into nftables rules in this release", nil
	default:
		return nodeFirewallListElement{}, false, "", fmt.Errorf("value_type must be cidr, address, range or dns")
	}
}

func runNFTScript(parent context.Context, script string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nft", "-f", "-")
	cmd.Stdin = bytes.NewBufferString(script)
	b, err := cmd.CombinedOutput()
	if err == nil {
		return string(b), nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return string(b), fmt.Errorf("nft apply failed with exit code %d: %s", ee.ExitCode(), firstLine(string(b)))
	}
	return string(b), err
}

func normalizeNodeFirewallCIDR(value string) (string, error) {
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

func normalizeNodeFirewallPorts(value string) (string, error) {
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
			from, err := parseNodeFirewallPort(left)
			if err != nil {
				return "", err
			}
			to, err := parseNodeFirewallPort(right)
			if err != nil {
				return "", err
			}
			if from > to {
				return "", fmt.Errorf("port range %q is reversed", part)
			}
			out = append(out, strconv.Itoa(from)+"-"+strconv.Itoa(to))
			continue
		}
		port, err := parseNodeFirewallPort(part)
		if err != nil {
			return "", err
		}
		out = append(out, strconv.Itoa(port))
	}
	return strings.Join(out, ","), nil
}

func parseNodeFirewallPort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("port %q must be between 1 and 65535", value)
	}
	return port, nil
}

func normalizeNodeFirewallStateMatch(raw any) []string {
	values := []string{}
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			values = append(values, stringify(item))
		}
	case []string:
		values = append(values, typed...)
	case string:
		values = strings.Split(typed, ",")
	}
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if !inLocal(value, "new", "established", "related", "invalid") || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func nftIPFamily(cidr string) string {
	if prefix, err := netip.ParsePrefix(cidr); err == nil && prefix.Addr().Is6() {
		return "ip6"
	}
	return "ip"
}

func nftFamilyForAddr(addr netip.Addr) string {
	if addr.Is6() {
		return "ip6"
	}
	return "ip"
}

func nftPorts(value string) string {
	parts := strings.Split(value, ",")
	if len(parts) == 1 {
		return strings.TrimSpace(parts[0])
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return nftSet(out)
}

func nftSet(values []string) string {
	clean := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			clean = append(clean, value)
		}
	}
	if len(clean) == 1 {
		return clean[0]
	}
	return "{ " + strings.Join(clean, ", ") + " }"
}

func nftElementSet(values []string) string {
	clean := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			clean = append(clean, value)
		}
	}
	return "{ " + strings.Join(clean, ", ") + " }"
}

func nodeFirewallRuleComment(rule nodeFirewallRule) string {
	key := slugifyLocal(first(rule.ID, rule.Comment, strconv.Itoa(rule.Priority)))
	if key == "" {
		key = strconv.Itoa(rule.Priority)
	}
	return firewallNFTCommentScope + key
}

func nftQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func inLocal(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
