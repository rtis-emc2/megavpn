package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

const (
	emergencyCleanupSchemaVersion = "node.emergency_cleanup.v1"
	emergencyCleanupOutcomeOK     = "succeeded"
	emergencyCleanupOutcomeWarn   = "succeeded_with_warnings"
	emergencyCleanupOutcomePart   = "partial_failure"
	emergencyCleanupOutcomeFailed = "failed"

	emergencyCleanupMaxResultTargets = 200
	emergencyCleanupMaxCodeLength    = 80

	agentSelfRemovalPendingDir = "/var/lib/megavpn/agent/post-ack"
	agentSelfRemovalRunDir     = "/run/megavpn-agent-self-remove"
)

var ErrAgentSelfRemovalScheduled = errors.New("agent self-removal scheduled after acknowledged cleanup result")

type jobExecutionOutcome struct {
	Status       string
	Result       map[string]any
	AfterSubmit  func(context.Context) error
	StopAfterAck bool
}

type emergencyCleanupJobPayload struct {
	SchemaVersion      string
	CleanupPlanVersion int
	NodeID             string
	NodeName           string
	CleanupScope       string
	IncludeAgent       bool
	Confirmation       string
	Reason             string
	RequestedAt        time.Time
	Instances          []emergencyCleanupTarget
}

type emergencyCleanupTarget struct {
	InstanceID         string
	ServiceCode        string
	RuntimeServiceCode string
	Name               string
	Slug               string
	SystemdUnit        string
	EndpointHost       string
	EndpointPort       int
	Enabled            bool
	Action             string
}

type emergencyTargetResult struct {
	InstanceID  string
	ServiceCode string
	Status      string
	Code        string
}

type emergencyCleanupSummary struct {
	TargetsTotal         int
	TargetsSucceeded     int
	TargetsAlreadyAbsent int
	TargetsFailed        int
	UnitsStopped         int
	PathsRemoved         int
	Warnings             int
}

type emergencyNginxResult struct {
	Status                 string `json:"status"`
	RemainingManagedConfig int    `json:"remaining_managed_configs"`
	RemovedManagedConfig   int    `json:"removed_managed_configs,omitempty"`
	RestoredManagedConfig  int    `json:"restored_managed_configs,omitempty"`
	Validation             string `json:"validation,omitempty"`
	Reload                 string `json:"reload,omitempty"`
	ActiveState            string `json:"active_state,omitempty"`
	ErrorCode              string `json:"error_code,omitempty"`
}

type emergencySystemdResult struct {
	DaemonReload string `json:"daemon_reload"`
	ErrorCode    string `json:"error_code,omitempty"`
}

type emergencyCleanupExecutor struct {
	root       string
	run        func(context.Context, string, ...string) (int, string)
	unitState  func(string) string
	unitOutput func(string, ...string) string
	clock      func() time.Time
}

func defaultEmergencyCleanupExecutor() emergencyCleanupExecutor {
	return emergencyCleanupExecutor{
		run:        runInstallCommand,
		unitState:  currentUnitState,
		unitOutput: runCombinedOutput,
		clock:      func() time.Time { return time.Now().UTC() },
	}
}

var newEmergencyCleanupExecutor = defaultEmergencyCleanupExecutor

func (c client) emergencyCleanupNodeOutcome(ctx context.Context, j job, st agentState) jobExecutionOutcome {
	payload, err := decodeEmergencyCleanupJobPayload(j.Payload, st)
	if err != nil {
		nodeID := st.NodeID
		if nodeID == "" {
			nodeID = stringify(j.Payload["node_id"])
		}
		return jobExecutionOutcome{
			Status: "failed",
			Result: map[string]any{
				"message": "emergency cleanup execution rejected",
				"outcome": emergencyCleanupOutcomeFailed,
				"node_id": nodeID,
				"error":   safeCleanupCode(err.Error()),
			},
		}
	}
	return newEmergencyCleanupExecutor().Execute(ctx, j.ID, payload)
}

func (e emergencyCleanupExecutor) Execute(ctx context.Context, jobID string, payload emergencyCleanupJobPayload) jobExecutionOutcome {
	started := e.now()
	sort.Slice(payload.Instances, func(i, j int) bool {
		if payload.Instances[i].ServiceCode != payload.Instances[j].ServiceCode {
			return payload.Instances[i].ServiceCode < payload.Instances[j].ServiceCode
		}
		return payload.Instances[i].InstanceID < payload.Instances[j].InstanceID
	})

	summary := emergencyCleanupSummary{TargetsTotal: len(payload.Instances)}
	targetResults := make([]map[string]any, 0, minInt(len(payload.Instances), emergencyCleanupMaxResultTargets))
	criticalFailures := 0
	unitFilesChanged := false
	nginxCandidates := map[string]struct{}{}
	seenUnits := map[string]struct{}{}

	for _, target := range payload.Instances {
		result, targetUnitChanged := e.cleanupInstanceTarget(ctx, payload.CleanupScope, target, seenUnits, nginxCandidates, &summary)
		if targetUnitChanged {
			unitFilesChanged = true
		}
		switch result.Status {
		case "failed":
			criticalFailures++
			summary.TargetsFailed++
		case "already_absent":
			summary.TargetsAlreadyAbsent++
			summary.TargetsSucceeded++
		default:
			summary.TargetsSucceeded++
		}
		if len(targetResults) < emergencyCleanupMaxResultTargets {
			targetResults = append(targetResults, map[string]any{
				"instance_id":  truncateSafe(result.InstanceID, 128),
				"service_code": truncateSafe(result.ServiceCode, 64),
				"status":       truncateSafe(result.Status, 32),
				"code":         truncateSafe(result.Code, emergencyCleanupMaxCodeLength),
			})
		}
	}

	leftoverChanged, leftoverFailures, leftoverWarnings := e.cleanupManagedLeftovers(ctx, payload.CleanupScope, seenUnits, nginxCandidates, &summary)
	if leftoverChanged {
		unitFilesChanged = true
	}
	criticalFailures += leftoverFailures
	summary.Warnings += leftoverWarnings

	nginx := e.cleanupNginxManagedConfigs(ctx, nginxCandidates, &summary)
	if nginx.ErrorCode != "" {
		criticalFailures++
	}

	systemd := emergencySystemdResult{DaemonReload: "not_required"}
	if unitFilesChanged {
		code, _ := e.command(ctx, "systemctl", "daemon-reload")
		if code != 0 {
			systemd.DaemonReload = "failed"
			systemd.ErrorCode = "systemd_reload_failed"
			criticalFailures++
		} else {
			systemd.DaemonReload = "succeeded"
		}
	}

	outcome := emergencyCleanupOutcomeOK
	status := "succeeded"
	if summary.Warnings > 0 {
		outcome = emergencyCleanupOutcomeWarn
	}
	if criticalFailures > 0 {
		outcome = emergencyCleanupOutcomePart
		status = "failed"
		if summary.TargetsSucceeded == 0 && len(payload.Instances) > 0 {
			outcome = emergencyCleanupOutcomeFailed
		}
	}

	agentState := "not_requested"
	var afterSubmit func(context.Context) error
	stopAfterAck := false
	if payload.IncludeAgent {
		if criticalFailures == 0 {
			agentState = "pending_result_ack"
			actionJobID := jobID
			afterSubmit = func(ctx context.Context) error {
				return defaultAgentSelfRemovalManager.activateAfterAck(ctx, actionJobID)
			}
			stopAfterAck = true
		} else {
			agentState = "skipped_cleanup_not_successful"
		}
	}

	result := map[string]any{
		"message":              "emergency cleanup execution finished",
		"outcome":              outcome,
		"node_id":              truncateSafe(payload.NodeID, 128),
		"cleanup_scope":        payload.CleanupScope,
		"include_agent":        payload.IncludeAgent,
		"cleanup_plan_version": payload.CleanupPlanVersion,
		"summary": map[string]any{
			"targets_total":          summary.TargetsTotal,
			"targets_succeeded":      summary.TargetsSucceeded,
			"targets_already_absent": summary.TargetsAlreadyAbsent,
			"targets_failed":         summary.TargetsFailed,
			"units_stopped":          summary.UnitsStopped,
			"paths_removed":          summary.PathsRemoved,
			"warnings":               summary.Warnings,
		},
		"targets": targetResults,
		"nginx": map[string]any{
			"status":                    nginx.Status,
			"remaining_managed_configs": nginx.RemainingManagedConfig,
		},
		"systemd": map[string]any{
			"daemon_reload": systemd.DaemonReload,
		},
		"agent_self_removal": map[string]any{
			"requested": payload.IncludeAgent,
			"state":     agentState,
		},
		"duration_ms": e.now().Sub(started).Milliseconds(),
	}
	if nginx.ErrorCode != "" {
		result["nginx"].(map[string]any)["error_code"] = nginx.ErrorCode
	}
	if systemd.ErrorCode != "" {
		result["systemd"].(map[string]any)["error_code"] = systemd.ErrorCode
	}
	if criticalFailures > 0 {
		result["error"] = "emergency_cleanup_partial_failure"
	}
	return jobExecutionOutcome{Status: status, Result: result, AfterSubmit: afterSubmit, StopAfterAck: stopAfterAck}
}

func decodeEmergencyCleanupJobPayload(raw map[string]any, st agentState) (emergencyCleanupJobPayload, error) {
	if raw == nil {
		return emergencyCleanupJobPayload{}, errors.New("payload_missing")
	}
	allowed := map[string]struct{}{
		"schema": {}, "node_id": {}, "node_name": {}, "cleanup_scope": {}, "include_agent": {},
		"confirmation": {}, "reason": {}, "requested_at": {}, "cleanup_plan_version": {}, "instances": {},
	}
	for key := range raw {
		if _, ok := allowed[key]; !ok {
			return emergencyCleanupJobPayload{}, fmt.Errorf("payload_unknown_field_%s", safeCleanupCode(key))
		}
	}
	payload := emergencyCleanupJobPayload{
		SchemaVersion: stringify(raw["schema"]),
		NodeID:        stringify(raw["node_id"]),
		NodeName:      stringify(raw["node_name"]),
		CleanupScope:  stringify(raw["cleanup_scope"]),
		Confirmation:  stringify(raw["confirmation"]),
		Reason:        stringify(raw["reason"]),
	}
	if payload.SchemaVersion != emergencyCleanupSchemaVersion {
		return emergencyCleanupJobPayload{}, errors.New("schema_version_unsupported")
	}
	if payload.NodeID == "" {
		return emergencyCleanupJobPayload{}, errors.New("node_id_required")
	}
	if st.NodeID != "" && payload.NodeID != st.NodeID {
		return emergencyCleanupJobPayload{}, errors.New("node_id_mismatch")
	}
	expected := first(payload.NodeName, st.NodeName)
	if expected == "" || payload.Confirmation != expected {
		return emergencyCleanupJobPayload{}, errors.New("confirmation_mismatch")
	}
	includeAgent, ok := raw["include_agent"].(bool)
	if !ok {
		return emergencyCleanupJobPayload{}, errors.New("include_agent_boolean_required")
	}
	payload.IncludeAgent = includeAgent
	switch payload.CleanupScope {
	case domain.NodeEmergencyCleanupScopeServicesOnly, domain.NodeEmergencyCleanupScopeFullNode:
	default:
		return emergencyCleanupJobPayload{}, errors.New("cleanup_scope_invalid")
	}
	if payload.IncludeAgent && payload.CleanupScope != domain.NodeEmergencyCleanupScopeFullNode {
		return emergencyCleanupJobPayload{}, errors.New("include_agent_requires_full_node")
	}
	if err := domain.ValidateNodeEmergencyCleanupReason(payload.Reason); err != nil {
		return emergencyCleanupJobPayload{}, fmt.Errorf("reason_invalid: %w", err)
	}
	planVersion, ok := emergencyCleanupStrictInt(raw["cleanup_plan_version"])
	if !ok {
		return emergencyCleanupJobPayload{}, errors.New("cleanup_plan_version_required")
	}
	if planVersion != domain.NodeEmergencyCleanupPlanVersion {
		return emergencyCleanupJobPayload{}, errors.New("cleanup_plan_version_unsupported")
	}
	payload.CleanupPlanVersion = planVersion
	requestedAt, err := parseEmergencyCleanupRequestedAt(raw["requested_at"])
	if err != nil {
		return emergencyCleanupJobPayload{}, err
	}
	payload.RequestedAt = requestedAt
	rawTargets, ok := raw["instances"].([]any)
	if !ok {
		return emergencyCleanupJobPayload{}, errors.New("instances_array_required")
	}
	seen := map[string]struct{}{}
	for i, item := range rawTargets {
		targetMap, ok := item.(map[string]any)
		if !ok {
			return emergencyCleanupJobPayload{}, fmt.Errorf("target_%d_not_object", i)
		}
		target, err := decodeEmergencyCleanupTarget(targetMap)
		if err != nil {
			return emergencyCleanupJobPayload{}, fmt.Errorf("target_%d_%w", i, err)
		}
		if _, exists := seen[target.InstanceID]; exists {
			return emergencyCleanupJobPayload{}, errors.New("duplicate_instance_target")
		}
		seen[target.InstanceID] = struct{}{}
		payload.Instances = append(payload.Instances, target)
	}
	return payload, nil
}

func decodeEmergencyCleanupTarget(raw map[string]any) (emergencyCleanupTarget, error) {
	allowed := map[string]struct{}{
		"instance_id": {}, "action": {}, "service_code": {}, "runtime_service_code": {}, "name": {},
		"slug": {}, "systemd_unit": {}, "endpoint_host": {}, "endpoint_port": {}, "enabled": {},
	}
	for key := range raw {
		if _, ok := allowed[key]; !ok {
			return emergencyCleanupTarget{}, fmt.Errorf("unknown_field_%s", safeCleanupCode(key))
		}
	}
	target := emergencyCleanupTarget{
		InstanceID:         stringify(raw["instance_id"]),
		Action:             stringify(raw["action"]),
		ServiceCode:        driver.NormalizeCode(stringify(raw["service_code"])),
		RuntimeServiceCode: driver.NormalizeCode(stringify(raw["runtime_service_code"])),
		Name:               stringify(raw["name"]),
		Slug:               stringify(raw["slug"]),
		SystemdUnit:        stringify(raw["systemd_unit"]),
		EndpointHost:       stringify(raw["endpoint_host"]),
	}
	if target.InstanceID == "" || !isSafeCleanupIdentifier(target.InstanceID, 128) {
		return emergencyCleanupTarget{}, errors.New("instance_id_invalid")
	}
	if target.Action != driver.OperationDelete {
		return emergencyCleanupTarget{}, errors.New("action_must_be_delete")
	}
	if _, ok := driver.ContractFor(target.ServiceCode); !ok {
		return emergencyCleanupTarget{}, errors.New("service_code_unsupported")
	}
	if target.RuntimeServiceCode == "" {
		target.RuntimeServiceCode = target.ServiceCode
	}
	if target.RuntimeServiceCode != target.ServiceCode {
		return emergencyCleanupTarget{}, errors.New("runtime_service_code_mismatch")
	}
	if target.Slug == "" {
		target.Slug = slugifyLocal(first(target.Name, target.InstanceID))
	}
	if !isSafeCleanupSlug(target.Slug) {
		return emergencyCleanupTarget{}, errors.New("slug_invalid")
	}
	if target.Name != "" && !isSafeCleanupDisplayValue(target.Name, 160) {
		return emergencyCleanupTarget{}, errors.New("name_invalid")
	}
	if target.EndpointHost != "" && !isSafeCleanupDisplayValue(target.EndpointHost, 255) {
		return emergencyCleanupTarget{}, errors.New("endpoint_host_invalid")
	}
	port, ok := emergencyCleanupStrictInt(raw["endpoint_port"])
	if raw["endpoint_port"] != nil && !ok {
		return emergencyCleanupTarget{}, errors.New("endpoint_port_invalid")
	}
	if port < 0 || port > 65535 {
		return emergencyCleanupTarget{}, errors.New("endpoint_port_invalid")
	}
	target.EndpointPort = port
	if enabled, ok := raw["enabled"].(bool); ok {
		target.Enabled = enabled
	}
	if target.ServiceCode == driver.Nginx && strings.TrimSpace(target.SystemdUnit) == "nginx" {
		target.SystemdUnit = ""
	}
	if target.SystemdUnit == "" {
		target.SystemdUnit = defaultInstanceSystemdUnit(instanceJobPayload{ServiceCode: target.ServiceCode, RuntimeServiceCode: target.RuntimeServiceCode, Slug: target.Slug, InstanceID: target.InstanceID, Name: target.Name})
	}
	if target.ServiceCode == driver.Nginx && target.SystemdUnit == "nginx" {
		target.SystemdUnit = ""
	}
	if target.SystemdUnit != "" {
		payload := target.toInstancePayload()
		if !isAllowedInstanceUnit(payload, target.SystemdUnit) {
			return emergencyCleanupTarget{}, errors.New("systemd_unit_not_owned")
		}
	}
	return target, nil
}

func (target emergencyCleanupTarget) toInstancePayload() instanceJobPayload {
	return instanceJobPayload{
		InstanceID:         target.InstanceID,
		Action:             driver.OperationDelete,
		ServiceCode:        target.ServiceCode,
		RuntimeServiceCode: target.RuntimeServiceCode,
		Name:               target.Name,
		Slug:               target.Slug,
		SystemdUnit:        target.SystemdUnit,
		EndpointHost:       target.EndpointHost,
		EndpointPort:       target.EndpointPort,
		Enabled:            false,
		Spec:               map[string]any{},
	}
}

func (e emergencyCleanupExecutor) cleanupInstanceTarget(ctx context.Context, cleanupScope string, target emergencyCleanupTarget, seenUnits map[string]struct{}, nginxCandidates map[string]struct{}, summary *emergencyCleanupSummary) (emergencyTargetResult, bool) {
	result := emergencyTargetResult{InstanceID: target.InstanceID, ServiceCode: target.ServiceCode, Status: "succeeded", Code: "removed"}
	unitFilesChanged := false
	if target.SystemdUnit != "" {
		unitCode := e.disableOwnedUnit(ctx, cleanupScope, target.SystemdUnit, seenUnits, summary)
		switch unitCode {
		case "systemd_unit_invalid", "systemd_disable_failed":
			result.Status = "failed"
			result.Code = unitCode
			return result, false
		case "stopped":
			summary.UnitsStopped++
		}
	}
	for _, logicalPath := range e.targetManagedPaths(target) {
		if isEmergencyNginxSnippetPath(logicalPath) {
			nginxCandidates[logicalPath] = struct{}{}
			continue
		}
		code, _ := e.removeLogicalManagedPath(logicalPath, cleanupScope, false, true, summary)
		if code == "cleanup_removed" && isEmergencySystemdUnitPath(logicalPath) {
			unitFilesChanged = true
		}
		switch code {
		case "cleanup_removed":
		case "cleanup_absent":
			if result.Code == "removed" {
				result.Status = "already_absent"
				result.Code = "already_absent"
			}
		default:
			result.Status = "failed"
			result.Code = code
			return result, unitFilesChanged
		}
	}
	return result, unitFilesChanged
}

func (e emergencyCleanupExecutor) cleanupManagedLeftovers(ctx context.Context, cleanupScope string, seenUnits map[string]struct{}, nginxCandidates map[string]struct{}, summary *emergencyCleanupSummary) (bool, int, int) {
	unitFilesChanged := false
	failures := 0
	warnings := 0
	for _, unit := range e.emergencyCleanupUnitNames(cleanupScope) {
		switch e.disableOwnedUnit(ctx, cleanupScope, unit, seenUnits, summary) {
		case "stopped":
			summary.UnitsStopped++
		case "systemd_disable_failed", "systemd_unit_invalid":
			failures++
		}
		if path := emergencyManagedUnitFilePathForScope(unit, cleanupScope); path != "" {
			code, changed := e.removeLogicalManagedPath(path, cleanupScope, false, false, summary)
			if changed {
				unitFilesChanged = true
			}
			if isCriticalCleanupCode(code) {
				failures++
			}
		}
	}

	fullNode := cleanupScope == domain.NodeEmergencyCleanupScopeFullNode
	if fullNode {
		for _, path := range []string{"/etc/megavpn/client-access-routes.json", "/etc/systemd/system/megavpn-route-policy.service"} {
			code, changed := e.removeLogicalManagedPath(path, cleanupScope, false, false, summary)
			if changed && isEmergencySystemdUnitPath(path) {
				unitFilesChanged = true
			}
			if isCriticalCleanupCode(code) {
				failures++
			}
		}
	}

	managedDirs := []string{"/etc/megavpn/certs", "/var/lib/megavpn/openvpn"}
	if fullNode {
		managedDirs = append([]string{"/etc/megavpn/backhaul"}, managedDirs...)
	}
	for _, path := range managedDirs {
		code, _ := e.removeLogicalManagedPath(path, cleanupScope, true, false, summary)
		if isCriticalCleanupCode(code) {
			failures++
		}
	}

	for _, path := range e.emergencyCleanupManagedOpenVPNRuntimeDirs() {
		code, _ := e.removeLogicalManagedPath(path, cleanupScope, true, false, summary)
		if isCriticalCleanupCode(code) {
			failures++
		}
	}
	for _, path := range e.emergencyCleanupManagedServiceConfigPaths() {
		if isEmergencyNginxSnippetPath(path) {
			nginxCandidates[path] = struct{}{}
			continue
		}
		code, _ := e.removeLogicalManagedPath(path, cleanupScope, false, true, summary)
		if isCriticalCleanupCode(code) {
			failures++
		}
	}
	for _, path := range e.globLogical("/etc/nginx/conf.d/megavpn-*.conf") {
		if isEmergencyManagedFilePathForScope(path, cleanupScope) {
			nginxCandidates[path] = struct{}{}
		}
	}
	for _, path := range e.globLogical("/etc/systemd/system/megavpn-*.service") {
		if isEmergencyManagedFilePathForScope(path, cleanupScope) {
			code, changed := e.removeLogicalManagedPath(path, cleanupScope, false, false, summary)
			if changed {
				unitFilesChanged = true
			}
			if isCriticalCleanupCode(code) {
				failures++
			}
		}
	}
	return unitFilesChanged, failures, warnings
}

func (e emergencyCleanupExecutor) targetManagedPaths(target emergencyCleanupTarget) []string {
	payload := target.toInstancePayload()
	paths := []string{}
	if cfg := driver.DefaultConfigPath(target.ServiceCode, target.Slug); cfg != "" {
		paths = append(paths, cfg)
	}
	paths = append(paths, legacyManagedDeletePaths(payload)...)
	if unitPath := managedInstanceUnitFilePath(payload); unitPath != "" {
		paths = append(paths, unitPath)
	}
	sort.Strings(paths)
	return compactStrings(paths)
}

func (e emergencyCleanupExecutor) disableOwnedUnit(ctx context.Context, cleanupScope, unit string, seen map[string]struct{}, summary *emergencyCleanupSummary) string {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return "skipped"
	}
	if _, ok := seen[unit]; ok {
		return "already_processed"
	}
	seen[unit] = struct{}{}
	if !isEmergencyCleanupRunnableUnitForScope(unit, cleanupScope) {
		return "systemd_unit_invalid"
	}
	code, out := e.command(ctx, "systemctl", "disable", "--now", "--", unit)
	if code != 0 {
		if isMissingSystemdUnitOutput(out) || e.unitLoadState(unit) == "not-found" {
			summary.Warnings++
			return "missing_unit"
		}
		return "systemd_disable_failed"
	}
	_, _ = e.command(ctx, "systemctl", "reset-failed", "--", unit)
	return "stopped"
}

func (e emergencyCleanupExecutor) cleanupNginxManagedConfigs(ctx context.Context, candidates map[string]struct{}, summary *emergencyCleanupSummary) emergencyNginxResult {
	paths := make([]string, 0, len(candidates))
	for path := range candidates {
		if isEmergencyNginxSnippetPath(path) && isEmergencyManagedFilePathForScope(path, domain.NodeEmergencyCleanupScopeFullNode) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	result := emergencyNginxResult{Status: "unchanged", RemainingManagedConfig: len(paths), ActiveState: e.unitStateSafe("nginx")}
	if len(paths) == 0 {
		return result
	}
	backupDir, err := os.MkdirTemp(e.actualPath("/run"), "megavpn-emergency-nginx-*")
	if os.IsNotExist(err) {
		if mkErr := os.MkdirAll(e.actualPath("/run"), 0o755); mkErr == nil {
			backupDir, err = os.MkdirTemp(e.actualPath("/run"), "megavpn-emergency-nginx-*")
		}
	}
	if err != nil {
		result.Status = "failed"
		result.ErrorCode = "nginx_backup_failed"
		return result
	}
	defer os.RemoveAll(backupDir)

	backups := map[string][]byte{}
	for _, path := range paths {
		body, err := os.ReadFile(e.actualPath(path))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			result.Status = "failed"
			result.ErrorCode = "nginx_backup_failed"
			return result
		}
		backups[path] = append([]byte(nil), body...)
	}
	removed := 0
	for _, path := range paths {
		code, changed := e.removeLogicalManagedPath(path, domain.NodeEmergencyCleanupScopeFullNode, false, false, summary)
		if changed {
			removed++
		}
		if isCriticalCleanupCode(code) {
			result.Status = "failed"
			result.ErrorCode = "nginx_remove_failed"
			return result
		}
	}
	result.RemovedManagedConfig = removed
	result.RemainingManagedConfig = len(e.globLogical("/etc/nginx/conf.d/megavpn-*.conf"))
	code, _ := e.command(ctx, "nginx", "-t")
	result.Validation = "succeeded"
	if code != 0 {
		result.Validation = "failed"
		result.Status = "failed"
		result.ErrorCode = "nginx_validation_failed"
		restored := e.restoreNginxBackups(backups)
		result.RestoredManagedConfig = restored
		if restoreCode, _ := e.command(ctx, "nginx", "-t"); restoreCode != 0 {
			result.ErrorCode = "nginx_restore_failed"
		}
		return result
	}
	if normalizeSystemctlState(result.ActiveState) == "active" {
		reloadCode, _ := e.command(ctx, "systemctl", "reload", "nginx")
		if reloadCode != 0 {
			result.Reload = "failed"
			result.Status = "failed"
			result.ErrorCode = "nginx_reload_failed"
			return result
		}
		result.Reload = "succeeded"
		result.Status = "reloaded"
		return result
	}
	result.Reload = "not_required"
	result.Status = "inactive_unchanged"
	return result
}

func (e emergencyCleanupExecutor) restoreNginxBackups(backups map[string][]byte) int {
	restored := 0
	for logical, body := range backups {
		actual := e.actualPath(logical)
		if err := os.MkdirAll(filepath.Dir(actual), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(actual, body, 0o644); err == nil {
			restored++
		}
	}
	return restored
}

func (e emergencyCleanupExecutor) removeLogicalManagedPath(logicalPath, cleanupScope string, recursive bool, requireOwnershipMarker bool, summary *emergencyCleanupSummary) (string, bool) {
	logicalPath = cleanEmergencyCleanupPath(logicalPath)
	if logicalPath == "" {
		return "cleanup_path_unsafe", false
	}
	if !isEmergencyManagedLogicalPathForScope(logicalPath, cleanupScope, recursive) {
		return "cleanup_path_forbidden", false
	}
	if requireOwnershipMarker && !e.hasOwnershipMarker(logicalPath) {
		return "cleanup_ownership_marker_missing", false
	}
	removed, err := e.removeNoFollow(logicalPath, recursive)
	if err != nil {
		return safeCleanupCode(err.Error()), false
	}
	if removed {
		summary.PathsRemoved++
		return "cleanup_removed", true
	}
	return "cleanup_absent", false
}

func (e emergencyCleanupExecutor) removeNoFollow(logicalPath string, recursive bool) (bool, error) {
	actual := e.actualPath(logicalPath)
	if err := rejectSymlinkParents(actual); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.New("cleanup_path_unsafe")
	}
	info, err := os.Lstat(actual)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.New("cleanup_lstat_failed")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("cleanup_symlink_rejected")
	}
	if info.IsDir() {
		if !recursive {
			return false, errors.New("cleanup_directory_requires_recursive_policy")
		}
		if err := removeDirNoFollow(actual); err != nil {
			return false, errors.New("cleanup_remove_failed")
		}
		return true, nil
	}
	if err := os.Remove(actual); err != nil {
		return false, errors.New("cleanup_remove_failed")
	}
	return true, nil
}

func removeDirNoFollow(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		info, err := os.Lstat(child)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlink child rejected")
		}
		if info.IsDir() {
			if err := removeDirNoFollow(child); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(child); err != nil {
			return err
		}
	}
	return os.Remove(path)
}

func rejectSymlinkParents(path string) error {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) || path == string(filepath.Separator) {
		return errors.New("cleanup_path_unsafe")
	}
	dir := filepath.Dir(path)
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(dir, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("cleanup_symlink_parent_rejected")
		}
	}
	return nil
}

func (e emergencyCleanupExecutor) hasOwnershipMarker(logicalPath string) bool {
	switch {
	case strings.HasPrefix(logicalPath, "/etc/openvpn/server/") && strings.HasSuffix(logicalPath, ".conf"):
		body, ok := e.readLogicalConfig(logicalPath)
		return ok && isEmergencyManagedOpenVPNConfigContent(body)
	case strings.HasPrefix(logicalPath, "/etc/wireguard/") && strings.HasSuffix(logicalPath, ".conf"):
		body, ok := e.readLogicalConfig(logicalPath)
		return ok && isEmergencyManagedWireGuardConfigContent(body)
	default:
		return true
	}
}

func (e emergencyCleanupExecutor) readLogicalConfig(logicalPath string) (string, bool) {
	if err := rejectSymlinkParents(e.actualPath(logicalPath)); err != nil {
		return "", false
	}
	info, err := os.Lstat(e.actualPath(logicalPath))
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	body, err := os.ReadFile(e.actualPath(logicalPath))
	if err != nil {
		return "", false
	}
	return string(body), true
}

func (e emergencyCleanupExecutor) emergencyCleanupUnitNames(cleanupScope string) []string {
	seen := map[string]bool{}
	add := func(unit string) {
		unit = strings.TrimSpace(unit)
		unit = strings.TrimSuffix(unit, ".service") + ".service"
		if isEmergencyCleanupRunnableUnitForScope(unit, cleanupScope) {
			seen[unit] = true
		}
	}
	for _, raw := range []string{
		e.unitOutput("systemctl", "list-unit-files", "--type=service", "--no-legend", "--no-pager"),
		e.unitOutput("systemctl", "list-units", "--type=service", "--all", "--no-legend", "--no-pager"),
	} {
		for _, line := range strings.Split(raw, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				add(fields[0])
			}
		}
	}
	for _, path := range e.globLogical("/etc/systemd/system/megavpn-*.service") {
		add(filepath.Base(path))
	}
	for _, unit := range e.emergencyCleanupManagedServiceConfigUnits() {
		add(unit)
	}
	out := make([]string, 0, len(seen))
	for unit := range seen {
		out = append(out, unit)
	}
	sort.Strings(out)
	return out
}

func (e emergencyCleanupExecutor) emergencyCleanupManagedServiceConfigUnits() []string {
	paths := e.emergencyCleanupManagedServiceConfigPaths()
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		unit := emergencyManagedServiceUnitForConfigPath(path)
		if unit == "" || seen[unit] {
			continue
		}
		seen[unit] = true
		out = append(out, unit)
	}
	sort.Strings(out)
	return out
}

func (e emergencyCleanupExecutor) emergencyCleanupManagedServiceConfigPaths() []string {
	out := []string{}
	for _, item := range []struct {
		pattern string
		managed func(string) bool
	}{
		{pattern: "/etc/openvpn/server/*.conf", managed: e.isEmergencyManagedOpenVPNConfigPath},
		{pattern: "/etc/wireguard/*.conf", managed: e.isEmergencyManagedWireGuardConfigPath},
	} {
		for _, path := range e.globLogical(item.pattern) {
			if item.managed(path) {
				out = append(out, cleanEmergencyCleanupPath(path))
			}
		}
	}
	sort.Strings(out)
	return out
}

func (e emergencyCleanupExecutor) emergencyCleanupManagedOpenVPNRuntimeDirs() []string {
	out := []string{}
	for _, path := range e.globLogical("/etc/openvpn/server/megavpn-*") {
		actual := e.actualPath(path)
		info, err := os.Lstat(actual)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if isEmergencyManagedLogicalPathForScope(path, domain.NodeEmergencyCleanupScopeFullNode, true) {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func (e emergencyCleanupExecutor) isEmergencyManagedOpenVPNConfigPath(path string) bool {
	path = cleanEmergencyCleanupPath(path)
	if path == "" || !pathWithinRoot(path, "/etc/openvpn/server") || !strings.HasSuffix(path, ".conf") {
		return false
	}
	body, ok := e.readLogicalConfig(path)
	return ok && isEmergencyManagedOpenVPNConfigContent(body)
}

func (e emergencyCleanupExecutor) isEmergencyManagedWireGuardConfigPath(path string) bool {
	path = cleanEmergencyCleanupPath(path)
	if path == "" || !pathWithinRoot(path, "/etc/wireguard") || !strings.HasSuffix(path, ".conf") {
		return false
	}
	body, ok := e.readLogicalConfig(path)
	return ok && isEmergencyManagedWireGuardConfigContent(body)
}

func (e emergencyCleanupExecutor) globLogical(pattern string) []string {
	actualPattern := e.actualPath(pattern)
	matches, err := filepath.Glob(actualPattern)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, actual := range matches {
		out = append(out, e.logicalPath(actual))
	}
	sort.Strings(out)
	return out
}

func (e emergencyCleanupExecutor) actualPath(logical string) string {
	logical = cleanEmergencyCleanupPath(logical)
	if e.root == "" || logical == "" {
		return logical
	}
	return filepath.Join(e.root, strings.TrimPrefix(logical, "/"))
}

func (e emergencyCleanupExecutor) logicalPath(actual string) string {
	if e.root == "" {
		return cleanEmergencyCleanupPath(actual)
	}
	rel, err := filepath.Rel(e.root, actual)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return cleanEmergencyCleanupPath("/" + rel)
}

func (e emergencyCleanupExecutor) command(ctx context.Context, name string, args ...string) (int, string) {
	if e.run == nil {
		e.run = runInstallCommand
	}
	return e.run(ctx, name, args...)
}

func (e emergencyCleanupExecutor) unitStateSafe(unit string) string {
	if e.unitState == nil {
		return currentUnitState(unit)
	}
	return e.unitState(unit)
}

func (e emergencyCleanupExecutor) unitLoadState(unit string) string {
	if e.unitOutput == nil {
		e.unitOutput = runCombinedOutput
	}
	return normalizeSystemctlState(strings.TrimSpace(e.unitOutput("systemctl", "show", unit, "-p", "LoadState", "--value")))
}

func (e emergencyCleanupExecutor) now() time.Time {
	if e.clock == nil {
		return time.Now().UTC()
	}
	return e.clock().UTC()
}

func isEmergencyManagedLogicalPathForScope(path, cleanupScope string, recursive bool) bool {
	path = cleanEmergencyCleanupPath(path)
	if path == "" || path == "/" {
		return false
	}
	fullNode := cleanupScope == domain.NodeEmergencyCleanupScopeFullNode
	switch {
	case path == "/etc/megavpn/client-access-routes.json":
		return fullNode && !recursive
	case path == "/etc/systemd/system/megavpn-route-policy.service":
		return fullNode && !recursive
	case path == "/etc/megavpn/backhaul":
		return fullNode && recursive
	case path == "/etc/megavpn/certs", path == "/var/lib/megavpn/openvpn":
		return recursive
	case strings.HasPrefix(path, "/etc/openvpn/server/megavpn-"):
		return recursive && pathWithinRoot(path, "/etc/openvpn/server")
	case strings.HasPrefix(path, "/etc/openvpn/server/") && strings.HasSuffix(path, ".conf"):
		return !recursive && pathWithinRoot(path, "/etc/openvpn/server")
	case strings.HasPrefix(path, "/etc/wireguard/") && strings.HasSuffix(path, ".conf"):
		return !recursive && pathWithinRoot(path, "/etc/wireguard")
	case isEmergencyNginxSnippetPath(path):
		return !recursive
	case isEmergencySystemdUnitPath(path):
		return !recursive && isEmergencyManagedUnitForScope(filepath.Base(path), cleanupScope)
	case strings.HasPrefix(path, "/usr/local/etc/xray/") && strings.HasSuffix(path, ".json"):
		return !recursive && pathWithinRoot(path, "/usr/local/etc/xray")
	case strings.HasPrefix(path, "/etc/squid/") && strings.HasSuffix(path, ".conf"):
		return !recursive && pathWithinRoot(path, "/etc/squid")
	case strings.HasPrefix(path, "/etc/shadowsocks-libev/") && strings.HasSuffix(path, ".json"):
		return !recursive && pathWithinRoot(path, "/etc/shadowsocks-libev")
	default:
		return false
	}
}

func isEmergencyNginxSnippetPath(path string) bool {
	path = cleanEmergencyCleanupPath(path)
	base := filepath.Base(path)
	return pathWithinRoot(path, "/etc/nginx/conf.d") && strings.HasPrefix(base, "megavpn-") && strings.HasSuffix(base, ".conf") && isSafeCleanupFilename(base)
}

func isEmergencySystemdUnitPath(path string) bool {
	path = cleanEmergencyCleanupPath(path)
	return pathWithinRoot(path, "/etc/systemd/system") && strings.HasSuffix(path, ".service") && isSafeSystemdUnitToken(filepath.Base(path))
}

func isEmergencyCleanupRunnableUnitForScope(unit, cleanupScope string) bool {
	unit = strings.TrimSpace(unit)
	if unit == "" || strings.HasPrefix(unit, "-") || !isSafeSystemdUnitToken(unit) {
		return false
	}
	if unit == "nginx.service" || unit == "nginx" {
		return false
	}
	if strings.HasPrefix(unit, "openvpn-server@") || strings.HasPrefix(unit, "wg-quick@") {
		return true
	}
	return isEmergencyManagedUnitForScope(unit, cleanupScope)
}

func cleanEmergencyCleanupPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "\x00") || !filepath.IsAbs(path) {
		return ""
	}
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == "." || part == ".." {
			return ""
		}
	}
	for _, r := range path {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return ""
		}
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == "/" {
		return ""
	}
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == "." || part == ".." {
			return ""
		}
	}
	return cleaned
}

func emergencyCleanupStrictInt(value any) (int, bool) {
	switch x := value.(type) {
	case int:
		return x, true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case float64:
		if x != float64(int(x)) {
			return 0, false
		}
		return int(x), true
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

func parseEmergencyCleanupRequestedAt(value any) (time.Time, error) {
	switch x := value.(type) {
	case time.Time:
		if x.IsZero() {
			return time.Time{}, errors.New("requested_at_required")
		}
		return x.UTC(), nil
	case string:
		x = strings.TrimSpace(x)
		if x == "" {
			return time.Time{}, errors.New("requested_at_required")
		}
		t, err := time.Parse(time.RFC3339Nano, x)
		if err != nil {
			return time.Time{}, errors.New("requested_at_invalid")
		}
		return t.UTC(), nil
	default:
		return time.Time{}, errors.New("requested_at_required")
	}
}

func isSafeCleanupIdentifier(value string, max int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > max {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func isSafeCleanupSlug(value string) bool {
	return isSafeCleanupIdentifier(value, 128) && !strings.Contains(value, "..") && !strings.ContainsAny(value, `/\`)
}

func isSafeCleanupDisplayValue(value string, max int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > max || strings.Contains(value, "\x00") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func isSafeCleanupFilename(value string) bool {
	return isSafeCleanupIdentifier(value, 180) && !strings.Contains(value, "..") && !strings.ContainsAny(value, `/\`)
}

func safeCleanupCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('_')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "cleanup_failed"
	}
	return truncateSafe(out, emergencyCleanupMaxCodeLength)
}

func isCriticalCleanupCode(code string) bool {
	return code != "" && code != "cleanup_removed" && code != "cleanup_absent"
}

func truncateSafe(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func compactStrings(items []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type agentSelfRemovalManager struct {
	pendingDir string
	runDir     string
	run        func(context.Context, string, ...string) (int, string)
	clock      func() time.Time
}

type pendingAgentSelfRemovalMarker struct {
	JobID      string    `json:"job_id"`
	ActionType string    `json:"action_type"`
	CreatedAt  time.Time `json:"created_at"`
	RetryCount int       `json:"retry_count"`
	NextRetry  time.Time `json:"next_retry_at"`
}

var defaultAgentSelfRemovalManager = agentSelfRemovalManager{
	pendingDir: agentSelfRemovalPendingDir,
	runDir:     agentSelfRemovalRunDir,
	run:        runInstallCommand,
	clock:      func() time.Time { return time.Now().UTC() },
}

func processPendingAgentSelfRemoval(ctx context.Context) (bool, error) {
	return defaultAgentSelfRemovalManager.processPending(ctx)
}

func (m agentSelfRemovalManager) activateAfterAck(ctx context.Context, jobID string) error {
	marker, err := m.writePendingMarker(jobID)
	if err != nil {
		return err
	}
	if err := m.schedule(ctx, marker); err != nil {
		_ = m.recordScheduleFailure(marker)
		return err
	}
	_ = os.Remove(m.markerPath(marker.JobID))
	return ErrAgentSelfRemovalScheduled
}

func (m agentSelfRemovalManager) processPending(ctx context.Context) (bool, error) {
	entries, err := os.ReadDir(m.pendingDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("agent_self_removal_pending_read_failed")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(m.pendingDir, entry.Name()))
		if err != nil {
			return false, fmt.Errorf("agent_self_removal_pending_read_failed")
		}
		var marker pendingAgentSelfRemovalMarker
		if err := json.Unmarshal(body, &marker); err != nil || marker.ActionType != "agent_self_removal" || marker.JobID == "" {
			return false, fmt.Errorf("agent_self_removal_pending_marker_invalid")
		}
		if now := m.now(); !marker.NextRetry.IsZero() && now.Before(marker.NextRetry) {
			return false, fmt.Errorf("agent_self_removal_pending_backoff")
		}
		if err := m.schedule(ctx, marker); err != nil {
			_ = m.recordScheduleFailure(marker)
			return false, err
		}
		_ = os.Remove(m.markerPath(marker.JobID))
		return true, nil
	}
	return false, nil
}

func (m agentSelfRemovalManager) writePendingMarker(jobID string) (pendingAgentSelfRemovalMarker, error) {
	marker := pendingAgentSelfRemovalMarker{
		JobID:      strings.TrimSpace(jobID),
		ActionType: "agent_self_removal",
		CreatedAt:  m.now(),
		NextRetry:  m.now(),
	}
	if marker.JobID == "" {
		return marker, fmt.Errorf("agent_self_removal_job_id_required")
	}
	if err := os.MkdirAll(m.pendingDir, 0o700); err != nil {
		return marker, fmt.Errorf("agent_self_removal_pending_write_failed")
	}
	body, _ := json.MarshalIndent(marker, "", "  ")
	if err := os.WriteFile(m.markerPath(marker.JobID), body, 0o600); err != nil {
		return marker, fmt.Errorf("agent_self_removal_pending_write_failed")
	}
	return marker, nil
}

func (m agentSelfRemovalManager) recordScheduleFailure(marker pendingAgentSelfRemovalMarker) error {
	marker.RetryCount++
	delay := retryDelay(marker.RetryCount-1, 5*time.Second, 2*time.Minute)
	marker.NextRetry = m.now().Add(delay)
	body, _ := json.MarshalIndent(marker, "", "  ")
	return os.WriteFile(m.markerPath(marker.JobID), body, 0o600)
}

func (m agentSelfRemovalManager) schedule(ctx context.Context, marker pendingAgentSelfRemovalMarker) error {
	id := safeAgentSelfRemovalID(marker.JobID)
	workDir := filepath.Join(m.runDir, id)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return fmt.Errorf("agent_self_removal_schedule_failed")
	}
	ackFile := filepath.Join(workDir, "ack")
	if err := os.WriteFile(ackFile, []byte("acknowledged\n"), 0o600); err != nil {
		return fmt.Errorf("agent_self_removal_schedule_failed")
	}
	scriptPath := filepath.Join(workDir, "self-remove.sh")
	if err := os.WriteFile(scriptPath, []byte(agentSelfRemovalScript), 0o700); err != nil {
		return fmt.Errorf("agent_self_removal_schedule_failed")
	}
	unit := "megavpn-agent-self-remove-" + id
	if !isSafeSystemdUnitToken(unit + ".service") {
		return fmt.Errorf("agent_self_removal_unit_invalid")
	}
	code, _ := m.command(ctx, "systemd-run", "--unit", unit, "--on-active=5s", "/bin/sh", scriptPath, ackFile)
	if code != 0 {
		return fmt.Errorf("agent_self_removal_schedule_failed")
	}
	return nil
}

const agentSelfRemovalScript = `#!/bin/sh
set -eu
ack_file="$1"
if [ ! -f "$ack_file" ]; then
  exit 0
fi
systemctl disable --now megavpn-agent.service >/dev/null 2>&1 || true
rm -f /etc/systemd/system/megavpn-agent.service
rm -f /etc/megavpn/agent.env /etc/megavpn/agent-bootstrap.env
rm -rf /var/lib/megavpn/agent
rm -f /opt/megavpn/bin/megavpn-agent
rmdir /opt/megavpn/bin >/dev/null 2>&1 || true
rmdir /opt/megavpn >/dev/null 2>&1 || true
systemctl daemon-reload >/dev/null 2>&1 || true
rm -f "$0"
`

func (m agentSelfRemovalManager) markerPath(jobID string) string {
	return filepath.Join(m.pendingDir, safeAgentSelfRemovalID(jobID)+".json")
}

func (m agentSelfRemovalManager) command(ctx context.Context, name string, args ...string) (int, string) {
	if m.run == nil {
		m.run = runInstallCommand
	}
	return m.run(ctx, name, args...)
}

func (m agentSelfRemovalManager) now() time.Time {
	if m.clock == nil {
		return time.Now().UTC()
	}
	return m.clock().UTC()
}

func safeAgentSelfRemovalID(jobID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(jobID)))
	return "job-" + hex.EncodeToString(sum[:])[:24]
}

func agentSelfRemovalMarkerContainsSecret(body []byte) bool {
	lower := strings.ToLower(string(body))
	for _, token := range []string{"agent_token", "authorization", "signature", "secret", "private_key", "cleanup_plan", "reason"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}
