package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

func (c client) reportInstanceRuntime(ctx context.Context, nodeID string) error {
	targets, err := c.listRuntimeTargets(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	reports := make([]instanceRuntimeReport, 0, len(targets))
	for _, target := range targets {
		report := collectInstanceRuntimeReport(target)
		if strings.TrimSpace(report.InstanceID) == "" {
			continue
		}
		reports = append(reports, report)
	}
	if len(reports) == 0 {
		return nil
	}
	return c.submitRuntimeReports(ctx, nodeID, reports)
}

func collectInstanceRuntimeReport(target instanceRuntimeTarget) instanceRuntimeReport {
	now := time.Now().UTC()
	configHash, configErr := fileSHA256(target.ConfigPath)
	activeState := observedUnitState(target.SystemdUnit, "is-active")
	enabledState := observedUnitState(target.SystemdUnit, "is-enabled")
	var observedRevisionID *string
	if configHash != "" && target.AppliedRevisionID != nil && strings.TrimSpace(*target.AppliedRevisionID) != "" {
		observedRevisionID = target.AppliedRevisionID
	}
	errorText := ""
	if configErr != nil && target.ConfigPath != "" {
		errorText = configErr.Error()
	}
	if activeState == "failed" {
		status := strings.TrimSpace(runCombinedOutput("systemctl", "status", target.SystemdUnit, "--no-pager", "--lines=20"))
		if status != "" {
			errorText = truncate(status, 2048)
		}
	}
	return instanceRuntimeReport{
		InstanceID:         strings.TrimSpace(target.InstanceID),
		ServiceCode:        strings.TrimSpace(target.ServiceCode),
		SystemdUnit:        strings.TrimSpace(target.SystemdUnit),
		ConfigPath:         strings.TrimSpace(target.ConfigPath),
		ConfigHash:         configHash,
		ActiveState:        activeState,
		EnabledState:       enabledState,
		ObservedRevisionID: observedRevisionID,
		ListeningPorts:     matchingListeningPorts(target.EndpointPort),
		ErrorText:          errorText,
		CheckedAt:          &now,
	}
}

func observedUnitState(unit, command string) string {
	unit = strings.TrimSpace(unit)
	if unit == "" || !systemdAvailable() {
		return "unknown"
	}
	return normalizeSystemctlState(firstLine(runCombinedOutput("systemctl", command, unit)))
}

func fileSHA256(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func matchingListeningPorts(endpointPort int) []map[string]any {
	if endpointPort <= 0 {
		return nil
	}
	ports := collectListeningPorts()
	if len(ports) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, 2)
	for _, item := range ports {
		if portFromLocalAddress(stringify(item["local_address"])) != endpointPort {
			continue
		}
		item["port"] = endpointPort
		out = append(out, item)
	}
	return out
}

func portFromLocalAddress(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if idx := strings.LastIndex(value, "]:"); idx >= 0 {
		return atoiSafe(value[idx+2:])
	}
	if idx := strings.LastIndex(value, ":"); idx >= 0 {
		return atoiSafe(value[idx+1:])
	}
	return atoiSafe(value)
}

func atoiSafe(value string) int {
	n, _ := strconv.Atoi(strings.Trim(strings.TrimSpace(value), "[]"))
	return n
}
