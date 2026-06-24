package postgres

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rtis-emc2/megavpn/internal/backhaul"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

type routePolicyBackhaul struct {
	LinkID         string
	TransportID    string
	Driver         string
	InterfaceName  string
	IngressAddress string
	EgressAddress  string
	RoutingTable   string
	RouteMetric    int
	Status         string
}

func (s *Store) ListBackhaulLinks(ctx context.Context) ([]domain.BackhaulLink, error) {
	rows, err := s.db.Query(ctx, `select
		id::text,
		name,
		ingress_node_id::text,
		egress_node_id::text,
		status,
		selected_transport_id::text,
		desired_driver,
		routing_table,
		route_metric,
		failover_policy_json,
		metadata_json,
		created_at,
		updated_at
	from backhaul_links
	where status <> 'deleted'
	   or updated_at >= now() - interval '24 hours'
	order by created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BackhaulLink{}
	for rows.Next() {
		link, err := scanBackhaulLink(rows)
		if err != nil {
			return nil, err
		}
		link.Transports, err = s.listBackhaulTransports(ctx, link.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (s *Store) GetBackhaulLink(ctx context.Context, linkID string) (domain.BackhaulLink, error) {
	linkID = strings.TrimSpace(linkID)
	row := s.db.QueryRow(ctx, `select
		id::text,
		name,
		ingress_node_id::text,
		egress_node_id::text,
		status,
		selected_transport_id::text,
		desired_driver,
		routing_table,
		route_metric,
		failover_policy_json,
		metadata_json,
		created_at,
		updated_at
	from backhaul_links
	where id=$1 and status <> 'deleted'`, linkID)
	link, err := scanBackhaulLink(row)
	if err != nil {
		return domain.BackhaulLink{}, err
	}
	link.Transports, err = s.listBackhaulTransports(ctx, link.ID)
	if err != nil {
		return domain.BackhaulLink{}, err
	}
	return link, nil
}

func (s *Store) CreateBackhaulLink(ctx context.Context, link domain.BackhaulLink) (domain.BackhaulLink, error) {
	link.Name = strings.TrimSpace(link.Name)
	link.IngressNodeID = strings.TrimSpace(link.IngressNodeID)
	link.EgressNodeID = strings.TrimSpace(link.EgressNodeID)
	link.DesiredDriver = backhaul.NormalizeDriver(firstNonEmptyRouteValue(link.DesiredDriver, backhaul.DriverWireGuard))
	link.RoutingTable = strings.TrimSpace(link.RoutingTable)
	if link.RouteMetric <= 0 {
		link.RouteMetric = 50
	}
	if link.FailoverPolicy == nil {
		link.FailoverPolicy = map[string]any{"mode": "manual_priority", "auto_failover": false}
	}
	if link.Metadata == nil {
		link.Metadata = map[string]any{}
	}
	if link.Name == "" {
		link.Name = "backhaul-" + shortID(link.IngressNodeID) + "-" + shortID(link.EgressNodeID)
	}
	if !isSafeRouteIdentifier(link.Name) {
		return domain.BackhaulLink{}, fmt.Errorf("backhaul name must be a safe identifier")
	}
	if err := backhaul.ValidateDriver(link.DesiredDriver); err != nil {
		return domain.BackhaulLink{}, err
	}
	if link.IngressNodeID == "" || link.EgressNodeID == "" {
		return domain.BackhaulLink{}, fmt.Errorf("ingress_node_id and egress_node_id are required")
	}
	if link.IngressNodeID == link.EgressNodeID {
		return domain.BackhaulLink{}, fmt.Errorf("ingress and egress nodes must be different")
	}
	ingress, err := s.GetNode(ctx, link.IngressNodeID)
	if err != nil {
		return domain.BackhaulLink{}, fmt.Errorf("ingress node not found: %w", err)
	}
	egress, err := s.GetNode(ctx, link.EgressNodeID)
	if err != nil {
		return domain.BackhaulLink{}, fmt.Errorf("egress node not found: %w", err)
	}
	if !strings.EqualFold(ingress.Role, "ingress") {
		return domain.BackhaulLink{}, fmt.Errorf("ingress node must have role=ingress")
	}
	if !strings.EqualFold(egress.Role, "egress") {
		return domain.BackhaulLink{}, fmt.Errorf("egress node must have role=egress")
	}
	endpointHost := strings.TrimSpace(stringify(link.Metadata["endpoint_host"]))
	if endpointHost == "" {
		endpointHost = strings.TrimSpace(egress.Address)
	}
	if endpointHost == "" {
		return domain.BackhaulLink{}, fmt.Errorf("endpoint_host is required when egress node address is empty")
	}
	link.ID = id.New()
	if link.RoutingTable == "" || strings.EqualFold(link.RoutingTable, "auto") || strings.EqualFold(link.RoutingTable, "main") {
		link.RoutingTable = defaultBackhaulRoutingTable(link.ID)
	}
	link.Status = "planned"
	now := time.Now().UTC()
	link.CreatedAt = now
	link.UpdatedAt = now
	drivers := backhaulDriversFromMetadata(link.Metadata, link.DesiredDriver)
	tunnelCIDR := strings.TrimSpace(stringify(link.Metadata["tunnel_cidr"]))
	if tunnelCIDR == "" {
		tunnelCIDR = defaultBackhaulCIDR(link.ID)
	}
	if err := backhaul.ValidateTunnelCIDR(tunnelCIDR); err != nil {
		return domain.BackhaulLink{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.BackhaulLink{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `insert into backhaul_links(
		id,name,ingress_node_id,egress_node_id,status,desired_driver,routing_table,route_metric,failover_policy_json,metadata_json,created_at,updated_at
	) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		link.ID, link.Name, link.IngressNodeID, link.EgressNodeID, link.Status, link.DesiredDriver, link.RoutingTable, link.RouteMetric, mustJSON(link.FailoverPolicy), mustJSON(link.Metadata), now, now); err != nil {
		return domain.BackhaulLink{}, err
	}
	var selectedTransportID string
	usedTunnelCIDRs := map[string]bool{}
	for idx, driver := range drivers {
		def, _ := backhaul.Definition(driver)
		transportCIDR := uniqueBackhaulTunnelCIDR(link.ID, tunnelCIDR, def.Code, idx, usedTunnelCIDRs)
		transport, err := s.buildBackhaulTransport(ctx, link, def, endpointHost, transportCIDR, idx)
		if err != nil {
			return domain.BackhaulLink{}, err
		}
		usedTunnelCIDRs[transport.TunnelCIDR] = true
		if _, err := tx.Exec(ctx, `insert into backhaul_transports(
			id,link_id,driver,priority,status,endpoint_host,endpoint_port,protocol,interface_name,tunnel_cidr,ingress_address,egress_address,config_json,secret_refs_json,health_json,created_at,updated_at
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			transport.ID, transport.LinkID, transport.Driver, transport.Priority, transport.Status, transport.EndpointHost, transport.EndpointPort, transport.Protocol,
			transport.InterfaceName, transport.TunnelCIDR, transport.IngressAddress, transport.EgressAddress, mustJSON(transport.Config), mustJSON(transport.SecretRefs), mustJSON(transport.Health), now, now); err != nil {
			return domain.BackhaulLink{}, err
		}
		if transport.Driver == link.DesiredDriver {
			selectedTransportID = transport.ID
		}
	}
	if selectedTransportID == "" {
		return domain.BackhaulLink{}, fmt.Errorf("desired backhaul transport was not created")
	}
	if _, err := tx.Exec(ctx, `update backhaul_links set selected_transport_id=$2, updated_at=now() where id=$1`, link.ID, selectedTransportID); err != nil {
		return domain.BackhaulLink{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.BackhaulLink{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "backhaul.create", "backhaul", &link.ID, "managed backhaul link created")
	return s.GetBackhaulLink(ctx, link.ID)
}

func (s *Store) DeleteBackhaulLink(ctx context.Context, linkID string) (domain.BackhaulLink, error) {
	link, err := s.GetBackhaulLink(ctx, linkID)
	if err != nil {
		return domain.BackhaulLink{}, err
	}
	if _, err := s.db.Exec(ctx, `update backhaul_links set status='deleted', updated_at=now() where id=$1`, link.ID); err != nil {
		return domain.BackhaulLink{}, err
	}
	_, _ = s.CreateAudit(ctx, "system", "backhaul.delete", "backhaul", &link.ID, "managed backhaul link deleted")
	link.Status = "deleted"
	return link, nil
}

func (s *Store) CreateBackhaulProbeJobs(ctx context.Context, linkID string) ([]domain.Job, error) {
	link, err := s.GetBackhaulLink(ctx, linkID)
	if err != nil {
		return nil, err
	}
	transport := selectedBackhaulTransport(link)
	if transport == nil {
		return nil, fmt.Errorf("selected backhaul transport not found")
	}
	if link.Status != "active" || transport.Status != "active" {
		return nil, fmt.Errorf("backhaul link must be active before probe; apply selected driver first")
	}
	jobs := make([]domain.Job, 0, 2)
	for _, role := range []string{"ingress", "egress"} {
		nodeID := nodeIDForBackhaulRole(link, role)
		job, err := s.CreateJob(ctx, domain.Job{
			Type:      "node.backhaul.probe",
			ScopeType: "backhaul",
			ScopeID:   &link.ID,
			NodeID:    &nodeID,
			Priority:  70,
			Payload:   buildBackhaulProbePayload(link, *transport, role),
		})
		if err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	_, _ = s.CreateAudit(ctx, "system", "backhaul.probe", "backhaul", &link.ID, "managed backhaul probe queued")
	return jobs, nil
}

func (s *Store) CreateBackhaulDeleteJobs(ctx context.Context, linkID string) (domain.BackhaulLink, []domain.Job, error) {
	if _, err := s.RecoverStaleJobLeases(ctx); err != nil {
		return domain.BackhaulLink{}, nil, err
	}
	link, err := s.GetBackhaulLink(ctx, linkID)
	if err != nil {
		return domain.BackhaulLink{}, nil, err
	}
	if len(link.Transports) == 0 {
		deleted, err := s.DeleteBackhaulLink(ctx, link.ID)
		return deleted, nil, err
	}
	batchID := id.New()
	if link.Metadata == nil {
		link.Metadata = map[string]any{}
	}
	link.Metadata["delete_batch_id"] = batchID
	link.Metadata["delete_requested_at"] = time.Now().UTC().Format(time.RFC3339)
	jobs := make([]domain.Job, 0, len(link.Transports)*2)
	for _, transport := range link.Transports {
		for _, role := range []string{"ingress", "egress"} {
			nodeID := nodeIDForBackhaulRole(link, role)
			job, err := s.CreateJob(ctx, domain.Job{
				Type:      "node.backhaul.cleanup",
				ScopeType: "backhaul",
				ScopeID:   &link.ID,
				NodeID:    &nodeID,
				Priority:  90,
				Payload:   buildBackhaulCleanupPayload(link, transport, role, batchID),
			})
			if err != nil {
				return domain.BackhaulLink{}, jobs, err
			}
			jobs = append(jobs, job)
		}
	}
	_, _ = s.db.Exec(ctx, `update backhaul_links
		set status='disabled', metadata_json=$2, updated_at=now()
		where id=$1`, link.ID, mustJSON(link.Metadata))
	_, _ = s.db.Exec(ctx, `update backhaul_transports
		set status='disabled', last_error='', updated_at=now()
		where link_id=$1`, link.ID)
	_, _ = s.CreateAudit(ctx, "system", "backhaul.delete", "backhaul", &link.ID, "managed backhaul cleanup queued")
	link.Status = "disabled"
	return link, jobs, nil
}

func (s *Store) CreateBackhaulApplyJobs(ctx context.Context, linkID string) ([]domain.Job, error) {
	link, err := s.GetBackhaulLink(ctx, linkID)
	if err != nil {
		return nil, err
	}
	if changed, err := s.ensureBackhaulUniqueTransportCIDRs(ctx, link); err != nil {
		return nil, err
	} else if changed {
		link, err = s.GetBackhaulLink(ctx, linkID)
		if err != nil {
			return nil, err
		}
	}
	if len(link.Transports) == 0 {
		return nil, fmt.Errorf("backhaul transport profiles not found")
	}
	jobs := make([]domain.Job, 0, len(link.Transports)*2)
	for _, transport := range link.Transports {
		if _, ok := backhaul.Definition(transport.Driver); !ok {
			return jobs, fmt.Errorf("unsupported backhaul transport %q", transport.Driver)
		}
		for _, role := range []string{"ingress", "egress"} {
			payload, err := s.buildBackhaulApplyPayload(ctx, link, transport, role)
			if err != nil {
				return jobs, err
			}
			nodeID := nodeIDForBackhaulRole(link, role)
			job, err := s.CreateJob(ctx, domain.Job{
				Type:      "node.backhaul.apply",
				ScopeType: "backhaul",
				ScopeID:   &link.ID,
				NodeID:    &nodeID,
				Priority:  75,
				Payload:   payload,
			})
			if err != nil {
				return jobs, err
			}
			jobs = append(jobs, job)
		}
	}
	_, _ = s.db.Exec(ctx, `update backhaul_links set status='pending_apply', updated_at=now() where id=$1`, link.ID)
	for _, transport := range link.Transports {
		_, _ = s.db.Exec(ctx, `update backhaul_transports set status='pending_apply', last_error='', updated_at=now() where id=$1`, transport.ID)
	}
	_, _ = s.CreateAudit(ctx, "system", "backhaul.apply", "backhaul", &link.ID, "managed backhaul profile apply queued")
	return jobs, nil
}

func (s *Store) ApplyBackhaulJobResult(ctx context.Context, job domain.Job, status string, result map[string]any) {
	linkID, transportID, role := backhaulJobIdentity(job, result)
	if linkID == "" || transportID == "" {
		return
	}
	if status != "succeeded" {
		health, lastError := normalizeBackhaulApplyHealth(status, result, time.Now().UTC())
		errText := firstNonEmptyBackhaulString(lastError, "backhaul apply failed")
		if role != "" {
			_, _ = s.db.Exec(ctx, `update backhaul_transports
				set status='failed',
				    last_error=$2,
				    health_json=jsonb_set(coalesce(health_json,'{}'::jsonb), $3::text[], $4::jsonb, true),
				    updated_at=now()
				where id=$1`, transportID, errText, []string{role}, mustJSON(health))
		} else {
			_, _ = s.db.Exec(ctx, `update backhaul_transports set status='failed', last_error=$2, updated_at=now() where id=$1`, transportID, errText)
		}
		s.refreshBackhaulLinkApplyStatus(ctx, linkID)
		return
	}
	switch role {
	case "ingress":
		_, _ = s.db.Exec(ctx, `update backhaul_transports set applied_ingress_at=now(), last_error='', updated_at=now() where id=$1`, transportID)
	case "egress":
		_, _ = s.db.Exec(ctx, `update backhaul_transports set applied_egress_at=now(), last_error='', updated_at=now() where id=$1`, transportID)
	default:
		return
	}
	if health, ok := result["health"].(map[string]any); ok && health != nil {
		_, _ = s.db.Exec(ctx, `update backhaul_transports
			set health_json=jsonb_set(coalesce(health_json,'{}'::jsonb), $2::text[], $3::jsonb, true), updated_at=now()
			where id=$1`, transportID, []string{role}, mustJSON(health))
	}
	var activationMode, driver string
	var ingressApplied, egressApplied bool
	if err := s.db.QueryRow(ctx, `select
		coalesce(config_json->>'activation_mode',''),
		driver,
		applied_ingress_at is not null,
		applied_egress_at is not null
	from backhaul_transports
	where id=$1`, transportID).Scan(&activationMode, &driver, &ingressApplied, &egressApplied); err == nil && ingressApplied && egressApplied {
		_, _ = s.db.Exec(ctx, `update backhaul_transports
			set status=$2, updated_at=now()
			where id=$1`, transportID, backhaulApplyCompleteStatus(activationMode, driver))
	}
	s.refreshBackhaulLinkApplyStatus(ctx, linkID)
}

func normalizeBackhaulApplyHealth(jobStatus string, result map[string]any, checkedAt time.Time) (map[string]any, string) {
	if result == nil {
		result = map[string]any{}
	}
	health, _ := result["health"].(map[string]any)
	if health == nil {
		health = map[string]any{}
	}
	out := make(map[string]any, len(health)+8)
	for key, value := range health {
		out[key] = value
	}
	if stringify(out["status"]) == "" {
		out["status"] = "failed"
	}
	if reason := firstNonEmptyBackhaulString(stringify(out["reason"]), stringify(result["health_reason"])); reason != "" {
		out["reason"] = reason
	}
	if errText := firstNonEmptyBackhaulString(stringify(out["reason"]), stringify(out["error"]), stringify(result["health_reason"]), stringify(result["health_error"]), stringify(result["error"]), stringify(result["message"])); errText != "" {
		out["error"] = errText
	}
	if stage := strings.TrimSpace(stringify(result["stage"])); stage != "" {
		out["stage"] = stage
	}
	out["job_status"] = jobStatus
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	out["checked_at"] = checkedAt.UTC().Format(time.RFC3339)
	lastError := firstNonEmptyBackhaulString(stringify(out["reason"]), stringify(out["error"]), "backhaul apply failed")
	return out, lastError
}

func (s *Store) refreshBackhaulLinkApplyStatus(ctx context.Context, linkID string) {
	_, _ = s.db.Exec(ctx, `update backhaul_links bl
		set status=case
				when st.status='active' then 'active'
				when st.status='materialized' then 'materialized'
				when st.status='failed' then 'failed'
				when st.status in ('pending_apply','planned') then 'pending_apply'
				else bl.status
			end,
			updated_at=now()
		from backhaul_transports st
		where bl.id=$1
		  and bl.selected_transport_id=st.id
		  and bl.status <> 'deleted'`, linkID)
}

func (s *Store) ApplyBackhaulProbeResult(ctx context.Context, job domain.Job, status string, result map[string]any) {
	_, transportID, role := backhaulJobIdentity(job, result)
	if transportID == "" || role == "" {
		return
	}
	health, lastError := normalizeBackhaulProbeHealth(status, result, time.Now().UTC())
	_, _ = s.db.Exec(ctx, `update backhaul_transports
		set health_json=jsonb_set(coalesce(health_json,'{}'::jsonb), $2::text[], $3::jsonb, true),
		    last_error=$4,
		    updated_at=now()
		where id=$1`, transportID, []string{role}, mustJSON(health), lastError)
}

func normalizeBackhaulProbeHealth(jobStatus string, result map[string]any, checkedAt time.Time) (map[string]any, string) {
	if result == nil {
		result = map[string]any{}
	}
	health, _ := result["health"].(map[string]any)
	if health == nil {
		health = map[string]any{}
	}
	out := make(map[string]any, len(health)+8)
	for key, value := range health {
		out[key] = value
	}
	if stringify(out["status"]) == "" {
		out["status"] = firstNonEmptyBackhaulString(stringify(result["health_status"]), jobStatus)
	}
	if reason := firstNonEmptyBackhaulString(stringify(out["reason"]), stringify(result["health_reason"])); reason != "" {
		out["reason"] = reason
	}
	if errText := firstNonEmptyBackhaulString(stringify(out["error"]), stringify(result["health_error"]), stringify(result["error"])); errText != "" {
		out["error"] = errText
	}
	if jobStatus != "succeeded" {
		out["job_status"] = jobStatus
		if stringify(out["status"]) == "" || stringify(out["status"]) == "succeeded" {
			out["status"] = "failed"
		}
		if stringify(out["error"]) == "" {
			out["error"] = firstNonEmptyBackhaulString(stringify(out["reason"]), "backhaul probe failed")
		}
	}
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	out["checked_at"] = checkedAt.UTC().Format(time.RFC3339)
	lastError := ""
	if jobStatus != "succeeded" || stringify(out["status"]) == "degraded" || stringify(out["status"]) == "unhealthy" || stringify(out["status"]) == "failed" {
		lastError = firstNonEmptyBackhaulString(stringify(out["error"]), stringify(out["reason"]))
	}
	return out, lastError
}

func firstNonEmptyBackhaulString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Store) ApplyBackhaulCleanupResult(ctx context.Context, job domain.Job, status string, result map[string]any) {
	linkID, transportID, role := backhaulJobIdentity(job, result)
	if linkID == "" || transportID == "" || role == "" {
		return
	}
	cleanup := map[string]any{
		"status":      status,
		"finished_at": time.Now().UTC().Format(time.RFC3339),
	}
	if removed, ok := result["removed_paths"]; ok {
		cleanup["removed_paths"] = removed
	}
	if units, ok := result["stopped_units"]; ok {
		cleanup["stopped_units"] = units
	}
	if skipped, ok := result["skipped_items"]; ok {
		cleanup["skipped_items"] = skipped
	}
	if status != "succeeded" {
		errText := firstNonEmptyRouteValue(stringify(result["error"]), "backhaul cleanup failed")
		cleanup["error"] = errText
		_, _ = s.db.Exec(ctx, `update backhaul_transports set status='failed', last_error=$2, updated_at=now() where id=$1`, transportID, errText)
		_, _ = s.db.Exec(ctx, `update backhaul_links set status='failed', updated_at=now() where id=$1`, linkID)
	} else {
		_, _ = s.db.Exec(ctx, `update backhaul_transports set status='disabled', last_error='', updated_at=now() where id=$1`, transportID)
	}
	_, _ = s.db.Exec(ctx, `update backhaul_transports
		set health_json=jsonb_set(coalesce(health_json,'{}'::jsonb), $2::text[], $3::jsonb, true), updated_at=now()
		where id=$1`, transportID, []string{"cleanup", role}, mustJSON(cleanup))

	batchID := firstNonEmptyRouteValue(stringify(result["delete_batch_id"]), stringify(job.Payload["delete_batch_id"]))
	if batchID == "" {
		return
	}
	var pending, failed int
	_ = s.db.QueryRow(ctx, `select
		count(*) filter (where status in ('queued','running','retrying'))::int,
		count(*) filter (where status in ('failed','cancelled'))::int
	from jobs
	where type='node.backhaul.cleanup'
	  and scope_id=$1
	  and payload_json->>'delete_batch_id'=$2`, linkID, batchID).Scan(&pending, &failed)
	if pending == 0 && failed == 0 {
		_, _ = s.db.Exec(ctx, `update backhaul_links set status='deleted', updated_at=now() where id=$1`, linkID)
	}
}

func backhaulJobIdentity(job domain.Job, result map[string]any) (string, string, string) {
	linkID := strings.TrimSpace(stringify(result["link_id"]))
	if linkID == "" {
		linkID = strings.TrimSpace(stringify(job.Payload["link_id"]))
	}
	transportID := strings.TrimSpace(stringify(result["transport_id"]))
	if transportID == "" {
		transportID = strings.TrimSpace(stringify(job.Payload["transport_id"]))
	}
	role := strings.ToLower(strings.TrimSpace(stringify(result["role"])))
	if role == "" {
		role = strings.ToLower(strings.TrimSpace(stringify(job.Payload["role"])))
	}
	return linkID, transportID, role
}

func (s *Store) buildBackhaulApplyPayload(ctx context.Context, link domain.BackhaulLink, transport domain.BackhaulTransport, role string) (map[string]any, error) {
	def, ok := backhaul.Definition(transport.Driver)
	if !ok {
		return nil, fmt.Errorf("unsupported backhaul driver %q", transport.Driver)
	}
	side, err := s.backhaulSideMaterial(ctx, link, transport, role, def)
	if err != nil {
		return nil, err
	}
	files, _ := side["files"].([]map[string]any)
	payloadFiles := make([]any, 0, len(files))
	for _, file := range files {
		payloadFiles = append(payloadFiles, file)
	}
	activate := def.ActivationMode == "managed_systemd"
	return map[string]any{
		"node_id":                nodeIDForBackhaulRole(link, role),
		"link_id":                link.ID,
		"transport_id":           transport.ID,
		"role":                   role,
		"driver":                 transport.Driver,
		"driver_layer":           def.Layer,
		"activation_mode":        def.ActivationMode,
		"activate":               activate,
		"supports_kernel_routes": def.SupportsKernelRoutes,
		"link_name":              link.Name,
		"interface_name":         transport.InterfaceName,
		"routing_table":          link.RoutingTable,
		"route_metric":           link.RouteMetric,
		"endpoint_host":          transport.EndpointHost,
		"endpoint_port":          transport.EndpointPort,
		"protocol":               transport.Protocol,
		"tunnel_cidr":            transport.TunnelCIDR,
		"ingress_address":        transport.IngressAddress,
		"egress_address":         transport.EgressAddress,
		"output_path":            backhaulManifestPath(link, transport, role),
		"systemd_unit":           stringify(side["systemd_unit"]),
		"files":                  payloadFiles,
		"warnings":               def.Warnings,
		"enforcement": map[string]any{
			"kernel_route_enforcement_enabled": false,
			"reason":                           "backhaul transport materialization is separate from client route kernel enforcement",
		},
	}, nil
}

func buildBackhaulProbePayload(link domain.BackhaulLink, transport domain.BackhaulTransport, role string) map[string]any {
	peerAddress := transport.EgressAddress
	if role == "egress" {
		peerAddress = transport.IngressAddress
	}
	return map[string]any{
		"node_id":        nodeIDForBackhaulRole(link, role),
		"link_id":        link.ID,
		"transport_id":   transport.ID,
		"role":           role,
		"driver":         transport.Driver,
		"interface_name": transport.InterfaceName,
		"systemd_unit":   backhaulSystemdUnit(link, transport, role),
		"peer_address":   peerAddress,
		"probe_count":    3,
	}
}

func buildBackhaulCleanupPayload(link domain.BackhaulLink, transport domain.BackhaulTransport, role, batchID string) map[string]any {
	return map[string]any{
		"node_id":         nodeIDForBackhaulRole(link, role),
		"link_id":         link.ID,
		"transport_id":    transport.ID,
		"role":            role,
		"driver":          transport.Driver,
		"interface_name":  transport.InterfaceName,
		"delete_batch_id": batchID,
		"systemd_units":   backhaulSystemdUnits(link, transport, role),
		"paths":           backhaulCleanupPaths(link, transport, role),
		"directories":     []string{"/etc/megavpn/backhaul/" + safeBackhaulSlug(link, transport)},
	}
}

func (s *Store) backhaulSideMaterial(ctx context.Context, link domain.BackhaulLink, transport domain.BackhaulTransport, role string, def backhaul.DriverDefinition) (map[string]any, error) {
	switch transport.Driver {
	case backhaul.DriverWireGuard:
		return s.wireGuardBackhaulMaterial(ctx, link, transport, role)
	case backhaul.DriverOpenVPNUDP, backhaul.DriverOpenVPNTCP443:
		return s.openVPNBackhaulMaterial(ctx, link, transport, role)
	default:
		return s.materializeOnlyBackhaulMaterial(ctx, link, transport, role, def)
	}
}

func (s *Store) wireGuardBackhaulMaterial(ctx context.Context, link domain.BackhaulLink, transport domain.BackhaulTransport, role string) (map[string]any, error) {
	ingressPriv, err := s.resolveSecretString(ctx, transport.SecretRefs["ingress_private_key_secret_ref_id"])
	if err != nil {
		return nil, err
	}
	egressPriv, err := s.resolveSecretString(ctx, transport.SecretRefs["egress_private_key_secret_ref_id"])
	if err != nil {
		return nil, err
	}
	ingressPub := stringify(transport.Config["ingress_public_key"])
	egressPub := stringify(transport.Config["egress_public_key"])
	slug := safeBackhaulSlug(link, transport)
	configPath := "/etc/megavpn/backhaul/" + slug + "/" + transport.InterfaceName + ".conf"
	unitName := backhaulSystemdUnit(link, transport, role)
	unitPath := "/etc/systemd/system/" + unitName
	var config string
	if role == "egress" {
		config = renderWireGuardBackhaulConfig(transport, role, egressPriv, ingressPub)
	} else {
		config = renderWireGuardBackhaulConfig(transport, role, ingressPriv, egressPub)
	}
	files := []map[string]any{
		{"path": configPath, "mode": "0600", "content": config},
	}
	unitStart := "/usr/bin/wg-quick up " + shellQuotePG(configPath)
	unitStop := "/usr/bin/wg-quick down " + shellQuotePG(configPath)
	startPath := "/etc/megavpn/backhaul/" + slug + "/" + role + "-start.sh"
	stopPath := "/etc/megavpn/backhaul/" + slug + "/" + role + "-stop.sh"
	if role == "egress" {
		files = append(files,
			map[string]any{"path": startPath, "mode": "0700", "content": renderBackhaulEgressStartScript(slug, transport.InterfaceName, unitStart)},
			map[string]any{"path": stopPath, "mode": "0700", "content": renderBackhaulEgressStopScript(slug, transport.InterfaceName, unitStop)},
		)
	} else {
		files = append(files,
			map[string]any{"path": startPath, "mode": "0700", "content": renderBackhaulIngressStartScript(slug, transport.InterfaceName, unitStart)},
			map[string]any{"path": stopPath, "mode": "0700", "content": renderBackhaulIngressStopScript(slug, transport.InterfaceName, unitStop)},
		)
	}
	unitStart = startPath
	unitStop = stopPath
	files = append(files, map[string]any{"path": unitPath, "mode": "0644", "content": renderBackhaulUnit(unitName, unitStart, unitStop)})
	return map[string]any{
		"systemd_unit": unitName,
		"files":        files,
	}, nil
}

func renderWireGuardBackhaulConfig(transport domain.BackhaulTransport, role, privateKey, peerPublicKey string) string {
	localAddress := backhaulAddressWithPrefix(transport.IngressAddress, transport.TunnelCIDR)
	peerAddress := hostAddress(transport.EgressAddress)
	if role == "egress" {
		localAddress = backhaulAddressWithPrefix(transport.EgressAddress, transport.TunnelCIDR)
		peerAddress = hostAddress(transport.IngressAddress)
	}
	lines := []string{
		"[Interface]",
		"PrivateKey = " + strings.TrimSpace(privateKey),
		"Address = " + localAddress,
	}
	if role == "egress" {
		lines = append(lines, "ListenPort = "+strconv.Itoa(transport.EndpointPort))
	}
	lines = append(lines,
		"Table = off",
		"",
		"[Peer]",
		"PublicKey = "+strings.TrimSpace(peerPublicKey),
	)
	if role != "egress" {
		lines = append(lines, "Endpoint = "+transport.EndpointHost+":"+strconv.Itoa(transport.EndpointPort))
	}
	lines = append(lines,
		"# Peer tunnel address: "+peerAddress,
		"AllowedIPs = 0.0.0.0/0, ::/0",
		"PersistentKeepalive = 25",
		"",
	)
	return strings.Join(lines, "\n")
}

func (s *Store) openVPNBackhaulMaterial(ctx context.Context, link domain.BackhaulLink, transport domain.BackhaulTransport, role string) (map[string]any, error) {
	staticKey, err := s.resolveSecretString(ctx, transport.SecretRefs["static_key_secret_ref_id"])
	if err != nil {
		return nil, err
	}
	slug := safeBackhaulSlug(link, transport)
	configPath := "/etc/megavpn/backhaul/" + slug + "/openvpn.conf"
	keyPath := "/etc/megavpn/backhaul/" + slug + "/openvpn.static.key"
	profilePath := "/etc/megavpn/backhaul/" + slug + "/openvpn-profile-" + role + ".json"
	unitName := backhaulSystemdUnit(link, transport, role)
	unitPath := "/etc/systemd/system/" + unitName
	pidPath := "/run/" + unitName + ".pid"
	proto := "udp"
	if transport.Driver == backhaul.DriverOpenVPNTCP443 {
		if role == "egress" {
			proto = "tcp-server"
		} else {
			proto = "tcp-client"
		}
	}
	lines := []string{
		"dev " + transport.InterfaceName,
		"dev-type tun",
		"proto " + proto,
		"port " + strconv.Itoa(transport.EndpointPort),
		"topology p2p",
		"cipher AES-256-CBC",
		"auth SHA256",
		"auth-nocache",
		"replay-window 64 15",
		"mute-replay-warnings",
		"tun-mtu 1500",
		"mssfix 1360",
	}
	if role == "ingress" {
		lines = append(lines, "remote "+transport.EndpointHost+" "+strconv.Itoa(transport.EndpointPort))
		lines = append(lines, "resolv-retry infinite")
		lines = append(lines, "nobind")
		lines = append(lines, "ifconfig "+hostAddress(transport.IngressAddress)+" "+hostAddress(transport.EgressAddress))
	} else {
		lines = append(lines, "local 0.0.0.0")
		lines = append(lines, "ifconfig "+hostAddress(transport.EgressAddress)+" "+hostAddress(transport.IngressAddress))
	}
	lines = append(lines,
		"secret "+keyPath,
		"persist-key",
		"persist-tun",
		"persist-local-ip",
		"persist-remote-ip",
		"keepalive 10 60",
		"ping-timer-rem",
		"verb 3",
		"",
	)
	if transport.Driver == backhaul.DriverOpenVPNUDP && role == "ingress" {
		lines = append(lines[:len(lines)-1], "explicit-exit-notify 1", "")
	}
	profile := backhaulOperatorProfile(link, transport, role, backhaul.DriverDefinition{
		Code:                 transport.Driver,
		Label:                "OpenVPN P2P",
		Layer:                "l3",
		ActivationMode:       "managed_systemd",
		SupportsKernelRoutes: true,
		Capabilities:         []string{"openvpn"},
	}, map[string]any{
		"config_path":  configPath,
		"key_path":     keyPath,
		"systemd_unit": unitName,
		"profile": map[string]any{
			"mode":          "static_key_p2p",
			"cipher":        "AES-256-CBC",
			"auth":          "SHA256",
			"tun_mtu":       1500,
			"mssfix":        1360,
			"keepalive":     "10 60",
			"replay_window": "64 15",
		},
	})
	profileJSON, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return nil, err
	}
	files := []map[string]any{
		{"path": configPath, "mode": "0600", "content": strings.Join(lines, "\n")},
		{"path": keyPath, "mode": "0600", "content": staticKey},
		{"path": profilePath, "mode": "0640", "content": string(profileJSON) + "\n"},
	}
	unitStart := "/usr/sbin/openvpn --daemon " + shellQuotePG(unitName) + " --writepid " + shellQuotePG(pidPath) + " --config " + shellQuotePG(configPath)
	unitStop := "test ! -f " + shellQuotePG(pidPath) + " || kill $(cat " + shellQuotePG(pidPath) + ")"
	startPath := "/etc/megavpn/backhaul/" + slug + "/" + role + "-start.sh"
	stopPath := "/etc/megavpn/backhaul/" + slug + "/" + role + "-stop.sh"
	if role == "egress" {
		files = append(files,
			map[string]any{"path": startPath, "mode": "0700", "content": renderBackhaulEgressStartScript(slug, transport.InterfaceName, unitStart)},
			map[string]any{"path": stopPath, "mode": "0700", "content": renderBackhaulEgressStopScript(slug, transport.InterfaceName, unitStop)},
		)
	} else {
		files = append(files,
			map[string]any{"path": startPath, "mode": "0700", "content": renderBackhaulIngressStartScript(slug, transport.InterfaceName, unitStart)},
			map[string]any{"path": stopPath, "mode": "0700", "content": renderBackhaulIngressStopScript(slug, transport.InterfaceName, unitStop)},
		)
	}
	unitStart = startPath
	unitStop = stopPath
	files = append(files, map[string]any{"path": unitPath, "mode": "0644", "content": renderBackhaulUnit(unitName, unitStart, unitStop)})
	return map[string]any{
		"systemd_unit": unitName,
		"files":        files,
	}, nil
}

func (s *Store) materializeOnlyBackhaulMaterial(ctx context.Context, link domain.BackhaulLink, transport domain.BackhaulTransport, role string, def backhaul.DriverDefinition) (map[string]any, error) {
	switch transport.Driver {
	case backhaul.DriverIPSecL2TP, backhaul.DriverIKEv2:
		return s.ipsecBackhaulMaterial(ctx, link, transport, role, def)
	case backhaul.DriverXrayVLESSWS, backhaul.DriverXrayVLESSGRPC, backhaul.DriverXrayReality, backhaul.DriverXrayTUNVLESS:
		return s.xrayBackhaulMaterial(ctx, link, transport, role, def)
	}
	slug := safeBackhaulSlug(link, transport)
	config := map[string]any{
		"link_id":          link.ID,
		"link_name":        link.Name,
		"driver":           transport.Driver,
		"role":             role,
		"endpoint_host":    transport.EndpointHost,
		"endpoint_port":    transport.EndpointPort,
		"protocol":         transport.Protocol,
		"interface_name":   transport.InterfaceName,
		"tunnel_cidr":      transport.TunnelCIDR,
		"ingress_address":  transport.IngressAddress,
		"egress_address":   transport.EgressAddress,
		"activation_mode":  def.ActivationMode,
		"materialized_at":  time.Now().UTC().Format(time.RFC3339),
		"operator_action":  "review driver config and enable transport-specific activation stage",
		"driver_warnings":  def.Warnings,
		"driver_config":    transport.Config,
		"secret_ref_names": sortedBackhaulSecretRefNames(transport.SecretRefs),
	}
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"systemd_unit": "",
		"files": []map[string]any{
			{"path": "/etc/megavpn/backhaul/" + slug + "/" + transport.Driver + "-" + role + ".json", "mode": "0600", "content": string(b) + "\n"},
		},
	}, nil
}

func (s *Store) ipsecBackhaulMaterial(ctx context.Context, link domain.BackhaulLink, transport domain.BackhaulTransport, role string, def backhaul.DriverDefinition) (map[string]any, error) {
	psk, err := s.resolveSecretString(ctx, transport.SecretRefs["psk_secret_ref_id"])
	if err != nil {
		return nil, err
	}
	slug := safeBackhaulSlug(link, transport)
	configPath := "/etc/megavpn/backhaul/" + slug + "/ipsec.conf"
	secretsPath := "/etc/megavpn/backhaul/" + slug + "/ipsec.secrets"
	profilePath := "/etc/megavpn/backhaul/" + slug + "/ipsec-profile.json"
	config, secrets := buildBackhaulIPSecConfig(link, transport, role, def, psk)
	profile := backhaulOperatorProfile(link, transport, role, def, map[string]any{
		"config_path":  configPath,
		"secrets_path": secretsPath,
		"activation":   "manual ipsec/strongSwan import required",
	})
	profileJSON, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"systemd_unit": "",
		"files": []map[string]any{
			{"path": configPath, "mode": "0640", "content": config},
			{"path": secretsPath, "mode": "0600", "content": secrets},
			{"path": profilePath, "mode": "0640", "content": string(profileJSON) + "\n"},
		},
	}, nil
}

func (s *Store) xrayBackhaulMaterial(ctx context.Context, link domain.BackhaulLink, transport domain.BackhaulTransport, role string, def backhaul.DriverDefinition) (map[string]any, error) {
	uuid, err := s.resolveSecretString(ctx, transport.SecretRefs["uuid_secret_ref_id"])
	if err != nil {
		return nil, err
	}
	realityPrivateKey := ""
	if transport.Driver == backhaul.DriverXrayReality {
		realityPrivateKey, err = s.resolveSecretString(ctx, transport.SecretRefs["reality_private_key_secret_ref_id"])
		if err != nil {
			return nil, err
		}
	}
	slug := safeBackhaulSlug(link, transport)
	configPath := "/etc/megavpn/backhaul/" + slug + "/xray-" + role + ".json"
	profilePath := "/etc/megavpn/backhaul/" + slug + "/xray-profile-" + role + ".json"
	unitName := backhaulSystemdUnit(link, transport, role)
	unitPath := "/etc/systemd/system/" + unitName
	config := buildBackhaulXrayConfig(link, transport, role, def, uuid, realityPrivateKey)
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	profile := backhaulOperatorProfile(link, transport, role, def, map[string]any{
		"config_path":  configPath,
		"systemd_unit": unitName,
		"activation":   "manual Xray/Nginx routing review required",
	})
	profileJSON, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"systemd_unit": "",
		"files": []map[string]any{
			{"path": configPath, "mode": "0640", "content": string(configJSON) + "\n"},
			{"path": profilePath, "mode": "0640", "content": string(profileJSON) + "\n"},
			{"path": unitPath, "mode": "0644", "content": renderBackhaulUnit(unitName, "/usr/bin/env xray run -config "+configPath, "")},
		},
	}, nil
}

func (s *Store) buildBackhaulTransport(ctx context.Context, link domain.BackhaulLink, def backhaul.DriverDefinition, endpointHost, tunnelCIDR string, idx int) (domain.BackhaulTransport, error) {
	ingressAddr, egressAddr, err := pointToPointAddresses(tunnelCIDR)
	if err != nil {
		return domain.BackhaulTransport{}, err
	}
	secretRefs := map[string]any{}
	config := map[string]any{
		"activation_mode":        def.ActivationMode,
		"supports_kernel_routes": def.SupportsKernelRoutes,
	}
	switch def.Code {
	case backhaul.DriverWireGuard:
		ingressPriv, ingressPub, err := generateX25519KeyPair()
		if err != nil {
			return domain.BackhaulTransport{}, err
		}
		egressPriv, egressPub, err := generateX25519KeyPair()
		if err != nil {
			return domain.BackhaulTransport{}, err
		}
		ingressRef, err := s.CreateSecretRef(ctx, "private_key", []byte(ingressPriv), map[string]any{"scope": "backhaul", "link_id": link.ID, "driver": def.Code, "role": "ingress"})
		if err != nil {
			return domain.BackhaulTransport{}, err
		}
		egressRef, err := s.CreateSecretRef(ctx, "private_key", []byte(egressPriv), map[string]any{"scope": "backhaul", "link_id": link.ID, "driver": def.Code, "role": "egress"})
		if err != nil {
			return domain.BackhaulTransport{}, err
		}
		secretRefs["ingress_private_key_secret_ref_id"] = ingressRef.ID
		secretRefs["egress_private_key_secret_ref_id"] = egressRef.ID
		config["ingress_public_key"] = ingressPub
		config["egress_public_key"] = egressPub
	case backhaul.DriverOpenVPNUDP, backhaul.DriverOpenVPNTCP443:
		key, err := generateOpenVPNStaticKey()
		if err != nil {
			return domain.BackhaulTransport{}, err
		}
		ref, err := s.CreateSecretRef(ctx, "psk", []byte(key), map[string]any{"scope": "backhaul", "link_id": link.ID, "driver": def.Code, "material": "openvpn_static_key"})
		if err != nil {
			return domain.BackhaulTransport{}, err
		}
		secretRefs["static_key_secret_ref_id"] = ref.ID
	case backhaul.DriverIPSecL2TP, backhaul.DriverIKEv2:
		psk, err := randomBase64(32)
		if err != nil {
			return domain.BackhaulTransport{}, err
		}
		ref, err := s.CreateSecretRef(ctx, "psk", []byte(psk), map[string]any{"scope": "backhaul", "link_id": link.ID, "driver": def.Code, "material": "ipsec_psk"})
		if err != nil {
			return domain.BackhaulTransport{}, err
		}
		secretRefs["psk_secret_ref_id"] = ref.ID
	default:
		uuid := id.New()
		ref, err := s.CreateSecretRef(ctx, "uuid", []byte(uuid), map[string]any{"scope": "backhaul", "link_id": link.ID, "driver": def.Code, "material": "xray_uuid"})
		if err != nil {
			return domain.BackhaulTransport{}, err
		}
		secretRefs["uuid_secret_ref_id"] = ref.ID
		if def.Code == backhaul.DriverXrayReality {
			privateKey, publicKey, err := generateXrayRealityKeyPair()
			if err != nil {
				return domain.BackhaulTransport{}, err
			}
			privateRef, err := s.CreateSecretRef(ctx, "private_key", []byte(privateKey), map[string]any{"scope": "backhaul", "link_id": link.ID, "driver": def.Code, "material": "xray_reality_private_key"})
			if err != nil {
				return domain.BackhaulTransport{}, err
			}
			shortIDRaw, err := randomBytes(8)
			if err != nil {
				return domain.BackhaulTransport{}, err
			}
			secretRefs["reality_private_key_secret_ref_id"] = privateRef.ID
			config["reality_public_key"] = publicKey
			config["reality_short_id"] = hex.EncodeToString(shortIDRaw)
		}
		config["xray_network"] = xrayBackhaulNetwork(def.Code)
		config["xray_security"] = xrayBackhaulSecurity(def.Code)
		config["path"] = "/assets/" + shortID(link.ID) + "-sync"
	}
	return domain.BackhaulTransport{
		ID:             id.New(),
		LinkID:         link.ID,
		Driver:         def.Code,
		Priority:       (idx + 1) * 10,
		Status:         "planned",
		EndpointHost:   endpointHost,
		EndpointPort:   def.DefaultPort,
		Protocol:       def.DefaultProtocol,
		InterfaceName:  defaultBackhaulInterface(link.ID, def.Code),
		TunnelCIDR:     tunnelCIDR,
		IngressAddress: ingressAddr,
		EgressAddress:  egressAddr,
		Config:         config,
		SecretRefs:     secretRefs,
		Health:         map[string]any{"status": "unknown"},
	}, nil
}

func (s *Store) listBackhaulTransports(ctx context.Context, linkID string) ([]domain.BackhaulTransport, error) {
	rows, err := s.db.Query(ctx, `select
		id::text,
		link_id::text,
		driver,
		priority,
		status,
		endpoint_host,
		endpoint_port,
		protocol,
		interface_name,
		tunnel_cidr,
		ingress_address,
		egress_address,
		config_json,
		secret_refs_json,
		health_json,
		applied_ingress_at,
		applied_egress_at,
		last_error,
		created_at,
		updated_at
	from backhaul_transports
	where link_id=$1
	order by priority asc, driver asc`, strings.TrimSpace(linkID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.BackhaulTransport{}
	for rows.Next() {
		transport, err := scanBackhaulTransport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, transport)
	}
	return out, rows.Err()
}

func scanBackhaulLink(row pgx.Row) (domain.BackhaulLink, error) {
	var link domain.BackhaulLink
	var selectedTransportID *string
	var failoverRaw, metadataRaw []byte
	if err := row.Scan(
		&link.ID,
		&link.Name,
		&link.IngressNodeID,
		&link.EgressNodeID,
		&link.Status,
		&selectedTransportID,
		&link.DesiredDriver,
		&link.RoutingTable,
		&link.RouteMetric,
		&failoverRaw,
		&metadataRaw,
		&link.CreatedAt,
		&link.UpdatedAt,
	); err != nil {
		return domain.BackhaulLink{}, err
	}
	link.SelectedTransportID = selectedTransportID
	_ = json.Unmarshal(failoverRaw, &link.FailoverPolicy)
	_ = json.Unmarshal(metadataRaw, &link.Metadata)
	if link.FailoverPolicy == nil {
		link.FailoverPolicy = map[string]any{}
	}
	if link.Metadata == nil {
		link.Metadata = map[string]any{}
	}
	return link, nil
}

func scanBackhaulTransport(row pgx.Row) (domain.BackhaulTransport, error) {
	var transport domain.BackhaulTransport
	var configRaw, secretRefsRaw, healthRaw []byte
	if err := row.Scan(
		&transport.ID,
		&transport.LinkID,
		&transport.Driver,
		&transport.Priority,
		&transport.Status,
		&transport.EndpointHost,
		&transport.EndpointPort,
		&transport.Protocol,
		&transport.InterfaceName,
		&transport.TunnelCIDR,
		&transport.IngressAddress,
		&transport.EgressAddress,
		&configRaw,
		&secretRefsRaw,
		&healthRaw,
		&transport.AppliedIngressAt,
		&transport.AppliedEgressAt,
		&transport.LastError,
		&transport.CreatedAt,
		&transport.UpdatedAt,
	); err != nil {
		return domain.BackhaulTransport{}, err
	}
	_ = json.Unmarshal(configRaw, &transport.Config)
	_ = json.Unmarshal(secretRefsRaw, &transport.SecretRefs)
	_ = json.Unmarshal(healthRaw, &transport.Health)
	if transport.Config == nil {
		transport.Config = map[string]any{}
	}
	if transport.SecretRefs == nil {
		transport.SecretRefs = map[string]any{}
	}
	if transport.Health == nil {
		transport.Health = map[string]any{}
	}
	return transport, nil
}

func (s *Store) listRoutePolicyBackhauls(ctx context.Context, ingressNodeID string) (map[string]routePolicyBackhaul, error) {
	rows, err := s.db.Query(ctx, `select
		bl.egress_node_id::text,
		bl.id::text,
		bt.id::text,
		bt.driver,
		bt.interface_name,
		bt.ingress_address,
		bt.egress_address,
		bl.routing_table,
		bl.route_metric,
		bt.status
	from backhaul_links bl
	join backhaul_transports bt on bt.id=bl.selected_transport_id
	where bl.ingress_node_id=$1
	  and bl.status='active'
	  and bt.status='active'`, strings.TrimSpace(ingressNodeID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]routePolicyBackhaul{}
	for rows.Next() {
		var egressNodeID string
		var b routePolicyBackhaul
		if err := rows.Scan(&egressNodeID, &b.LinkID, &b.TransportID, &b.Driver, &b.InterfaceName, &b.IngressAddress, &b.EgressAddress, &b.RoutingTable, &b.RouteMetric, &b.Status); err != nil {
			return nil, err
		}
		if b.RoutingTable == "" || strings.EqualFold(b.RoutingTable, "main") || strings.EqualFold(b.RoutingTable, "auto") {
			b.RoutingTable = defaultBackhaulRoutingTable(b.LinkID)
		}
		out[egressNodeID] = b
	}
	return out, rows.Err()
}

func selectedBackhaulTransport(link domain.BackhaulLink) *domain.BackhaulTransport {
	if link.SelectedTransportID == nil {
		return nil
	}
	for idx := range link.Transports {
		if link.Transports[idx].ID == *link.SelectedTransportID {
			return &link.Transports[idx]
		}
	}
	return nil
}

func nodeIDForBackhaulRole(link domain.BackhaulLink, role string) string {
	if role == "egress" {
		return link.EgressNodeID
	}
	return link.IngressNodeID
}

func backhaulDriversFromMetadata(metadata map[string]any, desired string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(driver string) {
		driver = backhaul.NormalizeDriver(driver)
		if driver == "" || seen[driver] || backhaul.ValidateDriver(driver) != nil {
			return
		}
		seen[driver] = true
		out = append(out, driver)
	}
	add(desired)
	_, explicitDrivers := metadata["drivers"]
	switch raw := metadata["drivers"].(type) {
	case []string:
		for _, driver := range raw {
			add(driver)
		}
	case []any:
		for _, driver := range raw {
			add(stringify(driver))
		}
	}
	if !explicitDrivers && len(out) <= 1 {
		for _, driver := range []string{
			backhaul.DriverWireGuard,
			backhaul.DriverOpenVPNUDP,
			backhaul.DriverOpenVPNTCP443,
			backhaul.DriverIPSecL2TP,
			backhaul.DriverIKEv2,
			backhaul.DriverXrayVLESSWS,
			backhaul.DriverXrayVLESSGRPC,
			backhaul.DriverXrayReality,
			backhaul.DriverXrayTUNVLESS,
		} {
			add(driver)
		}
	}
	return out
}

func backhaulApplyCompleteStatus(activationMode, driver string) string {
	if strings.EqualFold(strings.TrimSpace(activationMode), "managed_systemd") {
		return "active"
	}
	if strings.TrimSpace(activationMode) == "" {
		if def, ok := backhaul.Definition(driver); ok && def.ActivationMode == "managed_systemd" {
			return "active"
		}
	}
	return "materialized"
}

func (s *Store) ensureBackhaulUniqueTransportCIDRs(ctx context.Context, link domain.BackhaulLink) (bool, error) {
	used := map[string]bool{}
	changed := false
	for idx, transport := range link.Transports {
		cidr := strings.TrimSpace(transport.TunnelCIDR)
		if cidr != "" && !used[cidr] {
			used[cidr] = true
			continue
		}
		if strings.EqualFold(transport.Status, "active") {
			continue
		}
		nextCIDR := uniqueBackhaulTunnelCIDR(link.ID, cidr, transport.Driver, idx, used)
		ingressAddress, egressAddress, err := pointToPointAddresses(nextCIDR)
		if err != nil {
			return changed, err
		}
		if _, err := s.db.Exec(ctx, `update backhaul_transports
			set tunnel_cidr=$2,
			    ingress_address=$3,
			    egress_address=$4,
			    health_json=jsonb_set(coalesce(health_json,'{}'::jsonb), '{cidr_normalized}', $5::jsonb, true),
			    updated_at=now()
			where id=$1`,
			transport.ID,
			nextCIDR,
			ingressAddress,
			egressAddress,
			mustJSON(map[string]any{"from": cidr, "to": nextCIDR, "reason": "duplicate transport tunnel CIDR"})); err != nil {
			return changed, err
		}
		used[nextCIDR] = true
		changed = true
	}
	return changed, nil
}

func uniqueBackhaulTunnelCIDR(linkID, requested, driver string, idx int, used map[string]bool) string {
	requested = strings.TrimSpace(requested)
	if idx == 0 && requested != "" && !used[requested] && backhaul.ValidateTunnelCIDR(requested) == nil {
		return requested
	}
	for salt := 0; salt < 64; salt++ {
		candidate := defaultBackhaulCIDR(fmt.Sprintf("%s:%s:%d:%d", linkID, driver, idx, salt))
		if !used[candidate] {
			return candidate
		}
	}
	return defaultBackhaulCIDR(fmt.Sprintf("%s:%s:%d:fallback", linkID, driver, idx))
}

func defaultBackhaulCIDR(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	third := int(sum[0])
	fourth := int(sum[1]) & 0xfc
	return fmt.Sprintf("10.240.%d.%d/30", third, fourth)
}

func defaultBackhaulRoutingTable(seed string) string {
	sum := sha256.Sum256([]byte("backhaul-route-table:" + seed))
	return strconv.Itoa(20000 + int(sum[0])<<8 + int(sum[1]))
}

func pointToPointAddresses(cidr string) (string, string, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return "", "", err
	}
	base := prefix.Masked().Addr()
	ingress := base.Next()
	egress := ingress.Next()
	return ingress.String(), egress.String(), nil
}

func defaultBackhaulInterface(linkID, driver string) string {
	sum := sha256.Sum256([]byte(linkID + ":" + driver))
	return "mgbh" + hex.EncodeToString(sum[:])[:10]
}

func safeBackhaulSlug(link domain.BackhaulLink, transport domain.BackhaulTransport) string {
	return slugify(link.Name + "-" + transport.Driver + "-" + shortID(transport.ID))
}

func backhaulSystemdUnit(link domain.BackhaulLink, transport domain.BackhaulTransport, role string) string {
	units := backhaulSystemdUnits(link, transport, role)
	if len(units) == 0 {
		return ""
	}
	return units[0]
}

func backhaulSystemdUnits(link domain.BackhaulLink, transport domain.BackhaulTransport, role string) []string {
	switch transport.Driver {
	case backhaul.DriverWireGuard, backhaul.DriverOpenVPNUDP, backhaul.DriverOpenVPNTCP443:
		current := backhaulRoleSystemdUnit(link, transport, role)
		legacy := legacyBackhaulSystemdUnit(link, transport)
		if legacy == "" || legacy == current {
			return []string{current}
		}
		return []string{current, legacy}
	case backhaul.DriverXrayVLESSWS, backhaul.DriverXrayVLESSGRPC, backhaul.DriverXrayReality, backhaul.DriverXrayTUNVLESS:
		return []string{backhaulRoleSystemdUnit(link, transport, role)}
	default:
		return nil
	}
}

func backhaulRoleSystemdUnit(link domain.BackhaulLink, transport domain.BackhaulTransport, role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role != "ingress" && role != "egress" {
		role = "node"
	}
	const prefix = "megavpn-backhaul-"
	const suffix = ".service"
	roleSuffix := "-" + role
	maxSlug := 128 - len(prefix) - len(roleSuffix) - len(suffix)
	return prefix + boundedBackhaulUnitSlug(safeBackhaulSlug(link, transport), maxSlug) + roleSuffix + suffix
}

func legacyBackhaulSystemdUnit(link domain.BackhaulLink, transport domain.BackhaulTransport) string {
	slug := safeBackhaulSlug(link, transport)
	if slug == "" {
		return ""
	}
	return "megavpn-backhaul-" + slug + ".service"
}

func boundedBackhaulUnitSlug(slug string, maxLen int) string {
	slug = slugify(slug)
	if slug == "" {
		slug = "backhaul"
	}
	if maxLen < 16 {
		maxLen = 16
	}
	if len(slug) <= maxLen {
		return slug
	}
	sum := sha256.Sum256([]byte(slug))
	digest := hex.EncodeToString(sum[:])[:8]
	prefixLen := maxLen - len(digest) - 1
	if prefixLen < 1 {
		return digest[:maxLen]
	}
	prefix := strings.Trim(slug[:prefixLen], "-")
	if prefix == "" {
		prefix = "backhaul"
	}
	return prefix + "-" + digest
}

func backhaulCleanupPaths(link domain.BackhaulLink, transport domain.BackhaulTransport, role string) []string {
	paths := []string{}
	for _, unit := range backhaulSystemdUnits(link, transport, role) {
		paths = append(paths, "/etc/systemd/system/"+unit)
	}
	return paths
}

func backhaulManifestPath(link domain.BackhaulLink, transport domain.BackhaulTransport, role string) string {
	return "/etc/megavpn/backhaul/" + safeBackhaulSlug(link, transport) + "/" + role + "-manifest.json"
}

func renderBackhaulUnit(unitName, startCmd, stopCmd string) string {
	lines := []string{
		"[Unit]",
		"Description=RTIS MegaVPN managed backhaul transport " + unitName,
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=oneshot",
		"RemainAfterExit=yes",
		"ExecStart=" + startCmd,
	}
	if strings.TrimSpace(stopCmd) != "" {
		lines = append(lines, "ExecStop="+stopCmd)
	}
	lines = append(lines,
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	)
	return strings.Join(lines, "\n")
}

func renderBackhaulEgressStartScript(slug, iface, transportStartCmd string) string {
	return renderBackhaulNATStartScript(slug, iface, transportStartCmd, "iifname", "egress-masquerade")
}

func renderBackhaulIngressStartScript(slug, iface, transportStartCmd string) string {
	return renderBackhaulNATStartScript(slug, iface, transportStartCmd, "oifname", "ingress-snat")
}

func renderBackhaulNATStartScript(slug, iface, transportStartCmd, direction, label string) string {
	comment := "megavpn:backhaul:" + slug + ":" + label
	return strings.Join([]string{
		"#!/usr/bin/env sh",
		"set -eu",
		"",
		"# Generated by megavpn-agent. Do not edit manually.",
		transportStartCmd,
		"sysctl -w net.ipv4.ip_forward=1 >/dev/null",
		"if command -v nft >/dev/null 2>&1; then",
		"  nft list table ip megavpn_backhaul >/dev/null 2>&1 || nft add table ip megavpn_backhaul",
		"  nft list chain ip megavpn_backhaul postrouting >/dev/null 2>&1 || nft add chain ip megavpn_backhaul postrouting '{ type nat hook postrouting priority srcnat; policy accept; }'",
		"  if ! nft list chain ip megavpn_backhaul postrouting | grep -Fq " + shellQuotePG(comment) + "; then",
		"    nft add rule ip megavpn_backhaul postrouting " + direction + " " + shellQuotePG(iface) + " masquerade comment " + shellQuotePG(nftStringLiteral(comment)),
		"  fi",
		"fi",
		"",
	}, "\n")
}

func renderBackhaulEgressStopScript(slug, iface, transportStopCmd string) string {
	return renderBackhaulStopScript(slug, iface, transportStopCmd, "egress-masquerade")
}

func renderBackhaulIngressStopScript(slug, iface, transportStopCmd string) string {
	return renderBackhaulStopScript(slug, iface, transportStopCmd, "ingress-snat")
}

func renderBackhaulStopScript(slug, iface, transportStopCmd string, labels ...string) string {
	if len(labels) == 0 {
		labels = []string{"egress-masquerade"}
	}
	lines := []string{
		"#!/usr/bin/env sh",
		"set -eu",
		"",
		"# Generated by megavpn-agent. Do not edit manually.",
		"if command -v nft >/dev/null 2>&1; then",
	}
	for _, label := range labels {
		comment := "megavpn:backhaul:" + slug + ":" + label
		lines = append(lines,
			"  handles=$(nft -a list chain ip megavpn_backhaul postrouting 2>/dev/null | awk -v c="+shellQuotePG(comment)+" 'index($0, c) > 0 {print $NF}')",
			"  for handle in $handles; do",
			"    nft delete rule ip megavpn_backhaul postrouting handle \"$handle\" >/dev/null 2>&1 || true",
			"  done",
		)
	}
	lines = append(lines, "fi")
	if strings.TrimSpace(transportStopCmd) != "" {
		lines = append(lines, transportStopCmd+" >/dev/null 2>&1 || true")
	}
	_ = iface
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func buildBackhaulIPSecConfig(link domain.BackhaulLink, transport domain.BackhaulTransport, role string, def backhaul.DriverDefinition, psk string) (string, string) {
	connName := "megavpn-backhaul-" + safeBackhaulSlug(link, transport)
	left := "%defaultroute"
	right := transport.EndpointHost
	leftSubnet := hostAddress(transport.IngressAddress) + "/32"
	rightSubnet := hostAddress(transport.EgressAddress) + "/32"
	auto := "start"
	if role == "egress" {
		left = "%any"
		right = "%any"
		leftSubnet = hostAddress(transport.EgressAddress) + "/32"
		rightSubnet = hostAddress(transport.IngressAddress) + "/32"
		auto = "add"
	}
	keyExchange := "ikev2"
	if def.Code == backhaul.DriverIPSecL2TP {
		keyExchange = "ikev1"
	}
	config := strings.Join([]string{
		"config setup",
		"  uniqueids=no",
		"",
		"conn " + connName,
		"  keyexchange=" + keyExchange,
		"  authby=psk",
		"  type=tunnel",
		"  left=" + left,
		"  leftsubnet=" + leftSubnet,
		"  right=" + right,
		"  rightsubnet=" + rightSubnet,
		"  ike=aes256-sha256-modp2048!",
		"  esp=aes256-sha256!",
		"  dpdaction=restart",
		"  dpddelay=30s",
		"  dpdtimeout=120s",
		"  auto=" + auto,
		"",
	}, "\n")
	secrets := ": PSK \"" + strings.ReplaceAll(psk, "\"", "\\\"") + "\"\n"
	return config, secrets
}

func buildBackhaulXrayConfig(link domain.BackhaulLink, transport domain.BackhaulTransport, role string, def backhaul.DriverDefinition, uuid, realityPrivateKey string) map[string]any {
	if role == "egress" {
		return buildBackhaulXrayServerConfig(link, transport, def, uuid, realityPrivateKey)
	}
	return buildBackhaulXrayClientConfig(link, transport, def, uuid)
}

func buildBackhaulXrayServerConfig(link domain.BackhaulLink, transport domain.BackhaulTransport, def backhaul.DriverDefinition, uuid, realityPrivateKey string) map[string]any {
	streamSettings := xrayBackhaulServerStreamSettings(link, transport, def, realityPrivateKey)
	return map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{
			map[string]any{
				"tag":      "megavpn-backhaul-in",
				"listen":   "0.0.0.0",
				"port":     transport.EndpointPort,
				"protocol": "vless",
				"settings": map[string]any{
					"clients": []any{
						map[string]any{"id": uuid, "email": "backhaul-" + shortID(link.ID)},
					},
					"decryption": "none",
				},
				"streamSettings": streamSettings,
			},
		},
		"outbounds": []any{
			map[string]any{"tag": "direct", "protocol": "freedom"},
			map[string]any{"tag": "block", "protocol": "blackhole"},
		},
	}
}

func buildBackhaulXrayClientConfig(link domain.BackhaulLink, transport domain.BackhaulTransport, def backhaul.DriverDefinition, uuid string) map[string]any {
	outbound := map[string]any{
		"tag":      "megavpn-backhaul-out",
		"protocol": "vless",
		"settings": map[string]any{
			"vnext": []any{
				map[string]any{
					"address": transport.EndpointHost,
					"port":    transport.EndpointPort,
					"users": []any{
						map[string]any{"id": uuid, "encryption": "none"},
					},
				},
			},
		},
		"streamSettings": xrayBackhaulClientStreamSettings(link, transport, def),
	}
	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{
			map[string]any{
				"tag":      "local-socks",
				"listen":   "127.0.0.1",
				"port":     10808,
				"protocol": "socks",
				"settings": map[string]any{"udp": true},
			},
		},
		"outbounds": []any{
			outbound,
			map[string]any{"tag": "direct", "protocol": "freedom"},
			map[string]any{"tag": "block", "protocol": "blackhole"},
		},
		"routing": map[string]any{
			"rules": []any{
				map[string]any{"type": "field", "inboundTag": []any{"local-socks"}, "outboundTag": "megavpn-backhaul-out"},
			},
		},
	}
	if def.Code == backhaul.DriverXrayTUNVLESS {
		cfg["operator_note"] = "Xray TUN requires explicit Linux policy routing and loop protection before activation."
	}
	return cfg
}

func xrayBackhaulServerStreamSettings(link domain.BackhaulLink, transport domain.BackhaulTransport, def backhaul.DriverDefinition, realityPrivateKey string) map[string]any {
	settings := map[string]any{
		"network":  xrayBackhaulNetwork(def.Code),
		"security": xrayBackhaulSecurity(def.Code),
	}
	applyXrayBackhaulTransportSettings(settings, link, transport, def, false, realityPrivateKey)
	return settings
}

func xrayBackhaulClientStreamSettings(link domain.BackhaulLink, transport domain.BackhaulTransport, def backhaul.DriverDefinition) map[string]any {
	settings := map[string]any{
		"network":  xrayBackhaulNetwork(def.Code),
		"security": xrayBackhaulSecurity(def.Code),
	}
	applyXrayBackhaulTransportSettings(settings, link, transport, def, true, "")
	return settings
}

func applyXrayBackhaulTransportSettings(settings map[string]any, link domain.BackhaulLink, transport domain.BackhaulTransport, def backhaul.DriverDefinition, client bool, realityPrivateKey string) {
	switch xrayBackhaulNetwork(def.Code) {
	case "grpc":
		settings["grpcSettings"] = map[string]any{"serviceName": "megavpn-backhaul-" + shortID(link.ID)}
	case "ws":
		settings["wsSettings"] = map[string]any{
			"path": stringify(transport.Config["path"]),
			"headers": map[string]any{
				"Host": transport.EndpointHost,
			},
		}
	}
	switch xrayBackhaulSecurity(def.Code) {
	case "reality":
		if client {
			settings["realitySettings"] = map[string]any{
				"serverName":  transport.EndpointHost,
				"fingerprint": "chrome",
				"publicKey":   stringify(transport.Config["reality_public_key"]),
				"shortId":     stringify(transport.Config["reality_short_id"]),
			}
		} else {
			settings["realitySettings"] = map[string]any{
				"show":        false,
				"dest":        "www.cloudflare.com:443",
				"xver":        0,
				"serverNames": []any{transport.EndpointHost},
				"privateKey":  realityPrivateKey,
				"shortIds":    []any{stringify(transport.Config["reality_short_id"])},
			}
		}
	case "tls":
		if client {
			settings["tlsSettings"] = map[string]any{"serverName": transport.EndpointHost}
		} else {
			settings["operator_note"] = "TLS termination certificate is required; use Nginx edge or add tlsSettings before activation."
		}
	}
}

func backhaulOperatorProfile(link domain.BackhaulLink, transport domain.BackhaulTransport, role string, def backhaul.DriverDefinition, extra map[string]any) map[string]any {
	profile := map[string]any{
		"link_id":                link.ID,
		"link_name":              link.Name,
		"transport_id":           transport.ID,
		"driver":                 transport.Driver,
		"role":                   role,
		"layer":                  def.Layer,
		"activation_mode":        def.ActivationMode,
		"supports_kernel_routes": def.SupportsKernelRoutes,
		"endpoint_host":          transport.EndpointHost,
		"endpoint_port":          transport.EndpointPort,
		"protocol":               transport.Protocol,
		"interface_name":         transport.InterfaceName,
		"tunnel_cidr":            transport.TunnelCIDR,
		"ingress_address":        transport.IngressAddress,
		"egress_address":         transport.EgressAddress,
		"driver_warnings":        def.Warnings,
		"secret_ref_names":       sortedBackhaulSecretRefNames(transport.SecretRefs),
		"generated_at":           time.Now().UTC().Format(time.RFC3339),
	}
	for key, value := range extra {
		profile[key] = value
	}
	return profile
}

func generateX25519KeyPair() (string, string, error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(key.Bytes()), base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}

func generateXrayRealityKeyPair() (string, string, error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(key.Bytes()), base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}

func generateOpenVPNStaticKey() (string, error) {
	raw, err := randomBytes(256)
	if err != nil {
		return "", err
	}
	hexed := hex.EncodeToString(raw)
	lines := []string{"-----BEGIN OpenVPN Static key V1-----"}
	for idx := 0; idx < len(hexed); idx += 32 {
		end := idx + 32
		if end > len(hexed) {
			end = len(hexed)
		}
		lines = append(lines, hexed[idx:end])
	}
	lines = append(lines, "-----END OpenVPN Static key V1-----", "")
	return strings.Join(lines, "\n"), nil
}

func randomBytes(size int) ([]byte, error) {
	out := make([]byte, size)
	_, err := rand.Read(out)
	return out, err
}

func randomBase64(size int) (string, error) {
	raw, err := randomBytes(size)
	if err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(raw), nil
}

func (s *Store) resolveSecretString(ctx context.Context, ref any) (string, error) {
	refID := strings.TrimSpace(stringify(ref))
	if refID == "" {
		return "", fmt.Errorf("secret ref is required")
	}
	_, value, err := s.ResolveSecretValue(ctx, refID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}

func hostAddress(address string) string {
	if addr, err := netip.ParsePrefix(strings.TrimSpace(address)); err == nil {
		return addr.Addr().String()
	}
	return strings.TrimSpace(address)
}

func backhaulAddressWithPrefix(address, cidr string) string {
	host := hostAddress(address)
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil || host == "" {
		return host
	}
	return host + "/" + strconv.Itoa(prefix.Bits())
}

func xrayBackhaulNetwork(driver string) string {
	switch driver {
	case backhaul.DriverXrayVLESSGRPC:
		return "grpc"
	case backhaul.DriverXrayVLESSWS, backhaul.DriverXrayTUNVLESS:
		return "ws"
	default:
		return "tcp"
	}
}

func xrayBackhaulSecurity(driver string) string {
	if driver == backhaul.DriverXrayReality {
		return "reality"
	}
	return "tls"
}

func sortedBackhaulSecretRefNames(refs map[string]any) []string {
	out := make([]string, 0, len(refs))
	for key := range refs {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func shortID(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func shellQuotePG(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func nftStringLiteral(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
