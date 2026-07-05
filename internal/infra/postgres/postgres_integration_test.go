package postgres

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rtis-emc2/megavpn/internal/backhaul"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
)

func TestPostgresIntegrationJobsLocksProvisioningAccessRoutes(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-node-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.0.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	instance, err := store.CreateInstance(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "wireguard",
		Name:         "wg-" + suffix,
		Slug:         "wg-" + suffix,
		EndpointHost: "198.51.100.10",
		EndpointPort: 51820,
		Spec: map[string]any{
			"config_content": "[Interface]\nAddress = 10.99.0.1/24\nListenPort = 51820\nPrivateKey = integration-test\n",
		},
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if instance.SystemdUnit != "wg-quick@wg-"+suffix {
		t.Fatalf("instance systemd_unit = %q, want wg-quick@wg-%s", instance.SystemdUnit, suffix)
	}

	applyJob, ok, err := store.AgentNextJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("claim instance apply job: %v", err)
	}
	if !ok {
		t.Fatal("expected queued instance apply job")
	}
	if applyJob.Type != "instance.apply" {
		t.Fatalf("claimed job type = %q, want instance.apply", applyJob.Type)
	}
	assertResourceLockCount(t, ctx, store, applyJob.ID, "instance", "apply", 1)

	if err := store.CompleteJob(ctx, applyJob.ID, "succeeded", map[string]any{"active_state": "active"}); err != nil {
		t.Fatalf("complete instance apply job: %v", err)
	}
	assertResourceLockCount(t, ctx, store, applyJob.ID, "instance", "apply", 0)

	runtimeState, err := store.GetInstanceRuntimeState(ctx, instance.ID)
	if err != nil {
		t.Fatalf("get runtime state: %v", err)
	}
	if runtimeState.RuntimeStatus != "active" || runtimeState.HealthStatus != "healthy" || runtimeState.DriftStatus != "in_sync" {
		t.Fatalf("runtime projection = runtime:%s health:%s drift:%s, want active/healthy/in_sync", runtimeState.RuntimeStatus, runtimeState.HealthStatus, runtimeState.DriftStatus)
	}
	observations, err := store.ListInstanceRuntimeObservations(ctx, instance.ID, 10)
	if err != nil {
		t.Fatalf("list runtime observations after apply: %v", err)
	}
	if len(observations) != 1 || observations[0].Source != "job" || observations[0].RuntimeStatus != "active" {
		t.Fatalf("runtime observations after apply = %#v, want one active job observation", observations)
	}

	targets, err := store.ListAgentInstanceRuntimeTargets(ctx, node.ID)
	if err != nil {
		t.Fatalf("list agent runtime targets: %v", err)
	}
	if len(targets) != 1 || targets[0].InstanceID != instance.ID {
		t.Fatalf("runtime targets = %#v, want one target for instance %s", targets, instance.ID)
	}
	if targets[0].ConfigPath != "/etc/wireguard/wg-"+suffix+".conf" {
		t.Fatalf("runtime target config_path = %q, want default wireguard config path", targets[0].ConfigPath)
	}
	reportRevisionID := runtimeState.AppliedRevisionID
	agentStates, err := store.SubmitAgentInstanceRuntimeReports(ctx, node.ID, []domain.AgentInstanceRuntimeReport{{
		InstanceID:         instance.ID,
		ServiceCode:        "wireguard",
		SystemdUnit:        instance.SystemdUnit,
		ConfigPath:         targets[0].ConfigPath,
		ConfigHash:         "sha256:integration-runtime",
		ActiveState:        "active",
		EnabledState:       "enabled",
		ObservedRevisionID: reportRevisionID,
		ListeningPorts:     []map[string]any{{"network": "udp", "state": "UNCONN", "local_address": "0.0.0.0:51820", "port": 51820}},
	}})
	if err != nil {
		t.Fatalf("submit agent runtime report: %v", err)
	}
	if len(agentStates) != 1 {
		t.Fatalf("agent runtime states len = %d, want 1", len(agentStates))
	}
	if agentStates[0].ConfigHash != "sha256:integration-runtime" || agentStates[0].EnabledState != "enabled" || agentStates[0].AgentReportedAt == nil {
		t.Fatalf("agent runtime state = %#v, want hash/enabled/agent_reported_at", agentStates[0])
	}
	if agentStates[0].RuntimeStatus != "active" || agentStates[0].HealthStatus != "healthy" || agentStates[0].DriftStatus != "in_sync" {
		t.Fatalf("agent runtime projection = runtime:%s health:%s drift:%s, want active/healthy/in_sync", agentStates[0].RuntimeStatus, agentStates[0].HealthStatus, agentStates[0].DriftStatus)
	}
	observations, err = store.ListInstanceRuntimeObservations(ctx, instance.ID, 10)
	if err != nil {
		t.Fatalf("list runtime observations after agent report: %v", err)
	}
	if len(observations) < 2 || observations[0].Source != "agent" || observations[0].ConfigHash != "sha256:integration-runtime" {
		t.Fatalf("runtime observations after agent report = %#v, want latest agent observation with config hash", observations)
	}
	if len(observations[0].ListeningPorts) != 1 || observations[0].ListeningPorts[0]["local_address"] != "0.0.0.0:51820" {
		t.Fatalf("agent observation listening ports = %#v, want expected endpoint port", observations[0].ListeningPorts)
	}

	// Complete the route-policy job queued by instance.apply before creating the
	// client, so the next route-policy payload proves it contains access routes.
	if routeJob, ok, err := store.AgentNextJob(ctx, node.ID); err != nil {
		t.Fatalf("claim post-apply route policy job: %v", err)
	} else if ok {
		if routeJob.Type != "node.route_policy.apply" {
			t.Fatalf("claimed post-apply job type = %q, want node.route_policy.apply", routeJob.Type)
		}
		if err := store.CompleteJob(ctx, routeJob.ID, "succeeded", map[string]any{"active_state": "active"}); err != nil {
			t.Fatalf("complete post-apply route policy job: %v", err)
		}
	}

	client, err := store.CreateClient(ctx, domain.Client{
		Username:    "it-client-" + suffix,
		DisplayName: "Integration Client",
		Email:       "it-client-" + suffix + "@example.invalid",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	provisionJob, err := store.ProvisionClient(ctx, client.ID, []string{instance.ID})
	if err != nil {
		t.Fatalf("provision client: %v", err)
	}
	if provisionJob.Type != "client.provision" {
		t.Fatalf("provision job type = %q, want client.provision", provisionJob.Type)
	}

	accesses, err := store.ListServiceAccesses(ctx, client.ID)
	if err != nil {
		t.Fatalf("list service accesses: %v", err)
	}
	if len(accesses) != 1 || accesses[0].Status != "pending" || accesses[0].InstanceID != instance.ID {
		t.Fatalf("service accesses = %#v, want one pending access for instance", accesses)
	}
	inbound, _ := accesses[0].Metadata["inbound_service"].(map[string]any)
	if inbound["service_code"] != "wireguard" || inbound["node_id"] != node.ID || inbound["endpoint_port"] == nil {
		t.Fatalf("service access inbound metadata = %#v, want service/node/endpoint snapshot", inbound)
	}
	routes, err := store.ListClientAccessRoutes(ctx, client.ID)
	if err != nil {
		t.Fatalf("list client access routes: %v", err)
	}
	if len(routes) != 1 || routes[0].ServiceAccessID == nil || routes[0].NodeID == nil || *routes[0].NodeID != node.ID {
		t.Fatalf("client access routes = %#v, want one baseline route for node", routes)
	}
	routeInbound, _ := routes[0].Metadata["inbound"].(map[string]any)
	if routeInbound["service_code"] != "wireguard" || routeInbound["instance_id"] != instance.ID {
		t.Fatalf("baseline route inbound metadata = %#v, want service snapshot", routeInbound)
	}

	workerJob, ok, err := store.ClaimJob(ctx, "integration-worker")
	if err != nil {
		t.Fatalf("claim worker job: %v", err)
	}
	if !ok {
		t.Fatal("expected queued client provisioning worker job")
	}
	if workerJob.ID != provisionJob.ID || workerJob.Type != "client.provision" {
		t.Fatalf("worker claimed %s/%s, want %s/client.provision", workerJob.ID, workerJob.Type, provisionJob.ID)
	}
	assertResourceLockCount(t, ctx, store, workerJob.ID, "client", "provision", 1)
	if err := store.CompleteJob(ctx, workerJob.ID, "succeeded", map[string]any{"message": "integration provisioning complete"}); err != nil {
		t.Fatalf("complete client provision job: %v", err)
	}
	assertResourceLockCount(t, ctx, store, workerJob.ID, "client", "provision", 0)

	queuedRouteJob, err := store.CreateNodeRoutePolicyApplyJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("create route policy apply job: %v", err)
	}
	agentRouteJob, ok, err := store.AgentNextJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("claim route policy apply job: %v", err)
	}
	if !ok {
		t.Fatal("expected queued route policy apply job")
	}
	if agentRouteJob.ID != queuedRouteJob.ID {
		t.Fatalf("agent claimed route job %s, want %s", agentRouteJob.ID, queuedRouteJob.ID)
	}
	if routeCount, ok := agentRouteJob.Payload["route_count"].(float64); !ok || int(routeCount) != 1 {
		t.Fatalf("route policy route_count = %#v, want 1", agentRouteJob.Payload["route_count"])
	}
	payloadRoutes, _ := agentRouteJob.Payload["routes"].([]any)
	if len(payloadRoutes) != 1 {
		t.Fatalf("route policy routes = %#v, want one route", agentRouteJob.Payload["routes"])
	}
	payloadRoute, _ := payloadRoutes[0].(map[string]any)
	payloadInbound, _ := payloadRoute["inbound_service"].(map[string]any)
	if payloadInbound["service_code"] != "wireguard" || payloadInbound["instance_id"] != instance.ID {
		t.Fatalf("route policy inbound_service = %#v, want service snapshot", payloadInbound)
	}
}

func TestPostgresIntegrationBackhaulRouteToggleRefreshesRoutePolicy(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	ingress, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-ingress-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "198.51.100.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create ingress node: %v", err)
	}
	egress, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-egress-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "203.0.113.20",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create egress node: %v", err)
	}
	link, err := store.CreateBackhaulLink(ctx, domain.BackhaulLink{
		Name:          "it-backhaul-" + suffix,
		IngressNodeID: ingress.ID,
		EgressNodeID:  egress.ID,
		DesiredDriver: backhaul.DriverWireGuard,
		RouteMetric:   20,
		Metadata: map[string]any{
			"endpoint_host": egress.Address,
			"tunnel_cidr":   "10.240.251.0/30",
		},
	})
	if err != nil {
		t.Fatalf("create backhaul link: %v", err)
	}
	if len(link.Transports) != 1 {
		t.Fatalf("backhaul transports = %#v, want one transport", link.Transports)
	}

	applyJobs, err := store.CreateBackhaulApplyJobs(ctx, link.ID)
	if err != nil {
		t.Fatalf("create backhaul apply jobs: %v", err)
	}
	if len(applyJobs) != 2 {
		t.Fatalf("backhaul apply jobs = %d, want ingress and egress jobs", len(applyJobs))
	}
	for _, job := range applyJobs {
		role, _ := job.Payload["role"].(string)
		if role != "ingress" && role != "egress" {
			t.Fatalf("backhaul apply role = %#v, want ingress or egress", job.Payload["role"])
		}
		if err := store.CompleteJob(ctx, job.ID, "succeeded", map[string]any{
			"link_id":      link.ID,
			"transport_id": link.Transports[0].ID,
			"role":         role,
			"health":       map[string]any{"status": "healthy"},
		}); err != nil {
			t.Fatalf("complete %s backhaul apply job: %v", role, err)
		}
	}
	activeLink, err := store.GetBackhaulLink(ctx, link.ID)
	if err != nil {
		t.Fatalf("get active backhaul link: %v", err)
	}
	if activeLink.Status != "active" || len(activeLink.Transports) != 1 || activeLink.Transports[0].Status != "active" {
		t.Fatalf("backhaul state = link:%s transports:%#v, want active selected transport", activeLink.Status, activeLink.Transports)
	}

	client, err := store.CreateClient(ctx, domain.Client{
		Username:    "it-route-client-" + suffix,
		DisplayName: "Integration Route Client",
		Email:       "it-route-client-" + suffix + "@example.invalid",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	route, err := store.CreateClientAccessRoute(ctx, client.ID, domain.ClientAccessRoute{
		NodeID:          &ingress.ID,
		Name:            "remote-egress-" + suffix,
		Status:          "active",
		Action:          "allow",
		DestinationType: "cidr",
		Destination:     "0.0.0.0/0",
		Protocol:        "any",
		Ports:           "*",
		Policy: map[string]any{
			"egress_mode":    "egress_node",
			"egress_node_id": egress.ID,
		},
	})
	if err != nil {
		t.Fatalf("create client access route: %v", err)
	}
	routeJob, err := store.CreateNodeRoutePolicyApplyJob(ctx, ingress.ID)
	if err != nil {
		t.Fatalf("create route policy job with active backhaul: %v", err)
	}
	routeEntry := routePolicyPayloadRoute(t, routeJob.Payload, route.ID)
	egressProjection := routePolicyPayloadEgress(t, routeEntry)
	if got := strings.TrimSpace(stringify(egressProjection["status"])); got != "candidate" {
		t.Fatalf("active route egress status = %q, want candidate; egress=%#v", got, egressProjection)
	}
	managedBackhaul, _ := egressProjection["managed_backhaul"].(map[string]any)
	if managedBackhaul["link_id"] != link.ID || managedBackhaul["transport_id"] != link.Transports[0].ID {
		t.Fatalf("managed backhaul = %#v, want active link %s transport %s", managedBackhaul, link.ID, link.Transports[0].ID)
	}

	disabledLink, disableJobs, err := store.SetBackhaulRouteEnabled(ctx, link.ID, false)
	if err != nil {
		t.Fatalf("disable backhaul route: %v", err)
	}
	if disabledLink.Status != "disabled" {
		t.Fatalf("disabled link status = %q, want disabled", disabledLink.Status)
	}
	var refreshJob *domain.Job
	for idx := range disableJobs {
		job := disableJobs[idx]
		switch job.Type {
		case "node.backhaul.cleanup":
			t.Fatalf("disable route queued cleanup job: %#v", job)
		case "node.route_policy.apply":
			refreshJob = &disableJobs[idx]
		}
	}
	if len(disableJobs) != 1 || refreshJob == nil {
		t.Fatalf("disable jobs = %#v, want one route policy refresh and no cleanup jobs", disableJobs)
	}
	disabledRoute := routePolicyPayloadRoute(t, refreshJob.Payload, route.ID)
	disabledEgress := routePolicyPayloadEgress(t, disabledRoute)
	if got := strings.TrimSpace(stringify(disabledEgress["status"])); got != "blocked" {
		t.Fatalf("disabled route egress status = %q, want blocked; egress=%#v", got, disabledEgress)
	}
	if _, ok := disabledEgress["managed_backhaul"]; ok {
		t.Fatalf("disabled route still contains managed_backhaul: %#v", disabledEgress)
	}

	reloaded, err := store.GetBackhaulLink(ctx, link.ID)
	if err != nil {
		t.Fatalf("reload disabled backhaul: %v", err)
	}
	if reloaded.Status != "disabled" || len(reloaded.Transports) != 1 || reloaded.Transports[0].Status != "active" {
		t.Fatalf("reloaded disabled backhaul = link:%s transports:%#v, want disabled link with active transport", reloaded.Status, reloaded.Transports)
	}

	enabledLink, enableJobs, err := store.SetBackhaulRouteEnabled(ctx, link.ID, true)
	if err != nil {
		t.Fatalf("enable backhaul route: %v", err)
	}
	if enabledLink.Status != "active" {
		t.Fatalf("enabled link status = %q, want active", enabledLink.Status)
	}
	var enableRefreshJob *domain.Job
	for idx := range enableJobs {
		job := enableJobs[idx]
		switch job.Type {
		case "node.backhaul.cleanup", "node.backhaul.apply":
			t.Fatalf("enable route queued transport job: %#v", job)
		case "node.route_policy.apply":
			enableRefreshJob = &enableJobs[idx]
		}
	}
	if len(enableJobs) != 1 || enableRefreshJob == nil {
		t.Fatalf("enable jobs = %#v, want one route policy refresh", enableJobs)
	}
	enabledRoute := routePolicyPayloadRoute(t, enableRefreshJob.Payload, route.ID)
	enabledEgress := routePolicyPayloadEgress(t, enabledRoute)
	if got := strings.TrimSpace(stringify(enabledEgress["status"])); got != "candidate" {
		t.Fatalf("re-enabled route egress status = %q, want candidate; egress=%#v", got, enabledEgress)
	}
	enabledManagedBackhaul, _ := enabledEgress["managed_backhaul"].(map[string]any)
	if enabledManagedBackhaul["link_id"] != link.ID || enabledManagedBackhaul["transport_id"] != link.Transports[0].ID {
		t.Fatalf("re-enabled managed backhaul = %#v, want active link %s transport %s", enabledManagedBackhaul, link.ID, link.Transports[0].ID)
	}
}

func TestPostgresIntegrationBackhaulPromoteStandbyTransportRefreshesRoutePolicy(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	ingress, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-ingress-promote-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "198.51.100.30",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create ingress node: %v", err)
	}
	egress, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-egress-promote-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "203.0.113.30",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create egress node: %v", err)
	}
	link, err := store.CreateBackhaulLink(ctx, domain.BackhaulLink{
		Name:          "it-backhaul-promote-" + suffix,
		IngressNodeID: ingress.ID,
		EgressNodeID:  egress.ID,
		DesiredDriver: backhaul.DriverWireGuard,
		RouteMetric:   30,
		Metadata: map[string]any{
			"endpoint_host": egress.Address,
			"tunnel_cidr":   "10.240.252.0/30",
			"drivers":       []any{backhaul.DriverWireGuard, backhaul.DriverOpenVPNUDP},
		},
	})
	if err != nil {
		t.Fatalf("create backhaul link: %v", err)
	}
	if len(link.Transports) != 2 {
		t.Fatalf("backhaul transports = %#v, want wireguard and openvpn", link.Transports)
	}
	var wireguardTransport, openVPNTransport *domain.BackhaulTransport
	for idx := range link.Transports {
		switch link.Transports[idx].Driver {
		case backhaul.DriverWireGuard:
			wireguardTransport = &link.Transports[idx]
		case backhaul.DriverOpenVPNUDP:
			openVPNTransport = &link.Transports[idx]
		}
	}
	if wireguardTransport == nil || openVPNTransport == nil {
		t.Fatalf("backhaul transports = %#v, want wireguard and openvpn", link.Transports)
	}

	client, err := store.CreateClient(ctx, domain.Client{
		Username:    "it-route-promote-" + suffix,
		DisplayName: "Integration Promote Route Client",
		Email:       "it-route-promote-" + suffix + "@example.invalid",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	route, err := store.CreateClientAccessRoute(ctx, client.ID, domain.ClientAccessRoute{
		NodeID:          &ingress.ID,
		Name:            "remote-egress-promote-" + suffix,
		Status:          "active",
		Action:          "allow",
		DestinationType: "cidr",
		Destination:     "0.0.0.0/0",
		Protocol:        "any",
		Ports:           "*",
		Policy: map[string]any{
			"egress_mode":    "egress_node",
			"egress_node_id": egress.ID,
		},
	})
	if err != nil {
		t.Fatalf("create client access route: %v", err)
	}

	applyJobs, err := store.CreateBackhaulApplyJobs(ctx, link.ID)
	if err != nil {
		t.Fatalf("create backhaul apply jobs: %v", err)
	}
	for _, job := range applyJobs {
		role := strings.TrimSpace(stringify(job.Payload["role"]))
		transportID := strings.TrimSpace(stringify(job.Payload["transport_id"]))
		driver := strings.TrimSpace(stringify(job.Payload["driver"]))
		status := "succeeded"
		health := map[string]any{"status": "healthy", "reason": "service active and interface present"}
		if driver == backhaul.DriverWireGuard {
			status = "failed"
			health = map[string]any{"status": "unhealthy", "reason": "systemd unit is not active", "active_state": "unknown"}
		}
		if err := store.CompleteJob(ctx, job.ID, status, map[string]any{
			"link_id":      link.ID,
			"transport_id": transportID,
			"role":         role,
			"health":       health,
		}); err != nil {
			t.Fatalf("complete %s %s backhaul apply job: %v", driver, role, err)
		}
	}

	failedLink, err := store.GetBackhaulLink(ctx, link.ID)
	if err != nil {
		t.Fatalf("reload failed backhaul link: %v", err)
	}
	if failedLink.Status != "failed" {
		t.Fatalf("link status before promote = %q, want failed selected transport", failedLink.Status)
	}
	promoted, jobs, err := store.PromoteBackhaulTransport(ctx, link.ID, openVPNTransport.ID)
	if err != nil {
		t.Fatalf("promote openvpn standby transport: %v", err)
	}
	if promoted.Status != "active" || promoted.DesiredDriver != backhaul.DriverOpenVPNUDP || promoted.SelectedTransportID == nil || *promoted.SelectedTransportID != openVPNTransport.ID {
		t.Fatalf("promoted link = status:%s desired:%s selected:%v, want active openvpn %s", promoted.Status, promoted.DesiredDriver, promoted.SelectedTransportID, openVPNTransport.ID)
	}
	if len(jobs) != 1 || jobs[0].Type != "node.route_policy.apply" {
		t.Fatalf("promotion jobs = %#v, want one route policy refresh", jobs)
	}
	promotedRoute := routePolicyPayloadRoute(t, jobs[0].Payload, route.ID)
	promotedEgress := routePolicyPayloadEgress(t, promotedRoute)
	managedBackhaul, _ := promotedEgress["managed_backhaul"].(map[string]any)
	if managedBackhaul["transport_id"] != openVPNTransport.ID {
		t.Fatalf("promoted managed_backhaul = %#v, want openvpn transport %s", managedBackhaul, openVPNTransport.ID)
	}
	if managedBackhaul["transport_id"] == wireguardTransport.ID {
		t.Fatalf("promoted managed_backhaul still points to wireguard: %#v", managedBackhaul)
	}
}

func TestPostgresIntegrationBinaryRepositoryTicketLifecycle(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-node-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.0.20",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "x86_64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	artifact, err := store.CreateBinaryArtifact(ctx, domain.BinaryArtifact{
		Name:         "xray-install-" + suffix,
		Kind:         "script",
		ServiceCode:  "xray-core",
		Version:      "1.2.3",
		OSFamily:     "linux",
		Architecture: "amd64",
		StoragePath:  "runtime/xray-install.sh",
		SHA256:       strings.Repeat("a", 64),
		Metadata:     map[string]any{"install_mode": "xray_install_script"},
	})
	if err != nil {
		t.Fatalf("create binary artifact: %v", err)
	}
	instance, err := store.CreateInstanceDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "xray-" + suffix,
		Slug:         "xray-" + suffix,
		EndpointHost: "198.51.100.20",
		EndpointPort: 8443,
		Spec: map[string]any{
			"config_json": map[string]any{"inbounds": []any{}},
		},
	})
	if err != nil {
		t.Fatalf("create dependent draft instance: %v", err)
	}
	job, err := store.CreateNodeCapabilityInstallJobWithDependents(ctx, node.ID, "xray-core", "", "", []string{instance.ID})
	if err != nil {
		t.Fatalf("create capability install job: %v", err)
	}
	if got := job.Payload["strategy"]; got != "binary_repository" {
		t.Fatalf("strategy = %v, want binary_repository; payload=%#v", got, job.Payload)
	}
	repo, ok := job.Payload["binary_repository"].(map[string]any)
	if !ok {
		t.Fatalf("binary_repository payload missing: %#v", job.Payload)
	}
	if got := repo["artifact_id"]; got != artifact.ID {
		t.Fatalf("artifact_id = %v, want %s", got, artifact.ID)
	}
	token := stringify(repo["download_token"])
	if token == "" {
		t.Fatal("download token must be present for the agent payload")
	}
	capabilities, err := store.ListNodeCapabilities(ctx, node.ID)
	if err != nil {
		t.Fatalf("list node capabilities: %v", err)
	}
	if len(capabilities) != 1 || capabilities[0].CapabilityCode != "xray-core" || capabilities[0].Status != "installing" {
		t.Fatalf("capabilities after queue = %#v, want xray-core installing", capabilities)
	}
	runtimeState, err := store.GetInstanceRuntimeState(ctx, instance.ID)
	if err != nil {
		t.Fatalf("get dependent runtime state: %v", err)
	}
	if runtimeState.LastJobID == nil || *runtimeState.LastJobID != job.ID || runtimeState.LastJobType != "node.capability.install" || runtimeState.LastJobStatus != "queued" {
		t.Fatalf("dependent runtime job state = %#v, want queued capability install job %s", runtimeState, job.ID)
	}
	if runtimeState.RuntimeStatus != "provisioning" || runtimeState.HealthStatus != "provisioning" || runtimeState.DriftStatus != "pending_apply" {
		t.Fatalf("dependent runtime projection = runtime:%s health:%s drift:%s, want provisioning/provisioning/pending_apply", runtimeState.RuntimeStatus, runtimeState.HealthStatus, runtimeState.DriftStatus)
	}
	secondInstance, err := store.CreateInstanceDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "xray-second-" + suffix,
		Slug:         "xray-second-" + suffix,
		EndpointHost: "198.51.100.21",
		EndpointPort: 8444,
		Spec: map[string]any{
			"config_json": map[string]any{"inbounds": []any{}},
		},
	})
	if err != nil {
		t.Fatalf("create second dependent draft instance: %v", err)
	}
	reusedJob, err := store.CreateNodeCapabilityInstallJobWithDependents(ctx, node.ID, "xray-core", "", "", []string{secondInstance.ID})
	if err != nil {
		t.Fatalf("reuse active capability install job: %v", err)
	}
	if reusedJob.ID != job.ID {
		t.Fatalf("reused job id = %s, want active job %s", reusedJob.ID, job.ID)
	}
	dependentIDs := stringSetFromAny(reusedJob.Payload["dependent_instance_ids"])
	if !containsString(dependentIDs, instance.ID) || !containsString(dependentIDs, secondInstance.ID) {
		t.Fatalf("reused dependent ids = %#v, want both %s and %s", dependentIDs, instance.ID, secondInstance.ID)
	}
	secondRuntimeState, err := store.GetInstanceRuntimeState(ctx, secondInstance.ID)
	if err != nil {
		t.Fatalf("get second dependent runtime state: %v", err)
	}
	if secondRuntimeState.LastJobID == nil || *secondRuntimeState.LastJobID != job.ID || secondRuntimeState.LastJobType != "node.capability.install" || secondRuntimeState.LastJobStatus != "queued" {
		t.Fatalf("second dependent runtime job state = %#v, want queued capability install job %s", secondRuntimeState, job.ID)
	}
	ticket, resolved, err := store.ResolveBinaryDownloadTicket(ctx, token, artifact.ID, node.ID, job.ID)
	if err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}
	if resolved.ID != artifact.ID {
		t.Fatalf("resolved artifact = %s, want %s", resolved.ID, artifact.ID)
	}
	if ticket.Status != "active" {
		t.Fatalf("ticket status = %q, want active before download is marked complete", ticket.Status)
	}
	if _, _, err := store.ResolveBinaryDownloadTicket(ctx, token, artifact.ID, node.ID, job.ID); err != nil {
		t.Fatalf("resolve ticket second time before completion: %v", err)
	}
	if err := store.CompleteJob(ctx, job.ID, "failed", map[string]any{
		"message":      "install failed after verified download",
		"service_code": "xray-core",
		"binary_repository": map[string]any{
			"download_verified":  true,
			"download_ticket_id": ticket.ID,
			"sha256":             artifact.SHA256,
		},
	}); err != nil {
		t.Fatalf("complete capability install job: %v", err)
	}
	if _, _, err := store.ResolveBinaryDownloadTicket(ctx, token, artifact.ID, node.ID, job.ID); err == nil {
		t.Fatal("download ticket must be single-use after verified completion")
	}
}

func TestPostgresIntegrationShadowsocksUbuntuInstallCarriesBinaryFallback(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-ss-node-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.0.25",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "x86_64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	artifact, err := store.CreateBinaryArtifact(ctx, domain.BinaryArtifact{
		Name:         "ss-server-" + suffix,
		Kind:         "runtime",
		ServiceCode:  "shadowsocks",
		Version:      "3.3.5",
		OSFamily:     "linux",
		Architecture: "amd64",
		StoragePath:  "runtime/ss-server",
		SHA256:       strings.Repeat("c", 64),
		Metadata: map[string]any{
			"install_mode": "copy_binary",
			"install_path": "/usr/local/bin/ss-server",
		},
	})
	if err != nil {
		t.Fatalf("create binary artifact: %v", err)
	}
	job, err := store.CreateNodeCapabilityInstallJobWithDependents(ctx, node.ID, "shadowsocks", "ubuntu_repo", "", nil)
	if err != nil {
		t.Fatalf("create shadowsocks capability install job: %v", err)
	}
	if got := job.Payload["strategy"]; got != "ubuntu_repo" {
		t.Fatalf("strategy = %v, want ubuntu_repo; payload=%#v", got, job.Payload)
	}
	fallback, ok := job.Payload["binary_repository_fallback"].(map[string]any)
	if !ok {
		t.Fatalf("binary_repository_fallback payload missing: %#v", job.Payload)
	}
	if got := fallback["artifact_id"]; got != artifact.ID {
		t.Fatalf("fallback artifact_id = %v, want %s", got, artifact.ID)
	}
	if token := stringify(fallback["download_token"]); token == "" {
		t.Fatal("fallback download token must be present for the agent payload")
	}
}

func TestPostgresIntegrationCleanupBinaryDownloadTickets(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-cleanup-node-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.0.30",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	artifact, err := store.CreateBinaryArtifact(ctx, domain.BinaryArtifact{
		Name:         "cleanup-install-" + suffix,
		Kind:         "script",
		ServiceCode:  "shadowsocks",
		Version:      "1.2.3",
		OSFamily:     "linux",
		Architecture: "amd64",
		StoragePath:  "runtime/cleanup-install.sh",
		SHA256:       strings.Repeat("b", 64),
		Metadata:     map[string]any{"install_mode": "shell_script"},
	})
	if err != nil {
		t.Fatalf("create binary artifact: %v", err)
	}

	now := time.Now().UTC()
	expiredActiveTicketID := id.New()
	deletableUsedTicketID := id.New()
	if _, err := store.db.Exec(ctx, `insert into binary_download_tickets(id,artifact_id,node_id,token_hash,token_hint,status,expires_at,created_at)
		values($1,$2,$3,$4,$5,'active',$6,$7)`,
		expiredActiveTicketID, artifact.ID, node.ID, hashToken("cleanup-active-"+suffix), "cleanup-active", now.Add(-time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("insert active expired ticket: %v", err)
	}
	if _, err := store.db.Exec(ctx, `insert into binary_download_tickets(id,artifact_id,node_id,token_hash,token_hint,status,expires_at,used_at,created_at)
		values($1,$2,$3,$4,$5,'used',$6,$7,$8)`,
		deletableUsedTicketID, artifact.ID, node.ID, hashToken("cleanup-used-"+suffix), "cleanup-used", now.Add(-72*time.Hour), now.Add(-48*time.Hour), now.Add(-72*time.Hour)); err != nil {
		t.Fatalf("insert old used ticket: %v", err)
	}

	expired, deleted, err := store.CleanupBinaryDownloadTickets(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("cleanup binary download tickets: %v", err)
	}
	if expired < 1 {
		t.Fatalf("expired tickets = %d, want at least 1", expired)
	}
	if deleted < 1 {
		t.Fatalf("deleted tickets = %d, want at least 1", deleted)
	}

	var activeStatus string
	if err := store.db.QueryRow(ctx, `select status from binary_download_tickets where id=$1`, expiredActiveTicketID).Scan(&activeStatus); err != nil {
		t.Fatalf("query expired active ticket: %v", err)
	}
	if activeStatus != "expired" {
		t.Fatalf("active ticket status = %q, want expired", activeStatus)
	}
	var oldTicketCount int
	if err := store.db.QueryRow(ctx, `select count(*) from binary_download_tickets where id=$1`, deletableUsedTicketID).Scan(&oldTicketCount); err != nil {
		t.Fatalf("query deleted old ticket: %v", err)
	}
	if oldTicketCount != 0 {
		t.Fatalf("old used ticket count = %d, want 0", oldTicketCount)
	}
}

func TestPostgresIntegrationCapabilityInstallMissingBinaryArtifactDiagnostic(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-missing-artifact-node-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.0.40",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "x86_64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	_, err = store.CreateNodeCapabilityInstallJobWithDependents(ctx, node.ID, "xray-core", "binary_repository", "", nil)
	if err == nil {
		t.Fatal("expected missing binary artifact error")
	}
	message := err.Error()
	for _, want := range []string{
		"binary repository artifact is not available",
		"service_code=xray-core",
		"os_family=linux",
		"architecture=amd64",
		"ubuntu-24.04",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want to contain %q", message, want)
		}
	}
	if strings.Contains(message, "no rows") {
		t.Fatalf("error = %q, should not expose raw database no rows", message)
	}
}

func TestPostgresIntegrationDeletedInstanceDoesNotReserveSlugOrNodeName(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-recreate-node-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.0.50",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	first, err := store.CreateInstanceDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "wireguard",
		Name:         "recreated-" + suffix,
		Slug:         "recreated-" + suffix,
		EndpointHost: "198.51.100.50",
		EndpointPort: 51820,
		Spec: map[string]any{
			"network_cidr":   "10.70.0.0/24",
			"server_address": "10.70.0.1/24",
		},
	})
	if err != nil {
		t.Fatalf("create first instance: %v", err)
	}
	if _, err := store.db.Exec(ctx, `update instances set status='deleted',enabled=false,updated_at=now() where id=$1`, first.ID); err != nil {
		t.Fatalf("mark first instance deleted: %v", err)
	}

	second, err := store.CreateInstanceDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "wireguard",
		Name:         first.Name,
		Slug:         first.Slug,
		EndpointHost: "198.51.100.50",
		EndpointPort: 51820,
		Spec: map[string]any{
			"network_cidr":   "10.71.0.0/24",
			"server_address": "10.71.0.1/24",
		},
	})
	if err != nil {
		t.Fatalf("create replacement instance with same slug/name after delete: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("replacement instance must be a new row")
	}
	if second.Slug != first.Slug || second.Name != first.Name {
		t.Fatalf("replacement identity = %s/%s, want %s/%s", second.Slug, second.Name, first.Slug, first.Name)
	}
}

func TestPostgresIntegrationValidatedDraftRejectsInvalidCurrentRevision(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-validated-draft-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.0.60",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	slug := "invalid-xray-" + suffix
	_, err = store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         slug,
		Slug:         slug,
		EndpointHost: "198.51.100.60",
		EndpointPort: 8443,
		Spec: map[string]any{
			"config_json": map[string]any{"inbounds": []any{}},
		},
	})
	if err == nil {
		t.Fatal("expected invalid validated draft to fail")
	}
	if !strings.Contains(err.Error(), "initial instance revision is not apply-ready") || !strings.Contains(err.Error(), "xray config_json must contain at least one inbound") {
		t.Fatalf("unexpected error: %v", err)
	}
	var count int
	if err := store.db.QueryRow(ctx, `select count(*) from instances where slug=$1`, slug).Scan(&count); err != nil {
		t.Fatalf("count discarded instance: %v", err)
	}
	if count != 0 {
		t.Fatalf("discarded instance count = %d, want 0", count)
	}

	draft, err := store.CreateInstanceDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         slug,
		Slug:         slug,
		EndpointHost: "198.51.100.60",
		EndpointPort: 8443,
		Spec: map[string]any{
			"config_json": map[string]any{"inbounds": []any{}},
		},
	})
	if err != nil {
		t.Fatalf("plain draft must still be persisted for manual repair: %v", err)
	}
	revisions, err := store.ListInstanceRevisions(ctx, draft.ID, 1)
	if err != nil {
		t.Fatalf("list draft revisions: %v", err)
	}
	if len(revisions) != 1 || revisions[0].Status != "draft" {
		t.Fatalf("plain draft revision status = %#v, want draft", revisions)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestPostgresIntegrationRecoverStaleJobLeases(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-stale-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.1.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	job, err := store.CreateJob(ctx, domain.Job{
		Type:      "node.route_policy.apply",
		ScopeType: "node",
		ScopeID:   &node.ID,
		NodeID:    &node.ID,
		Priority:  20,
		Payload: map[string]any{
			"node_id": node.ID,
		},
	})
	if err != nil {
		t.Fatalf("create route policy job: %v", err)
	}

	claimed, ok, err := store.AgentNextJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("claim route policy job: %v", err)
	}
	if !ok || claimed.ID != job.ID || claimed.Status != "running" || claimed.LockedUntil == nil {
		t.Fatalf("claimed job = %#v, want running claimed job %s", claimed, job.ID)
	}
	assertResourceLockCount(t, ctx, store, claimed.ID, "node", "bootstrap", 1)

	if _, err := store.db.Exec(ctx, `update jobs set locked_until=$2 where id=$1`, claimed.ID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("expire job lease: %v", err)
	}
	recoveredCount, err := store.RecoverStaleJobLeases(ctx)
	if err != nil {
		t.Fatalf("recover stale job leases: %v", err)
	}
	if recoveredCount != 1 {
		t.Fatalf("recovered job count = %d, want 1", recoveredCount)
	}
	recovered, err := store.GetJob(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("get recovered job: %v", err)
	}
	if recovered.Status != "retrying" || recovered.LockedBy != nil || recovered.LockedUntil != nil {
		t.Fatalf("recovered job state = status:%s locked_by:%v locked_until:%v, want retrying with no lease", recovered.Status, recovered.LockedBy, recovered.LockedUntil)
	}
	assertResourceLockCount(t, ctx, store, claimed.ID, "node", "bootstrap", 0)
	if err := store.CompleteAgentJob(ctx, claimed.ID, "agent:"+node.ID, "succeeded", map[string]any{"message": "stale result"}); err == nil {
		t.Fatal("expected stale unleased agent completion to be rejected")
	}

	cancelled, err := store.CancelJob(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("cancel recovered job: %v", err)
	}
	if cancelled.Status != "cancelled" || cancelled.FinishedAt == nil {
		t.Fatalf("cancelled job = %#v, want terminal cancelled job", cancelled)
	}
}

func TestPostgresIntegrationAgentVersionProjection(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-agent-version-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "draft",
		Address:       "10.50.2.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	token, err := store.CreateNodeEnrollmentToken(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatalf("create enrollment token: %v", err)
	}
	const registeredAgentVersion = "previous-agent-build"
	const heartbeatAgentVersion = "current-agent-build"

	if _, _, err := store.RegisterAgentWithEnrollmentVersion(ctx, node.ID, token.Token, node.Name, node.Address, registeredAgentVersion, "v1"); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	registered, err := store.GetNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("get registered node: %v", err)
	}
	if registered.AgentVersion != registeredAgentVersion || registered.AgentProtocolVersion != "v1" || registered.AgentRegisteredAt == nil || registered.AgentLastSeenAt == nil {
		t.Fatalf("registered node agent projection = version:%q protocol:%q registered:%v last_seen:%v", registered.AgentVersion, registered.AgentProtocolVersion, registered.AgentRegisteredAt, registered.AgentLastSeenAt)
	}

	if err := store.HeartbeatByNodeIDWithVersion(ctx, node.ID, heartbeatAgentVersion, "v1"); err != nil {
		t.Fatalf("heartbeat with version: %v", err)
	}
	nodes, err := store.ListNodes(ctx)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	var projected *domain.Node
	for idx := range nodes {
		if nodes[idx].ID == node.ID {
			projected = &nodes[idx]
			break
		}
	}
	if projected == nil {
		t.Fatal("registered node missing from list nodes")
	}
	if projected.AgentVersion != heartbeatAgentVersion {
		t.Fatalf("projected agent version = %q, want heartbeat version", projected.AgentVersion)
	}
}

func TestPostgresIntegrationDeleteClientRemovesProvisioningRows(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	store.SetArtifactRoot(t.TempDir())

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-client-cleanup-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "203.0.113.25",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	instance, err := store.CreateInstance(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "it-cleanup-xray-" + suffix,
		Slug:         "it-cleanup-xray-" + suffix,
		EndpointHost: "portal.example.invalid",
		EndpointPort: 7080,
		Spec: map[string]any{
			"security":            "none",
			"network":             "ws",
			"path":                "/assets/rtis-sync",
			"public_security":     "tls",
			"public_network":      "ws",
			"public_path":         "/assets/rtis-sync",
			"public_host_header":  "portal.example.invalid",
			"public_port":         443,
			"default_vless_group": "default",
			"vless_groups": []any{
				map[string]any{"key": "default", "name": "Default", "egress_mode": "auto"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}

	client, err := store.CreateClient(ctx, domain.Client{
		Username:    "it-cleanup-client-" + suffix,
		DisplayName: "Integration Cleanup Client",
		Email:       "it-cleanup-client-" + suffix + "@example.invalid",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := store.ProvisionClientWithOptions(ctx, client.ID, []string{instance.ID}, map[string]map[string]any{
		instance.ID: {"vless_group": "default"},
	}); err != nil {
		t.Fatalf("provision client: %v", err)
	}
	accesses, err := store.ListServiceAccesses(ctx, client.ID)
	if err != nil {
		t.Fatalf("list service accesses: %v", err)
	}
	if len(accesses) != 1 {
		t.Fatalf("service access count = %d, want 1", len(accesses))
	}
	accessID := accesses[0].ID
	artifact, err := store.SaveArtifactContent(ctx, client.ID, &accessID, "vless_url", "client.txt", []byte("vless://integration-test"))
	if err != nil {
		t.Fatalf("save artifact: %v", err)
	}
	if _, err := os.Stat(artifact.StoragePath); err != nil {
		t.Fatalf("artifact file before delete: %v", err)
	}
	share, err := store.PublishShareLink(ctx, client.ID, artifact.ID, time.Hour)
	if err != nil {
		t.Fatalf("publish share link: %v", err)
	}
	sub, err := store.RotateClientSubscription(ctx, client.ID, time.Hour)
	if err != nil {
		t.Fatalf("rotate subscription: %v", err)
	}
	createdBy := "integration-test"
	if _, err := store.CreateClientEmailDelivery(ctx, domain.ClientEmailDelivery{
		ClientAccountID: client.ID,
		Email:           client.Email,
		Subject:         "Integration cleanup",
		Status:          "queued",
		ArtifactIDs:     []string{artifact.ID},
		ShareLinkIDs:    []string{share.ID},
		CreatedBy:       &createdBy,
	}); err != nil {
		t.Fatalf("create email delivery: %v", err)
	}
	secret, err := store.CreateSecretRef(ctx, "uuid", []byte("integration-secret"), map[string]any{
		"scope":             "service_access",
		"service_access_id": accessID,
	})
	if err != nil {
		t.Fatalf("create service access secret: %v", err)
	}

	result, err := store.DeleteClient(ctx, client.ID)
	if err != nil {
		t.Fatalf("delete client: %v", err)
	}
	if !result.Deleted {
		t.Fatalf("delete result did not mark client deleted: %#v", result)
	}
	if result.ConfigCleanup.ArtifactsDeleted != 1 || result.ConfigCleanup.ShareLinksDeleted != 1 || result.ConfigCleanup.SubscriptionsDeleted != 1 {
		t.Fatalf("config cleanup result = %#v, want one artifact/share/subscription", result.ConfigCleanup)
	}
	if result.ServiceAccessesDeleted != 1 || result.AccessRoutesDeleted != 1 || result.EmailDeliveriesDeleted != 1 || result.SecretRefsDeleted != 1 {
		t.Fatalf("delete result = %#v, want one access/route/email/secret", result)
	}

	assertPostgresCount(t, ctx, store, `select count(*) from client_accounts where id=$1`, 0, client.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where client_account_id=$1`, 0, client.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from client_access_routes where client_account_id=$1`, 0, client.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from artifacts where client_account_id=$1`, 0, client.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from share_links where id=$1`, 0, share.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from client_subscriptions where id=$1`, 0, sub.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from client_email_deliveries where client_account_id=$1`, 0, client.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 0, secret.ID)
	if _, err := os.Stat(artifact.StoragePath); !os.IsNotExist(err) {
		t.Fatalf("artifact file after delete error = %v, want not exist", err)
	}
}

func routePolicyPayloadRoute(t *testing.T, payload map[string]any, routeID string) map[string]any {
	t.Helper()

	routes, ok := payload["routes"].([]any)
	if !ok {
		t.Fatalf("route policy payload routes type = %T, want []any; payload=%#v", payload["routes"], payload)
	}
	for _, raw := range routes {
		route, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("route policy route type = %T, want map[string]any", raw)
		}
		if strings.TrimSpace(stringify(route["route_id"])) == routeID {
			return route
		}
	}
	t.Fatalf("route %s not found in route policy payload: %#v", routeID, payload)
	return nil
}

func routePolicyPayloadEgress(t *testing.T, route map[string]any) map[string]any {
	t.Helper()

	egress, ok := route["egress"].(map[string]any)
	if !ok {
		t.Fatalf("route egress type = %T, want map[string]any; route=%#v", route["egress"], route)
	}
	return egress
}

func setupPostgresIntegrationStore(t *testing.T) (*Store, context.Context) {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("MEGAVPN_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("set MEGAVPN_TEST_DATABASE_DSN to run PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	adminPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	schema := "megavpn_it_" + strings.ReplaceAll(id.New(), "-", "")[:16]
	if _, err := adminPool.Exec(ctx, "create schema "+quotePostgresIdentifier(schema)); err != nil {
		adminPool.Close()
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dropCancel()
		_, _ = adminPool.Exec(dropCtx, "drop schema if exists "+quotePostgresIdentifier(schema)+" cascade")
		adminPool.Close()
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse test database dsn: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect test database schema: %v", err)
	}
	t.Cleanup(pool.Close)

	applyPostgresIntegrationMigrations(t, ctx, pool)
	return New(pool), ctx
}

func applyPostgresIntegrationMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	if _, err := pool.Exec(ctx, `create table if not exists schema_migrations(version text primary key, applied_at timestamptz not null default now())`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}

	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no migrations found")
	}
	for _, file := range files {
		version := strings.TrimSuffix(filepath.Base(file), ".up.sql")
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", version, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin migration %s: %v", version, err)
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("apply migration %s: %v", version, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(version, applied_at) values($1, now())`, version); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("record migration %s: %v", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit migration %s: %v", version, err)
		}
	}
}

func assertResourceLockCount(t *testing.T, ctx context.Context, store *Store, jobID, resourceType, lockKind string, want int) {
	t.Helper()

	var got int
	if err := store.db.QueryRow(ctx, `select count(*) from resource_locks where job_id=$1 and resource_type=$2 and lock_kind=$3`, jobID, resourceType, lockKind).Scan(&got); err != nil {
		t.Fatalf("count resource locks: %v", err)
	}
	if got != want {
		t.Fatalf("resource lock count for job=%s resource=%s kind=%s = %d, want %d", jobID, resourceType, lockKind, got, want)
	}
}

func assertPostgresCount(t *testing.T, ctx context.Context, store *Store, query string, want int, args ...any) {
	t.Helper()

	var got int
	if err := store.db.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("count query %q = %d, want %d", query, got, want)
	}
}

func quotePostgresIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
