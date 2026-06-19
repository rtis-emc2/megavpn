package postgres

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
	"github.com/rtis-emc2/megavpn/internal/service/driver"
)

const (
	instanceRuntimeObservationRetention          = 30 * 24 * time.Hour
	instanceRuntimeObservationMaxRowsPerInstance = 1000
)

func (s *Store) ListInstanceRuntimeStates(ctx context.Context) ([]domain.InstanceRuntimeState, error) {
	rows, err := s.db.Query(ctx, `select
		id,instance_id,node_id,service_code,systemd_unit,desired_status,runtime_status,health_status,drift_status,active_state,
		enabled_state,config_hash,last_job_id,last_job_type,last_job_status,applied_revision_id,observed_revision_id,endpoint_host,coalesce(endpoint_port,0),
		listening_ports_json,result_json,error_text,agent_reported_at,checked_at,updated_at
		from instance_runtime_states
		order by updated_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.InstanceRuntimeState{}
	for rows.Next() {
		item, err := scanInstanceRuntimeState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetInstanceRuntimeState(ctx context.Context, instanceID string) (domain.InstanceRuntimeState, error) {
	row := s.db.QueryRow(ctx, `select
		id,instance_id,node_id,service_code,systemd_unit,desired_status,runtime_status,health_status,drift_status,active_state,
		enabled_state,config_hash,last_job_id,last_job_type,last_job_status,applied_revision_id,observed_revision_id,endpoint_host,coalesce(endpoint_port,0),
		listening_ports_json,result_json,error_text,agent_reported_at,checked_at,updated_at
		from instance_runtime_states
		where instance_id=$1`, instanceID)
	return scanInstanceRuntimeState(row)
}

func (s *Store) ListInstanceRuntimeObservations(ctx context.Context, instanceID string, limit int) ([]domain.InstanceRuntimeObservation, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `select
		id,instance_id,node_id,source,service_code,systemd_unit,desired_status,runtime_status,health_status,drift_status,active_state,
		enabled_state,config_hash,last_job_id,last_job_type,last_job_status,applied_revision_id,observed_revision_id,endpoint_host,coalesce(endpoint_port,0),
		listening_ports_json,result_json,error_text,observed_at,received_at
		from instance_runtime_observations
		where instance_id=$1
		order by observed_at desc, received_at desc
		limit $2`, instanceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.InstanceRuntimeObservation{}
	for rows.Next() {
		item, err := scanInstanceRuntimeObservation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListAgentInstanceRuntimeTargets(ctx context.Context, nodeID string) ([]domain.AgentInstanceRuntimeTarget, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `select
			i.id,
			i.node_id,
			sd.code,
			i.slug,
			coalesce(i.systemd_unit,''),
			coalesce(i.endpoint_host,''),
			coalesce(i.endpoint_port,0),
			i.status,
			i.enabled,
			i.current_revision_id,
			i.last_applied_revision_id
		from instances i
		join service_definitions sd on sd.id=i.service_definition_id
		where i.node_id=$1 and i.status <> 'deleted'
		order by i.created_at asc`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AgentInstanceRuntimeTarget{}
	for rows.Next() {
		var target domain.AgentInstanceRuntimeTarget
		if err := rows.Scan(
			&target.InstanceID,
			&target.NodeID,
			&target.ServiceCode,
			&target.Slug,
			&target.SystemdUnit,
			&target.EndpointHost,
			&target.EndpointPort,
			&target.DesiredStatus,
			&target.DesiredEnabled,
			&target.CurrentRevisionID,
			&target.AppliedRevisionID,
		); err != nil {
			return nil, err
		}
		target.ServiceCode = normalizeInstanceRuntimeCode(target.ServiceCode)
		target.ConfigPath = s.agentRuntimeTargetConfigPath(ctx, target)
		out = append(out, target)
	}
	return out, rows.Err()
}

func (s *Store) SubmitAgentInstanceRuntimeReports(ctx context.Context, nodeID string, reports []domain.AgentInstanceRuntimeReport) ([]domain.InstanceRuntimeState, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, nil
	}
	if len(reports) > 500 {
		reports = reports[:500]
	}
	out := make([]domain.InstanceRuntimeState, 0, len(reports))
	for _, report := range reports {
		report.InstanceID = strings.TrimSpace(report.InstanceID)
		if report.InstanceID == "" {
			continue
		}
		item, err := s.upsertInstanceRuntimeStateForAgentReport(ctx, nodeID, report)
		if err != nil {
			return out, err
		}
		out = append(out, item)
	}
	if _, err := s.db.Exec(ctx, `update node_agents set last_runtime_sync_at=now(), last_seen_at=now(), status='active' where node_id=$1`, nodeID); err != nil {
		return out, err
	}
	if len(out) > 0 {
		if err := s.pruneInstanceRuntimeObservations(ctx); err != nil {
			return out, err
		}
	}
	return out, nil
}

type instanceRuntimeStateScanner interface {
	Scan(dest ...any) error
}

type instanceRuntimeObservationScanner interface {
	Scan(dest ...any) error
}

func scanInstanceRuntimeState(row instanceRuntimeStateScanner) (domain.InstanceRuntimeState, error) {
	var item domain.InstanceRuntimeState
	var resultRaw []byte
	var listeningPortsRaw []byte
	if err := row.Scan(
		&item.ID,
		&item.InstanceID,
		&item.NodeID,
		&item.ServiceCode,
		&item.SystemdUnit,
		&item.DesiredStatus,
		&item.RuntimeStatus,
		&item.HealthStatus,
		&item.DriftStatus,
		&item.ActiveState,
		&item.EnabledState,
		&item.ConfigHash,
		&item.LastJobID,
		&item.LastJobType,
		&item.LastJobStatus,
		&item.AppliedRevisionID,
		&item.ObservedRevisionID,
		&item.EndpointHost,
		&item.EndpointPort,
		&listeningPortsRaw,
		&resultRaw,
		&item.ErrorText,
		&item.AgentReportedAt,
		&item.CheckedAt,
		&item.UpdatedAt,
	); err != nil {
		return domain.InstanceRuntimeState{}, err
	}
	_ = json.Unmarshal(resultRaw, &item.Result)
	if item.Result == nil {
		item.Result = map[string]any{}
	}
	_ = json.Unmarshal(listeningPortsRaw, &item.ListeningPorts)
	if item.ListeningPorts == nil {
		item.ListeningPorts = []map[string]any{}
	}
	enrichInstanceRuntimeStateProjection(&item)
	return item, nil
}

func scanInstanceRuntimeObservation(row instanceRuntimeObservationScanner) (domain.InstanceRuntimeObservation, error) {
	var item domain.InstanceRuntimeObservation
	var resultRaw []byte
	var listeningPortsRaw []byte
	if err := row.Scan(
		&item.ID,
		&item.InstanceID,
		&item.NodeID,
		&item.Source,
		&item.ServiceCode,
		&item.SystemdUnit,
		&item.DesiredStatus,
		&item.RuntimeStatus,
		&item.HealthStatus,
		&item.DriftStatus,
		&item.ActiveState,
		&item.EnabledState,
		&item.ConfigHash,
		&item.LastJobID,
		&item.LastJobType,
		&item.LastJobStatus,
		&item.AppliedRevisionID,
		&item.ObservedRevisionID,
		&item.EndpointHost,
		&item.EndpointPort,
		&listeningPortsRaw,
		&resultRaw,
		&item.ErrorText,
		&item.ObservedAt,
		&item.ReceivedAt,
	); err != nil {
		return domain.InstanceRuntimeObservation{}, err
	}
	_ = json.Unmarshal(resultRaw, &item.Result)
	if item.Result == nil {
		item.Result = map[string]any{}
	}
	_ = json.Unmarshal(listeningPortsRaw, &item.ListeningPorts)
	if item.ListeningPorts == nil {
		item.ListeningPorts = []map[string]any{}
	}
	enrichInstanceRuntimeObservationProjection(&item)
	return item, nil
}

type runtimeProjectionInput struct {
	ServiceCode        string
	DesiredStatus      string
	RuntimeStatus      string
	HealthStatus       string
	DriftStatus        string
	ActiveState        string
	EnabledState       string
	ConfigHash         string
	LastJobType        string
	LastJobStatus      string
	AppliedRevisionID  *string
	ObservedRevisionID *string
	EndpointHost       string
	EndpointPort       int
	ListeningPorts     []map[string]any
	Result             map[string]any
	ErrorText          string
}

func enrichInstanceRuntimeStateProjection(item *domain.InstanceRuntimeState) {
	checks, healthReasons, driftReasons := buildRuntimeProjection(runtimeProjectionInput{
		ServiceCode:        item.ServiceCode,
		DesiredStatus:      item.DesiredStatus,
		RuntimeStatus:      item.RuntimeStatus,
		HealthStatus:       item.HealthStatus,
		DriftStatus:        item.DriftStatus,
		ActiveState:        item.ActiveState,
		EnabledState:       item.EnabledState,
		ConfigHash:         item.ConfigHash,
		LastJobType:        item.LastJobType,
		LastJobStatus:      item.LastJobStatus,
		AppliedRevisionID:  item.AppliedRevisionID,
		ObservedRevisionID: item.ObservedRevisionID,
		EndpointHost:       item.EndpointHost,
		EndpointPort:       item.EndpointPort,
		ListeningPorts:     item.ListeningPorts,
		Result:             item.Result,
		ErrorText:          item.ErrorText,
	})
	item.HealthChecks = checks
	item.HealthReasons = healthReasons
	item.DriftReasons = driftReasons
}

func enrichInstanceRuntimeObservationProjection(item *domain.InstanceRuntimeObservation) {
	checks, healthReasons, driftReasons := buildRuntimeProjection(runtimeProjectionInput{
		ServiceCode:        item.ServiceCode,
		DesiredStatus:      item.DesiredStatus,
		RuntimeStatus:      item.RuntimeStatus,
		HealthStatus:       item.HealthStatus,
		DriftStatus:        item.DriftStatus,
		ActiveState:        item.ActiveState,
		EnabledState:       item.EnabledState,
		ConfigHash:         item.ConfigHash,
		LastJobType:        item.LastJobType,
		LastJobStatus:      item.LastJobStatus,
		AppliedRevisionID:  item.AppliedRevisionID,
		ObservedRevisionID: item.ObservedRevisionID,
		EndpointHost:       item.EndpointHost,
		EndpointPort:       item.EndpointPort,
		ListeningPorts:     item.ListeningPorts,
		Result:             item.Result,
		ErrorText:          item.ErrorText,
	})
	item.HealthChecks = checks
	item.HealthReasons = healthReasons
	item.DriftReasons = driftReasons
}

func buildRuntimeProjection(input runtimeProjectionInput) ([]domain.RuntimeCheck, []string, []string) {
	checks := evaluateDriverHealthChecks(input)
	return checks, healthReasonsForRuntimeProjection(input, checks), driftReasonsForRuntimeProjection(input)
}

func evaluateDriverHealthChecks(input runtimeProjectionInput) []domain.RuntimeCheck {
	specs := driver.HealthChecksFor(input.ServiceCode)
	if len(specs) == 0 {
		return nil
	}
	checks := make([]domain.RuntimeCheck, 0, len(specs))
	for _, spec := range specs {
		check := domain.RuntimeCheck{
			Code:        spec.Code,
			DisplayName: spec.DisplayName,
			Signal:      spec.Signal,
			Source:      spec.Source,
			Required:    spec.Required,
		}
		switch spec.Code {
		case driver.HealthCheckSystemdActive:
			check = evaluateSystemdActiveCheck(input, check)
		case driver.HealthCheckConfigObserved:
			check = evaluateConfigObservedCheck(input, check)
		case driver.HealthCheckEndpointListening:
			check = evaluateEndpointListeningCheck(input, check)
		default:
			check.Status = "unknown"
			check.Message = "Driver health check is registered but not implemented in runtime projection."
		}
		checks = append(checks, check)
	}
	return checks
}

func evaluateSystemdActiveCheck(input runtimeProjectionInput, check domain.RuntimeCheck) domain.RuntimeCheck {
	observed := normalizeRuntimeObservationState(firstString(input.ActiveState, firstString(input.Result["active_state"])))
	check.Observed = observed
	if runtimeProjectionDesiredStopped(input) {
		check.Expected = "inactive"
		switch observed {
		case "inactive", "stopped":
			check.Status = "ok"
			check.Message = "Systemd unit is stopped as desired."
		case "active":
			check.Status = "warning"
			check.Message = "Systemd unit is active while the instance is desired stopped."
		case "failed":
			check.Status = "warning"
			check.Message = "Systemd unit is failed while the instance is desired stopped."
		default:
			check.Status = "unknown"
			check.Message = "Systemd unit state is not available yet."
		}
		return check
	}
	check.Expected = "active"
	switch observed {
	case "active":
		check.Status = "ok"
		check.Message = "Systemd unit is active."
	case "failed":
		check.Status = "failed"
		check.Message = "Systemd unit is failed."
	case "inactive", "stopped":
		check.Status = "warning"
		check.Message = "Systemd unit is not active."
	case "activating", "deactivating", "reloading":
		check.Status = "warning"
		check.Message = "Systemd unit is transitioning."
	default:
		check.Status = "unknown"
		check.Message = "Systemd unit state is not available yet."
	}
	return check
}

func evaluateConfigObservedCheck(input runtimeProjectionInput, check domain.RuntimeCheck) domain.RuntimeCheck {
	configHash := strings.TrimSpace(firstString(input.ConfigHash, input.Result["config_hash"]))
	check.Expected = "config hash"
	if configHash != "" {
		check.Status = "ok"
		check.Observed = configHash
		check.Message = "Rendered config hash was observed by the agent."
		return check
	}
	if strings.TrimSpace(input.ErrorText) != "" {
		check.Status = "failed"
		check.Message = "Agent reported a runtime error before config hash could be observed."
		return check
	}
	if input.LastJobType == "agent.runtime.report" {
		check.Status = "warning"
		check.Message = "Agent runtime report did not include a config hash."
		return check
	}
	check.Status = "unknown"
	check.Message = "Waiting for an agent runtime report with rendered config hash."
	return check
}

func evaluateEndpointListeningCheck(input runtimeProjectionInput, check domain.RuntimeCheck) domain.RuntimeCheck {
	check.Expected = input.EndpointPort
	check.Observed = input.ListeningPorts
	if input.EndpointPort <= 0 {
		check.Status = "skipped"
		check.Message = "No endpoint port is configured for this instance."
		return check
	}
	observed := runtimeProjectionHasListeningPort(input.ListeningPorts, input.EndpointPort)
	if runtimeProjectionDesiredStopped(input) {
		if observed {
			check.Status = "warning"
			check.Message = "Endpoint port is still listening while the instance is desired stopped."
			return check
		}
		if len(input.ListeningPorts) == 0 && input.LastJobType != "agent.runtime.report" {
			check.Status = "unknown"
			check.Message = "Waiting for an agent listening-port report."
			return check
		}
		check.Status = "ok"
		check.Message = "Endpoint port is not listening while the instance is stopped."
		return check
	}
	if observed {
		check.Status = "ok"
		check.Message = "Configured endpoint port is listening."
		return check
	}
	if len(input.ListeningPorts) == 0 && input.LastJobType != "agent.runtime.report" {
		check.Status = "unknown"
		check.Message = "Waiting for an agent listening-port report."
		return check
	}
	check.Status = "warning"
	check.Message = "Configured endpoint port is not present in the agent listening-port report."
	return check
}

func healthReasonsForRuntimeProjection(input runtimeProjectionInput, checks []domain.RuntimeCheck) []string {
	reasons := make([]string, 0, 4)
	if errText := strings.TrimSpace(input.ErrorText); errText != "" {
		reasons = append(reasons, "Runtime error: "+truncateRuntimeReason(errText))
	}
	for _, check := range checks {
		if check.Status == "ok" || check.Status == "skipped" || strings.TrimSpace(check.Message) == "" {
			continue
		}
		reasons = append(reasons, check.Message)
	}
	if len(reasons) == 0 {
		switch strings.ToLower(strings.TrimSpace(input.HealthStatus)) {
		case "healthy":
			reasons = append(reasons, "All required runtime checks passed.")
		case "stopped":
			reasons = append(reasons, "Instance runtime is stopped by desired state or last lifecycle action.")
		case "unknown", "":
			reasons = append(reasons, "Runtime health has not been observed yet.")
		default:
			reasons = append(reasons, "Runtime health status is "+input.HealthStatus+".")
		}
	}
	return reasons
}

func driftReasonsForRuntimeProjection(input runtimeProjectionInput) []string {
	status := strings.ToLower(strings.TrimSpace(input.DriftStatus))
	switch status {
	case "in_sync":
		return []string{"Applied revision and observed runtime state are in sync."}
	case "pending_apply":
		return []string{"Current desired revision has not been applied to the node yet."}
	case "drifted":
		reasons := make([]string, 0, 3)
		if input.ObservedRevisionID != nil && input.AppliedRevisionID != nil && strings.TrimSpace(*input.ObservedRevisionID) != strings.TrimSpace(*input.AppliedRevisionID) {
			reasons = append(reasons, "Agent observed a revision that differs from the applied revision.")
		}
		if runtimeProjectionDesiredStopped(input) && normalizeRuntimeObservationState(input.ActiveState) == "active" {
			reasons = append(reasons, "Instance is desired stopped but systemd unit is still active.")
		}
		if !runtimeProjectionDesiredStopped(input) {
			activeState := normalizeRuntimeObservationState(input.ActiveState)
			if activeState != "active" && activeState != "unknown" {
				reasons = append(reasons, "Instance is desired running but systemd unit is "+activeState+".")
			}
		}
		if len(reasons) == 0 {
			reasons = append(reasons, "Runtime observation differs from the desired instance state.")
		}
		return reasons
	case "unknown", "":
		return []string{"Runtime drift has not been observed yet."}
	default:
		return []string{"Runtime drift status is " + input.DriftStatus + "."}
	}
}

func runtimeProjectionDesiredStopped(input runtimeProjectionInput) bool {
	if input.LastJobType == "instance.stop" || input.LastJobType == "instance.disable" {
		return true
	}
	return runtimeProjectionStoppedValue(input.HealthStatus) ||
		runtimeProjectionStoppedValue(input.RuntimeStatus) ||
		runtimeProjectionStoppedValue(input.DesiredStatus)
}

func runtimeProjectionStoppedValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "stopped", "disabled", "deleted", "deleting", "inactive":
		return true
	default:
		return false
	}
}

func runtimeProjectionHasListeningPort(ports []map[string]any, endpointPort int) bool {
	if endpointPort <= 0 {
		return true
	}
	want := int64(endpointPort)
	for _, port := range ports {
		if intFromAny(port["port"]) == want {
			return true
		}
		if portFromRuntimeLocalAddress(firstString(port["local_address"])) == want {
			return true
		}
	}
	return false
}

func portFromRuntimeLocalAddress(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if idx := strings.LastIndex(value, "]:"); idx >= 0 {
		n, _ := strconv.ParseInt(strings.TrimSpace(value[idx+2:]), 10, 64)
		return n
	}
	if idx := strings.LastIndex(value, ":"); idx >= 0 {
		n, _ := strconv.ParseInt(strings.TrimSpace(value[idx+1:]), 10, 64)
		return n
	}
	n, _ := strconv.ParseInt(strings.Trim(value, "[]"), 10, 64)
	return n
}

func truncateRuntimeReason(value string) string {
	value = strings.TrimSpace(value)
	const max = 240
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}

func (s *Store) upsertInstanceRuntimeStateForJob(ctx context.Context, instanceID, jobID, jobType, jobStatus string, result map[string]any) error {
	instance, err := s.GetInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	if result == nil {
		result = map[string]any{}
	}
	activeState := strings.TrimSpace(firstString(result["active_state"], result["runtime_status"]))
	runtimeStatus := runtimeStatusFromJob(jobType, jobStatus, activeState)
	healthStatus := healthStatusFromRuntime(jobType, jobStatus, activeState)
	driftStatus := driftStatusForInstance(instance, jobType, jobStatus)
	errorText := strings.TrimSpace(firstString(result["error"], result["message"]))
	if jobStatus == "succeeded" {
		errorText = ""
	}
	resultRaw := mustJSON(result)
	now := time.Now().UTC()
	stateID := id.New()
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `insert into instance_runtime_states(
			id,instance_id,node_id,service_code,systemd_unit,desired_status,runtime_status,health_status,drift_status,active_state,
			enabled_state,config_hash,last_job_id,last_job_type,last_job_status,applied_revision_id,observed_revision_id,endpoint_host,endpoint_port,listening_ports_json,result_json,error_text,agent_reported_at,checked_at,updated_at
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'','',$11,$12,$13,$14,$15,$16,nullif($17,0),'[]'::jsonb,$18,$19,null,$20,$20)
		on conflict(instance_id) do update set
			node_id=excluded.node_id,
			service_code=excluded.service_code,
			systemd_unit=excluded.systemd_unit,
			desired_status=excluded.desired_status,
			runtime_status=excluded.runtime_status,
			health_status=excluded.health_status,
			drift_status=excluded.drift_status,
			active_state=excluded.active_state,
			last_job_id=excluded.last_job_id,
			last_job_type=excluded.last_job_type,
			last_job_status=excluded.last_job_status,
			applied_revision_id=excluded.applied_revision_id,
			observed_revision_id=excluded.observed_revision_id,
			endpoint_host=excluded.endpoint_host,
			endpoint_port=excluded.endpoint_port,
			result_json=excluded.result_json,
			error_text=excluded.error_text,
			checked_at=excluded.checked_at,
			updated_at=excluded.updated_at`,
		stateID,
		instance.ID,
		nullableString(instance.NodeID),
		instance.ServiceCode,
		instance.SystemdUnit,
		instance.Status,
		runtimeStatus,
		healthStatus,
		driftStatus,
		activeState,
		nullableString(jobID),
		jobType,
		jobStatus,
		instance.LastAppliedRevisionID,
		instance.CurrentRevisionID,
		instance.EndpointHost,
		instance.EndpointPort,
		resultRaw,
		errorText,
		now,
	)
	if err != nil {
		return err
	}
	if err := insertInstanceRuntimeObservation(ctx, tx, domain.InstanceRuntimeObservation{
		ID:                 id.New(),
		InstanceID:         instance.ID,
		NodeID:             nullableString(instance.NodeID),
		Source:             "job",
		ServiceCode:        instance.ServiceCode,
		SystemdUnit:        instance.SystemdUnit,
		DesiredStatus:      instance.Status,
		RuntimeStatus:      runtimeStatus,
		HealthStatus:       healthStatus,
		DriftStatus:        driftStatus,
		ActiveState:        activeState,
		EnabledState:       "",
		ConfigHash:         "",
		LastJobID:          nullableString(jobID),
		LastJobType:        jobType,
		LastJobStatus:      jobStatus,
		AppliedRevisionID:  instance.LastAppliedRevisionID,
		ObservedRevisionID: instance.CurrentRevisionID,
		EndpointHost:       instance.EndpointHost,
		EndpointPort:       instance.EndpointPort,
		ListeningPorts:     []map[string]any{},
		Result:             result,
		ErrorText:          errorText,
		ObservedAt:         now,
		ReceivedAt:         now,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) upsertInstanceRuntimeStateForAgentReport(ctx context.Context, nodeID string, report domain.AgentInstanceRuntimeReport) (domain.InstanceRuntimeState, error) {
	instance, err := s.GetInstance(ctx, report.InstanceID)
	if err != nil {
		return domain.InstanceRuntimeState{}, err
	}
	if strings.TrimSpace(instance.NodeID) != nodeID {
		return domain.InstanceRuntimeState{}, pgx.ErrNoRows
	}
	if report.CheckedAt == nil {
		now := time.Now().UTC()
		report.CheckedAt = &now
	}
	report.ServiceCode = normalizeInstanceRuntimeCode(firstString(report.ServiceCode, instance.ServiceCode))
	report.SystemdUnit = strings.TrimSpace(firstString(report.SystemdUnit, instance.SystemdUnit))
	report.ActiveState = normalizeRuntimeObservationState(report.ActiveState)
	report.EnabledState = strings.TrimSpace(report.EnabledState)
	report.ConfigHash = strings.TrimSpace(report.ConfigHash)
	report.ConfigPath = strings.TrimSpace(report.ConfigPath)
	report.ErrorText = strings.TrimSpace(report.ErrorText)
	runtimeStatus := runtimeStatusFromAgentReport(instance, report)
	healthStatus := healthStatusFromAgentReport(instance, report)
	driftStatus := driftStatusFromAgentReport(instance, report)
	result := map[string]any{
		"source":          "agent_runtime_report",
		"config_path":     report.ConfigPath,
		"config_hash":     report.ConfigHash,
		"active_state":    report.ActiveState,
		"enabled_state":   report.EnabledState,
		"listening_ports": report.ListeningPorts,
		"checked_at":      report.CheckedAt.UTC().Format(time.RFC3339),
	}
	listeningPortsRaw := mustJSON(report.ListeningPorts)
	resultRaw := mustJSON(result)
	receivedAt := time.Now().UTC()
	stateID := id.New()
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.InstanceRuntimeState{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `insert into instance_runtime_states(
			id,instance_id,node_id,service_code,systemd_unit,desired_status,runtime_status,health_status,drift_status,active_state,
			enabled_state,config_hash,last_job_id,last_job_type,last_job_status,applied_revision_id,observed_revision_id,endpoint_host,endpoint_port,listening_ports_json,result_json,error_text,agent_reported_at,checked_at,updated_at
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,null,'agent.runtime.report','succeeded',$13,$14,$15,nullif($16,0),$17,$18,$19,$20,$20,$21)
		on conflict(instance_id) do update set
			node_id=excluded.node_id,
			service_code=excluded.service_code,
			systemd_unit=excluded.systemd_unit,
			desired_status=excluded.desired_status,
			runtime_status=excluded.runtime_status,
			health_status=excluded.health_status,
			drift_status=excluded.drift_status,
			active_state=excluded.active_state,
			enabled_state=excluded.enabled_state,
			config_hash=excluded.config_hash,
			last_job_type=excluded.last_job_type,
			last_job_status=excluded.last_job_status,
			applied_revision_id=excluded.applied_revision_id,
			observed_revision_id=excluded.observed_revision_id,
			endpoint_host=excluded.endpoint_host,
			endpoint_port=excluded.endpoint_port,
			listening_ports_json=excluded.listening_ports_json,
			result_json=excluded.result_json,
			error_text=excluded.error_text,
			agent_reported_at=excluded.agent_reported_at,
			checked_at=excluded.checked_at,
			updated_at=excluded.updated_at`,
		stateID,
		instance.ID,
		nullableString(instance.NodeID),
		report.ServiceCode,
		report.SystemdUnit,
		instance.Status,
		runtimeStatus,
		healthStatus,
		driftStatus,
		report.ActiveState,
		report.EnabledState,
		report.ConfigHash,
		instance.LastAppliedRevisionID,
		report.ObservedRevisionID,
		instance.EndpointHost,
		instance.EndpointPort,
		listeningPortsRaw,
		resultRaw,
		report.ErrorText,
		*report.CheckedAt,
		receivedAt,
	)
	if err != nil {
		return domain.InstanceRuntimeState{}, err
	}
	if err := insertInstanceRuntimeObservation(ctx, tx, domain.InstanceRuntimeObservation{
		ID:                 id.New(),
		InstanceID:         instance.ID,
		NodeID:             nullableString(instance.NodeID),
		Source:             "agent",
		ServiceCode:        report.ServiceCode,
		SystemdUnit:        report.SystemdUnit,
		DesiredStatus:      instance.Status,
		RuntimeStatus:      runtimeStatus,
		HealthStatus:       healthStatus,
		DriftStatus:        driftStatus,
		ActiveState:        report.ActiveState,
		EnabledState:       report.EnabledState,
		ConfigHash:         report.ConfigHash,
		LastJobType:        "agent.runtime.report",
		LastJobStatus:      "succeeded",
		AppliedRevisionID:  instance.LastAppliedRevisionID,
		ObservedRevisionID: report.ObservedRevisionID,
		EndpointHost:       instance.EndpointHost,
		EndpointPort:       instance.EndpointPort,
		ListeningPorts:     report.ListeningPorts,
		Result:             result,
		ErrorText:          report.ErrorText,
		ObservedAt:         *report.CheckedAt,
		ReceivedAt:         receivedAt,
	}); err != nil {
		return domain.InstanceRuntimeState{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.InstanceRuntimeState{}, err
	}
	return s.GetInstanceRuntimeState(ctx, instance.ID)
}

func insertInstanceRuntimeObservation(ctx context.Context, tx pgx.Tx, obs domain.InstanceRuntimeObservation) error {
	if obs.ID == "" {
		obs.ID = id.New()
	}
	now := time.Now().UTC()
	if obs.ObservedAt.IsZero() {
		obs.ObservedAt = now
	}
	if obs.ReceivedAt.IsZero() {
		obs.ReceivedAt = now
	}
	obs.Source = strings.TrimSpace(obs.Source)
	obs.ServiceCode = normalizeInstanceRuntimeCode(obs.ServiceCode)
	obs.SystemdUnit = strings.TrimSpace(obs.SystemdUnit)
	obs.DesiredStatus = strings.TrimSpace(obs.DesiredStatus)
	obs.RuntimeStatus = strings.TrimSpace(obs.RuntimeStatus)
	obs.HealthStatus = strings.TrimSpace(obs.HealthStatus)
	obs.DriftStatus = strings.TrimSpace(obs.DriftStatus)
	obs.ActiveState = strings.TrimSpace(obs.ActiveState)
	obs.EnabledState = strings.TrimSpace(obs.EnabledState)
	obs.ConfigHash = strings.TrimSpace(obs.ConfigHash)
	obs.LastJobType = strings.TrimSpace(obs.LastJobType)
	obs.LastJobStatus = strings.TrimSpace(obs.LastJobStatus)
	obs.EndpointHost = strings.TrimSpace(obs.EndpointHost)
	obs.ErrorText = strings.TrimSpace(obs.ErrorText)
	if obs.Result == nil {
		obs.Result = map[string]any{}
	}
	if obs.ListeningPorts == nil {
		obs.ListeningPorts = []map[string]any{}
	}
	_, err := tx.Exec(ctx, `insert into instance_runtime_observations(
			id,instance_id,node_id,source,service_code,systemd_unit,desired_status,runtime_status,health_status,drift_status,active_state,
			enabled_state,config_hash,last_job_id,last_job_type,last_job_status,applied_revision_id,observed_revision_id,endpoint_host,endpoint_port,
			listening_ports_json,result_json,error_text,observed_at,received_at
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,nullif($20,0),$21,$22,$23,$24,$25)`,
		obs.ID,
		obs.InstanceID,
		obs.NodeID,
		obs.Source,
		obs.ServiceCode,
		obs.SystemdUnit,
		obs.DesiredStatus,
		obs.RuntimeStatus,
		obs.HealthStatus,
		obs.DriftStatus,
		obs.ActiveState,
		obs.EnabledState,
		obs.ConfigHash,
		obs.LastJobID,
		obs.LastJobType,
		obs.LastJobStatus,
		obs.AppliedRevisionID,
		obs.ObservedRevisionID,
		obs.EndpointHost,
		obs.EndpointPort,
		mustJSON(obs.ListeningPorts),
		mustJSON(obs.Result),
		obs.ErrorText,
		obs.ObservedAt,
		obs.ReceivedAt,
	)
	return err
}

func (s *Store) pruneInstanceRuntimeObservations(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-instanceRuntimeObservationRetention)
	if _, err := s.db.Exec(ctx, `delete from instance_runtime_observations where received_at < $1`, cutoff); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `with ranked as (
			select id, row_number() over(partition by instance_id order by observed_at desc, received_at desc) as rn
			from instance_runtime_observations
		)
		delete from instance_runtime_observations o
		using ranked r
		where o.id=r.id and r.rn > $1`, instanceRuntimeObservationMaxRowsPerInstance)
	return err
}

func (s *Store) agentRuntimeTargetConfigPath(ctx context.Context, target domain.AgentInstanceRuntimeTarget) string {
	spec, err := s.latestInstanceSpec(ctx, target.InstanceID)
	if err != nil {
		return defaultAgentRuntimeConfigPath(target)
	}
	if path := strings.TrimSpace(firstString(spec["config_path"])); path != "" {
		return path
	}
	if files, ok := spec["files"].([]any); ok {
		for _, raw := range files {
			file, _ := raw.(map[string]any)
			if file == nil {
				continue
			}
			path := strings.TrimSpace(firstString(file["path"]))
			if path != "" && !strings.HasSuffix(path, ".service") {
				return path
			}
		}
	}
	return defaultAgentRuntimeConfigPath(target)
}

func defaultAgentRuntimeConfigPath(target domain.AgentInstanceRuntimeTarget) string {
	if path := driver.DefaultConfigPath(target.ServiceCode, target.Slug); path != "" {
		return path
	}
	return ""
}

func normalizeRuntimeObservationState(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "active", "running", "listening":
		return "active"
	case "inactive", "dead", "stopped", "not-found", "not found":
		return "inactive"
	case "failed", "error":
		return "failed"
	case "activating", "deactivating", "reloading", "maintenance":
		return value
	default:
		if value == "" {
			return "unknown"
		}
		return value
	}
}

func runtimeStatusFromAgentReport(instance domain.Instance, report domain.AgentInstanceRuntimeReport) string {
	if report.ErrorText != "" {
		return "failed"
	}
	switch normalizeRuntimeObservationState(report.ActiveState) {
	case "active":
		return "active"
	case "inactive":
		return "stopped"
	case "failed":
		return "failed"
	case "activating", "deactivating", "reloading":
		return "transitioning"
	default:
		if !instance.Enabled || desiredInstanceStopped(instance) {
			return "stopped"
		}
		return "unknown"
	}
}

func healthStatusFromAgentReport(instance domain.Instance, report domain.AgentInstanceRuntimeReport) string {
	if report.ErrorText != "" {
		return "unhealthy"
	}
	activeState := normalizeRuntimeObservationState(report.ActiveState)
	if activeState == "failed" {
		return "unhealthy"
	}
	checks := evaluateDriverHealthChecks(runtimeProjectionInput{
		ServiceCode:    firstString(report.ServiceCode, instance.ServiceCode),
		DesiredStatus:  instance.Status,
		ActiveState:    activeState,
		ConfigHash:     report.ConfigHash,
		LastJobType:    "agent.runtime.report",
		EndpointHost:   instance.EndpointHost,
		EndpointPort:   instance.EndpointPort,
		ListeningPorts: report.ListeningPorts,
		ErrorText:      report.ErrorText,
	})
	if desiredInstanceStopped(instance) {
		if activeState == "active" {
			return "degraded"
		}
		return "stopped"
	}
	if runtimeChecksHaveStatus(checks, "failed") {
		return "unhealthy"
	}
	if activeState != "active" {
		if activeState == "unknown" {
			return "unknown"
		}
		return "degraded"
	}
	if runtimeChecksHaveStatus(checks, "warning") {
		return "degraded"
	}
	return "healthy"
}

func runtimeChecksHaveStatus(checks []domain.RuntimeCheck, statuses ...string) bool {
	for _, check := range checks {
		for _, status := range statuses {
			if check.Status == status {
				return true
			}
		}
	}
	return false
}

func driftStatusFromAgentReport(instance domain.Instance, report domain.AgentInstanceRuntimeReport) string {
	activeState := normalizeRuntimeObservationState(report.ActiveState)
	if instance.CurrentRevisionID != nil && instance.LastAppliedRevisionID != nil && *instance.CurrentRevisionID != *instance.LastAppliedRevisionID {
		return "pending_apply"
	}
	if instance.CurrentRevisionID != nil && instance.LastAppliedRevisionID == nil {
		return "pending_apply"
	}
	if report.ObservedRevisionID != nil && instance.CurrentRevisionID != nil && strings.TrimSpace(*report.ObservedRevisionID) != strings.TrimSpace(*instance.CurrentRevisionID) {
		return "drifted"
	}
	if desiredInstanceStopped(instance) && activeState == "active" {
		return "drifted"
	}
	if !desiredInstanceStopped(instance) && activeState != "active" {
		return "drifted"
	}
	if instance.CurrentRevisionID != nil && instance.LastAppliedRevisionID != nil && *instance.CurrentRevisionID == *instance.LastAppliedRevisionID {
		return "in_sync"
	}
	return "unknown"
}

func desiredInstanceStopped(instance domain.Instance) bool {
	status := strings.ToLower(strings.TrimSpace(instance.Status))
	return !instance.Enabled || status == "disabled" || status == "deleted" || status == "deleting"
}

func intFromAny(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func runtimeStatusFromJob(jobType, jobStatus, activeState string) string {
	if jobStatus == "failed" {
		return "failed"
	}
	if jobStatus == "cancelled" {
		return "unknown"
	}
	switch jobType {
	case "instance.stop", "instance.disable":
		return "stopped"
	case "instance.start", "instance.enable", "instance.restart", "instance.apply":
		if activeState != "" {
			return activeState
		}
		return "active"
	default:
		if activeState != "" {
			return activeState
		}
		return "unknown"
	}
}

func healthStatusFromRuntime(jobType, jobStatus, activeState string) string {
	if jobStatus == "failed" {
		return "unhealthy"
	}
	if jobStatus == "cancelled" {
		return "unknown"
	}
	if jobType == "instance.stop" || jobType == "instance.disable" {
		return "stopped"
	}
	switch strings.ToLower(strings.TrimSpace(activeState)) {
	case "", "active", "running":
		return "healthy"
	case "inactive", "deactivating", "activating":
		return "degraded"
	case "failed":
		return "unhealthy"
	default:
		return "unknown"
	}
}

func driftStatusForInstance(instance domain.Instance, jobType, jobStatus string) string {
	if jobStatus != "succeeded" {
		return "unknown"
	}
	if jobType == "instance.apply" {
		return "in_sync"
	}
	if instance.CurrentRevisionID != nil && instance.LastAppliedRevisionID != nil && *instance.CurrentRevisionID == *instance.LastAppliedRevisionID {
		return "in_sync"
	}
	if instance.CurrentRevisionID != nil {
		return "pending_apply"
	}
	return "unknown"
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

var _ instanceRuntimeStateScanner = pgx.Row(nil)
