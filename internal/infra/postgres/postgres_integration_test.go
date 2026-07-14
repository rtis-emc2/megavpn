package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rtis-emc2/megavpn/internal/backhaul"
	"github.com/rtis-emc2/megavpn/internal/domain"
	"github.com/rtis-emc2/megavpn/internal/platform/id"
	"github.com/rtis-emc2/megavpn/internal/secrets"
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
	if len(observations) < 1 || observations[0].Source != "job" || observations[0].RuntimeStatus != "active" {
		t.Fatalf("runtime observations after apply = %#v, want latest active job observation", observations)
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

func TestPostgresIntegrationSeedLocalInventoryUsesActiveNodeNameUniqueness(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	if err := store.SeedLocalInventory(ctx); err != nil {
		t.Fatalf("seed local inventory: %v", err)
	}
	if err := store.SeedLocalInventory(ctx); err != nil {
		t.Fatalf("repeat seed local inventory: %v", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from nodes where name='local' and status <> 'retired'`, 1)

	if _, err := store.db.Exec(ctx, `update nodes set status='retired' where name='local'`); err != nil {
		t.Fatalf("retire local node: %v", err)
	}
	if err := store.SeedLocalInventory(ctx); err != nil {
		t.Fatalf("seed local inventory after retire: %v", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from nodes where name='local' and status <> 'retired'`, 1)
	assertPostgresCount(t, ctx, store, `select count(*) from nodes where name='local'`, 2)
}

func TestPostgresIntegrationDeletedInstanceRuntimeReportsAreIgnored(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-runtime-deleted-node-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.0.11",
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
		Name:         "wg-runtime-deleted-" + suffix,
		Slug:         "wg-runtime-deleted-" + suffix,
		EndpointHost: "198.51.100.11",
		EndpointPort: 51821,
		Spec: map[string]any{
			"config_content": "[Interface]\nAddress = 10.99.1.1/24\nListenPort = 51821\nPrivateKey = integration-test\n",
		},
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	deleted, err := store.ForceDeleteInstance(ctx, instance.ID, instance.Name, "integration stale runtime report")
	if err != nil {
		t.Fatalf("force delete instance: %v", err)
	}
	if deleted.Status != "deleted" {
		t.Fatalf("force-deleted instance status = %q, want deleted", deleted.Status)
	}

	states, err := store.SubmitAgentInstanceRuntimeReports(ctx, node.ID, []domain.AgentInstanceRuntimeReport{{
		InstanceID:     instance.ID,
		ServiceCode:    "wireguard",
		SystemdUnit:    instance.SystemdUnit,
		ConfigHash:     "sha256:stale-after-delete",
		ActiveState:    "active",
		EnabledState:   "enabled",
		ListeningPorts: []map[string]any{{"network": "udp", "state": "UNCONN", "local_address": "0.0.0.0:51821", "port": 51821}},
	}})
	if err != nil {
		t.Fatalf("submit stale runtime report for deleted instance: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("stale runtime report states len = %d, want 0", len(states))
	}
	assertPostgresCount(t, ctx, store, `select count(*) from instance_runtime_states where instance_id=$1`, 0, instance.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from instance_runtime_observations where instance_id=$1`, 0, instance.ID)
}

func TestPostgresIntegrationXrayProvisioningReusesClientUUIDAcrossInstances(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-xray-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.10",
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
		ServiceCode:  "xray-core",
		Name:         "it-xray-primary-" + suffix,
		Slug:         "it-xray-primary-" + suffix,
		EndpointHost: "portal1.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("portal1.example.test"),
	})
	if err != nil {
		t.Fatalf("create first xray instance: %v", err)
	}
	second, err := store.CreateInstanceDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "it-xray-secondary-" + suffix,
		Slug:         "it-xray-secondary-" + suffix,
		EndpointHost: "portal2.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("portal2.example.test"),
	})
	if err != nil {
		t.Fatalf("create second xray instance: %v", err)
	}
	client, err := store.CreateClient(ctx, domain.Client{
		Username:    "it-xray-client-" + suffix,
		DisplayName: "Xray Shared Identity Client",
		Email:       "it-xray-client-" + suffix + "@example.invalid",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	if _, err := store.ProvisionClientWithOptions(ctx, client.ID, []string{first.ID}, map[string]map[string]any{
		first.ID: {"vless_group": "out_usa_sf"},
	}); err != nil {
		t.Fatalf("provision first xray access: %v", err)
	}
	firstAccess := xrayServiceAccessByInstance(t, ctx, store, client.ID, first.ID)
	firstMetadata, err := store.EnsureXrayServiceAccessUUID(ctx, firstAccess.ID)
	if err != nil {
		t.Fatalf("ensure first xray uuid: %v", err)
	}
	firstUUID := firstString(firstMetadata["xray_uuid"])
	if firstUUID == "" {
		t.Fatal("first xray uuid must be generated")
	}

	if _, err := store.ProvisionClientWithOptions(ctx, client.ID, []string{second.ID}, map[string]map[string]any{
		second.ID: {"vless_group": "default"},
	}); err != nil {
		t.Fatalf("provision second xray access: %v", err)
	}
	secondAccess := xrayServiceAccessByInstance(t, ctx, store, client.ID, second.ID)
	secondUUID := firstString(secondAccess.Metadata["xray_uuid"])
	if secondUUID != firstUUID {
		t.Fatalf("second xray uuid = %q, want reused client uuid %q", secondUUID, firstUUID)
	}
	secondMetadata, err := store.EnsureXrayServiceAccessUUID(ctx, secondAccess.ID)
	if err != nil {
		t.Fatalf("ensure second xray uuid: %v", err)
	}
	if got := firstString(secondMetadata["xray_uuid"]); got != firstUUID {
		t.Fatalf("second ensured xray uuid = %q, want %q", got, firstUUID)
	}

	rotateJob, err := store.RotateServiceAccess(ctx, client.ID, firstAccess.ID, "xray-core")
	if err != nil {
		t.Fatalf("rotate first xray uuid: %v", err)
	}
	queuedInstanceIDs := map[string]bool{}
	for _, raw := range stringSliceFromAny(rotateJob.Payload["instance_ids"]) {
		queuedInstanceIDs[raw] = true
	}
	if !queuedInstanceIDs[first.ID] || !queuedInstanceIDs[second.ID] {
		t.Fatalf("rotation job instance_ids = %#v, want both xray instances %s and %s", rotateJob.Payload["instance_ids"], first.ID, second.ID)
	}
	rotatedMetadata, err := store.EnsureXrayServiceAccessUUID(ctx, firstAccess.ID)
	if err != nil {
		t.Fatalf("ensure rotated xray uuid: %v", err)
	}
	rotatedUUID := firstString(rotatedMetadata["xray_uuid"])
	if rotatedUUID == "" || rotatedUUID == firstUUID {
		t.Fatalf("rotated xray uuid = %q, want a new uuid different from %q", rotatedUUID, firstUUID)
	}
	if got := firstString(rotatedMetadata["vless_group"], rotatedMetadata["xray_group"], rotatedMetadata["outbound_group"]); got != "out_usa_sf" {
		t.Fatalf("rotated xray group = %q, want preserved out_usa_sf", got)
	}
	if xrayServiceAccessUUIDRotationRequested(rotatedMetadata) {
		t.Fatalf("rotation marker was not cleared: %#v", rotatedMetadata)
	}
	secondAfterRotate := xrayServiceAccessByInstance(t, ctx, store, client.ID, second.ID)
	if got := firstString(secondAfterRotate.Metadata["xray_uuid"]); got != rotatedUUID {
		t.Fatalf("second xray uuid after rotation = %q, want propagated uuid %q", got, rotatedUUID)
	}
	if got := firstString(secondAfterRotate.Metadata["vless_group"], secondAfterRotate.Metadata["xray_group"], secondAfterRotate.Metadata["outbound_group"]); got != "default" {
		t.Fatalf("second xray group after rotation = %q, want preserved default", got)
	}
	if xrayServiceAccessUUIDRotationRequested(secondAfterRotate.Metadata) {
		t.Fatalf("second rotation marker was not cleared: %#v", secondAfterRotate.Metadata)
	}
}

func TestPostgresIntegrationXrayClientIdentitySurvivesServiceAccessDeletion(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-xray-replace-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.20",
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
		ServiceCode:  "xray-core",
		Name:         "it-xray-old-" + suffix,
		Slug:         "it-xray-old-" + suffix,
		EndpointHost: "portal.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("portal.example.test"),
	})
	if err != nil {
		t.Fatalf("create first xray instance: %v", err)
	}
	client, err := store.CreateClient(ctx, domain.Client{
		Username:    "it-xray-replace-client-" + suffix,
		DisplayName: "Xray Replace Client",
		Email:       "it-xray-replace-client-" + suffix + "@example.invalid",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := store.ProvisionClientWithOptions(ctx, client.ID, []string{first.ID}, map[string]map[string]any{
		first.ID: {"vless_group": "out_usa_sf"},
	}); err != nil {
		t.Fatalf("provision first xray access: %v", err)
	}
	firstAccess := xrayServiceAccessByInstance(t, ctx, store, client.ID, first.ID)
	firstMetadata, err := store.EnsureXrayServiceAccessUUID(ctx, firstAccess.ID)
	if err != nil {
		t.Fatalf("ensure first xray uuid: %v", err)
	}
	firstUUID := firstString(firstMetadata["xray_uuid"])
	if firstUUID == "" {
		t.Fatal("first xray uuid must be generated")
	}
	assertPostgresCount(t, ctx, store, `select count(*) from client_service_identities where client_account_id=$1 and credential_json->>'xray_uuid'=$2`, 1, client.ID, firstUUID)

	if _, err := store.DeleteClientServiceAccess(ctx, client.ID, firstAccess.ID); err != nil {
		t.Fatalf("delete old service access: %v", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where id=$1`, 0, firstAccess.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from client_service_identities where client_account_id=$1 and credential_json->>'xray_uuid'=$2`, 1, client.ID, firstUUID)

	replacement, err := store.CreateInstanceDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "it-xray-new-" + suffix,
		Slug:         "it-xray-new-" + suffix,
		EndpointHost: "portal.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("portal.example.test"),
	})
	if err != nil {
		t.Fatalf("create replacement xray instance: %v", err)
	}
	if _, err := store.ProvisionClientWithOptions(ctx, client.ID, []string{replacement.ID}, map[string]map[string]any{
		replacement.ID: {"vless_group": "out_usa_sf"},
	}); err != nil {
		t.Fatalf("provision replacement xray access: %v", err)
	}
	replacementAccess := xrayServiceAccessByInstance(t, ctx, store, client.ID, replacement.ID)
	if got := firstString(replacementAccess.Metadata["xray_uuid"]); got != firstUUID {
		t.Fatalf("replacement access xray uuid = %q, want retained uuid %q", got, firstUUID)
	}
	replacementMetadata, err := store.EnsureXrayServiceAccessUUID(ctx, replacementAccess.ID)
	if err != nil {
		t.Fatalf("ensure replacement xray uuid: %v", err)
	}
	if got := firstString(replacementMetadata["xray_uuid"]); got != firstUUID {
		t.Fatalf("replacement ensured xray uuid = %q, want retained uuid %q", got, firstUUID)
	}
}

func TestPostgresIntegrationInstanceVLESSGroupMembershipBulk(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-vless-members-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.30",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	instance, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "it-vless-members-" + suffix,
		Slug:         "it-vless-members-" + suffix,
		EndpointHost: "members.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("members.example.test"),
	})
	if err != nil {
		t.Fatalf("create xray instance: %v", err)
	}
	first, err := store.CreateClient(ctx, domain.Client{Username: "it-vless-member-a-" + suffix, Email: "member-a-" + suffix + "@example.invalid"})
	if err != nil {
		t.Fatalf("create first client: %v", err)
	}
	second, err := store.CreateClient(ctx, domain.Client{Username: "it-vless-member-b-" + suffix, Email: "member-b-" + suffix + "@example.invalid"})
	if err != nil {
		t.Fatalf("create second client: %v", err)
	}

	added, err := store.AddInstanceVLESSGroupMembers(ctx, instance.ID, "out_usa_sf", domain.VLESSGroupMembershipRequest{
		ClientIDs:  []string{first.ID, second.ID},
		Mode:       "add_or_move",
		QueueApply: true,
	})
	if err != nil {
		t.Fatalf("bulk add: %v", err)
	}
	if added.Created != 2 || added.Updated != 0 || added.Skipped != 0 || len(added.Failed) != 0 {
		t.Fatalf("bulk add result = %#v, want 2 created", added)
	}
	if added.ApplyJobID == "" {
		t.Fatalf("bulk add did not queue a bounded instance apply job: %#v", added)
	}
	overview, err := store.ListInstanceVLESSGroupMembers(ctx, instance.ID)
	if err != nil {
		t.Fatalf("list overview: %v", err)
	}
	if got := vlessGroupMemberCount(overview, "out_usa_sf"); got != 2 {
		t.Fatalf("out_usa_sf member count = %d, want 2", got)
	}
	firstAccess := xrayServiceAccessByInstance(t, ctx, store, first.ID, instance.ID)
	firstMetadata, err := store.EnsureXrayServiceAccessUUID(ctx, firstAccess.ID)
	if err != nil {
		t.Fatalf("ensure first uuid: %v", err)
	}
	firstUUID := firstString(firstMetadata["xray_uuid"])
	if firstUUID == "" {
		t.Fatal("first uuid is empty")
	}

	repeated, err := store.AddInstanceVLESSGroupMembers(ctx, instance.ID, "out_usa_sf", domain.VLESSGroupMembershipRequest{
		ClientIDs: []string{first.ID, second.ID},
		Mode:      "add_or_move",
	})
	if err != nil {
		t.Fatalf("repeat add: %v", err)
	}
	if repeated.Created != 0 || repeated.Updated != 0 || repeated.Skipped != 2 {
		t.Fatalf("repeat add result = %#v, want 2 skipped", repeated)
	}
	if repeated.ApplyJobID != "" {
		t.Fatalf("repeat idempotent add queued apply job %s", repeated.ApplyJobID)
	}

	unresolved, err := store.AddInstanceVLESSGroupMembers(ctx, instance.ID, "out_usa_sf", domain.VLESSGroupMembershipRequest{
		ClientRefs: []string{"missing-" + suffix + "@example.invalid"},
		Mode:       "add_or_move",
	})
	if err != nil {
		t.Fatalf("unresolved refs should return structured failures, not transport error: %v", err)
	}
	if len(unresolved.Failed) != 1 || unresolved.Created != 0 || unresolved.Updated != 0 || unresolved.ApplyJobID != "" {
		t.Fatalf("unresolved refs result = %#v, want one failed ref and no apply", unresolved)
	}

	moved, err := store.MoveInstanceVLESSGroupMember(ctx, instance.ID, first.ID, "default")
	if err != nil {
		t.Fatalf("move member: %v", err)
	}
	if moved.Updated != 1 {
		t.Fatalf("move result = %#v, want one updated", moved)
	}
	movedAccess := xrayServiceAccessByInstance(t, ctx, store, first.ID, instance.ID)
	movedMetadata, err := store.EnsureXrayServiceAccessUUID(ctx, movedAccess.ID)
	if err != nil {
		t.Fatalf("ensure moved uuid: %v", err)
	}
	if got := firstString(movedMetadata["xray_uuid"]); got != firstUUID {
		t.Fatalf("moved uuid = %q, want preserved %q", got, firstUUID)
	}
	if got := firstString(movedMetadata["vless_group"], movedMetadata["xray_group"], movedMetadata["outbound_group"]); got != "default" {
		t.Fatalf("moved group = %q, want default", got)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where client_account_id=$1 and instance_id=$2`, 1, first.ID, instance.ID)
}

func TestPostgresIntegrationGlobalVLESSGroupMembershipMaterializesAllInstances(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	nodeA, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-vless-global-node-a-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.130",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node a: %v", err)
	}
	nodeB, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-vless-global-node-b-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.131",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node b: %v", err)
	}
	instanceA, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       nodeA.ID,
		ServiceCode:  "xray-core",
		Name:         "it-vless-global-a-" + suffix,
		Slug:         "it-vless-global-a-" + suffix,
		EndpointHost: "global-a.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("global-a.example.test"),
	})
	if err != nil {
		t.Fatalf("create xray instance a: %v", err)
	}
	instanceB, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       nodeB.ID,
		ServiceCode:  "xray-core",
		Name:         "it-vless-global-b-" + suffix,
		Slug:         "it-vless-global-b-" + suffix,
		EndpointHost: "global-b.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("global-b.example.test"),
	})
	if err != nil {
		t.Fatalf("create xray instance b: %v", err)
	}
	client, err := store.CreateClient(ctx, domain.Client{Username: "it-vless-global-client-" + suffix, Email: "global-" + suffix + "@example.invalid"})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	result, err := store.AddVLESSGroupMembers(ctx, "out_usa_sf", domain.VLESSGroupMembershipRequest{
		ClientIDs:  []string{client.ID},
		Mode:       "add_or_move",
		QueueApply: true,
	})
	if err != nil {
		t.Fatalf("global add: %v", err)
	}
	if result.Created != 1 || result.Updated != 0 || result.Skipped != 0 || len(result.Failed) != 0 {
		t.Fatalf("global add result = %#v, want one desired membership created", result)
	}
	if result.AffectedInstances < 2 || result.Materialized < 2 || result.ApplyJobCount < 2 {
		t.Fatalf("global add projection = %#v, want materialized/apply across both instances", result)
	}
	assertPostgresCount(t, ctx, store, `select count(*)
		from client_access_group_memberships m
		join client_access_groups g on g.id=m.group_id
		where m.client_account_id=$1
		  and g.service_code='vless'
		  and g.group_key='out_usa_sf'
		  and m.status='active'`, 1, client.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where client_account_id=$1 and instance_id in ($2,$3)`, 2, client.ID, instanceA.ID, instanceB.ID)

	accessA := xrayServiceAccessByInstance(t, ctx, store, client.ID, instanceA.ID)
	metaA, err := store.EnsureXrayServiceAccessUUID(ctx, accessA.ID)
	if err != nil {
		t.Fatalf("ensure uuid a: %v", err)
	}
	uuidA := firstString(metaA["xray_uuid"])
	if uuidA == "" {
		t.Fatal("instance a uuid is empty")
	}

	moved, err := store.AddVLESSGroupMembers(ctx, "default", domain.VLESSGroupMembershipRequest{
		ClientIDs: []string{client.ID},
		Mode:      "add_or_move",
	})
	if err != nil {
		t.Fatalf("global move: %v", err)
	}
	if moved.Created != 0 || moved.Updated != 1 || moved.Skipped != 0 {
		t.Fatalf("global move result = %#v, want one desired membership moved", moved)
	}
	movedAccessA := xrayServiceAccessByInstance(t, ctx, store, client.ID, instanceA.ID)
	movedMetaA, err := store.EnsureXrayServiceAccessUUID(ctx, movedAccessA.ID)
	if err != nil {
		t.Fatalf("ensure moved uuid a: %v", err)
	}
	if got := firstString(movedMetaA["xray_uuid"]); got != uuidA {
		t.Fatalf("moved uuid = %q, want preserved %q", got, uuidA)
	}
	if got := firstString(movedMetaA["vless_group"], movedMetaA["xray_group"], movedMetaA["outbound_group"]); got != "default" {
		t.Fatalf("moved group = %q, want default", got)
	}

	nodeC, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-vless-global-node-c-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.132",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node c: %v", err)
	}
	instanceC, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       nodeC.ID,
		ServiceCode:  "xray-core",
		Name:         "it-vless-global-c-" + suffix,
		Slug:         "it-vless-global-c-" + suffix,
		EndpointHost: "global-c.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("global-c.example.test"),
	})
	if err != nil {
		t.Fatalf("create xray instance c: %v", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where client_account_id=$1 and instance_id=$2`, 1, client.ID, instanceC.ID)
	accessC := xrayServiceAccessByInstance(t, ctx, store, client.ID, instanceC.ID)
	if got := firstString(accessC.Metadata["vless_group"], accessC.Metadata["xray_group"], accessC.Metadata["outbound_group"]); got != "default" {
		t.Fatalf("new instance materialized group = %q, want default", got)
	}
}

func TestPostgresIntegrationInstanceVLESSGroupMembershipRejectsUnknownGroup(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-vless-unknown-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.31",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	instance, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "it-vless-unknown-" + suffix,
		Slug:         "it-vless-unknown-" + suffix,
		EndpointHost: "unknown.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("unknown.example.test"),
	})
	if err != nil {
		t.Fatalf("create xray instance: %v", err)
	}
	client, err := store.CreateClient(ctx, domain.Client{Username: "it-vless-unknown-client-" + suffix})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	_, err = store.AddInstanceVLESSGroupMembers(ctx, instance.ID, "route", domain.VLESSGroupMembershipRequest{ClientIDs: []string{client.ID}})
	if err == nil {
		t.Fatal("unknown group add succeeded, want error")
	}
	if !strings.Contains(err.Error(), "available groups:") || !strings.Contains(err.Error(), "out_usa_sf") {
		t.Fatalf("unknown group error = %q, want available group keys", err.Error())
	}
}

func TestPostgresIntegrationInstanceVLESSGroupMembershipBoundedJobsForLargeBatch(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-vless-bulk-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.32",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	instance, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "it-vless-bulk-" + suffix,
		Slug:         "it-vless-bulk-" + suffix,
		EndpointHost: "bulk.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("bulk.example.test"),
	})
	if err != nil {
		t.Fatalf("create xray instance: %v", err)
	}
	clientIDs := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		client, err := store.CreateClient(ctx, domain.Client{Username: fmt.Sprintf("it-vless-bulk-%s-%04d", suffix, i)})
		if err != nil {
			t.Fatalf("create client %d: %v", i, err)
		}
		clientIDs = append(clientIDs, client.ID)
	}

	var before int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&before); err != nil {
		t.Fatalf("count jobs before: %v", err)
	}
	result, err := store.AddInstanceVLESSGroupMembers(ctx, instance.ID, "out_usa_sf", domain.VLESSGroupMembershipRequest{
		ClientIDs:  clientIDs,
		Mode:       "add_or_move",
		QueueApply: true,
	})
	if err != nil {
		t.Fatalf("bulk add 1000: %v", err)
	}
	if result.Created != 1000 || result.Updated != 0 || len(result.Failed) != 0 {
		t.Fatalf("bulk 1000 result = %#v, want 1000 created", result)
	}
	if result.ApplyJobCount != 1 {
		t.Fatalf("bulk 1000 apply job count = %d, want 1", result.ApplyJobCount)
	}
	var after int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&after); err != nil {
		t.Fatalf("count jobs after: %v", err)
	}
	if got := after - before; got > 2 {
		t.Fatalf("bulk 1000 queued %d jobs, want bounded <= 2", got)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where instance_id=$1`, 1000, instance.ID)
}

func TestPostgresIntegrationInstanceVLESSGroupMembershipModes(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-vless-mode-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.33",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	instance, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "it-vless-mode-" + suffix,
		Slug:         "it-vless-mode-" + suffix,
		EndpointHost: "mode.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("mode.example.test"),
	})
	if err != nil {
		t.Fatalf("create xray instance: %v", err)
	}
	client, err := store.CreateClient(ctx, domain.Client{Username: "it-vless-mode-client-" + suffix})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	initial, err := store.AddInstanceVLESSGroupMembers(ctx, instance.ID, "default", domain.VLESSGroupMembershipRequest{
		ClientIDs: []string{client.ID},
		Mode:      "add_or_move",
	})
	if err != nil {
		t.Fatalf("initial add: %v", err)
	}
	if initial.Created != 1 {
		t.Fatalf("initial result = %#v, want one created", initial)
	}
	access := xrayServiceAccessByInstance(t, ctx, store, client.ID, instance.ID)
	metadata, err := store.EnsureXrayServiceAccessUUID(ctx, access.ID)
	if err != nil {
		t.Fatalf("ensure uuid: %v", err)
	}
	uuid := firstString(metadata["xray_uuid"])
	if uuid == "" {
		t.Fatal("initial uuid is empty")
	}

	addOnly, err := store.AddInstanceVLESSGroupMembers(ctx, instance.ID, "out_usa_sf", domain.VLESSGroupMembershipRequest{
		ClientIDs: []string{client.ID},
		Mode:      "add_only",
	})
	if err != nil {
		t.Fatalf("add_only: %v", err)
	}
	if addOnly.Created != 0 || addOnly.Updated != 0 || addOnly.Skipped != 1 {
		t.Fatalf("add_only result = %#v, want one skipped", addOnly)
	}
	afterAddOnly := xrayServiceAccessByInstance(t, ctx, store, client.ID, instance.ID)
	if got := firstString(afterAddOnly.Metadata["vless_group"], afterAddOnly.Metadata["xray_group"], afterAddOnly.Metadata["outbound_group"]); got != "default" {
		t.Fatalf("add_only moved group to %q, want default", got)
	}

	moved, err := store.AddInstanceVLESSGroupMembers(ctx, instance.ID, "out_usa_sf", domain.VLESSGroupMembershipRequest{
		ClientIDs: []string{client.ID},
		Mode:      "add_or_move",
	})
	if err != nil {
		t.Fatalf("add_or_move: %v", err)
	}
	if moved.Created != 0 || moved.Updated != 1 || moved.Skipped != 0 {
		t.Fatalf("add_or_move result = %#v, want one moved", moved)
	}
	afterMove := xrayServiceAccessByInstance(t, ctx, store, client.ID, instance.ID)
	afterMoveMetadata, err := store.EnsureXrayServiceAccessUUID(ctx, afterMove.ID)
	if err != nil {
		t.Fatalf("ensure moved uuid: %v", err)
	}
	if got := firstString(afterMoveMetadata["xray_uuid"]); got != uuid {
		t.Fatalf("moved uuid = %q, want preserved %q", got, uuid)
	}
	if got := firstString(afterMoveMetadata["vless_group"], afterMoveMetadata["xray_group"], afterMoveMetadata["outbound_group"]); got != "out_usa_sf" {
		t.Fatalf("moved group = %q, want out_usa_sf", got)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where client_account_id=$1 and instance_id=$2`, 1, client.ID, instance.ID)
}

func TestPostgresIntegrationInstanceVLESSGroupMembershipAllFilteredDryRun(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-vless-filter-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.34",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	instance, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "it-vless-filter-" + suffix,
		Slug:         "it-vless-filter-" + suffix,
		EndpointHost: "filter.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("filter.example.test"),
	})
	if err != nil {
		t.Fatalf("create xray instance: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := store.CreateClient(ctx, domain.Client{
			Username: fmt.Sprintf("it-vless-filter-%s-%02d", suffix, i),
			Email:    fmt.Sprintf("filter-%s-%02d@example.invalid", suffix, i),
		}); err != nil {
			t.Fatalf("create filtered client %d: %v", i, err)
		}
	}
	if _, err := store.CreateClient(ctx, domain.Client{Username: "it-vless-other-" + suffix}); err != nil {
		t.Fatalf("create unrelated client: %v", err)
	}
	preview, err := store.AddInstanceVLESSGroupMembers(ctx, instance.ID, "out_usa_sf", domain.VLESSGroupMembershipRequest{
		DryRun:           true,
		AllFiltered:      true,
		FilterSearch:     "it-vless-filter-" + suffix,
		FilterAssignment: "unassigned",
		QueueApply:       true,
		Mode:             "add_or_move",
	})
	if err != nil {
		t.Fatalf("all filtered preview: %v", err)
	}
	if !preview.DryRun || !preview.AllFiltered || preview.Created != 5 || preview.Updated != 0 || preview.Skipped != 0 || len(preview.Failed) != 0 {
		t.Fatalf("all filtered preview = %#v, want dry-run with 5 creates", preview)
	}
	if preview.ApplyJobCount != 1 || preview.ApplyJobID != "" {
		t.Fatalf("all filtered preview apply fields = count %d id %q, want one projected job and no real job id", preview.ApplyJobCount, preview.ApplyJobID)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where instance_id=$1`, 0, instance.ID)
}

func TestPostgresIntegrationInstanceVLESSGroupMembershipRejectsInactiveGroup(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-vless-inactive-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.35",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	spec := xraySharedClientIdentityTestSpec("inactive.example.test")
	spec["vless_groups"] = append(spec["vless_groups"].([]any),
		map[string]any{"key": "disabled_group", "label": "Disabled group", "egress_mode": "default", "outbound_tag": "direct", "status": "disabled"},
		map[string]any{"key": "deleted_group", "label": "Deleted group", "egress_mode": "default", "outbound_tag": "direct", "status": "deleted"},
	)
	instance, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       node.ID,
		ServiceCode:  "xray-core",
		Name:         "it-vless-inactive-" + suffix,
		Slug:         "it-vless-inactive-" + suffix,
		EndpointHost: "inactive.example.test",
		EndpointPort: 7080,
		Spec:         spec,
	})
	if err != nil {
		t.Fatalf("create xray instance: %v", err)
	}
	client, err := store.CreateClient(ctx, domain.Client{Username: "it-vless-inactive-client-" + suffix})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	for _, groupKey := range []string{"disabled_group", "deleted_group"} {
		_, err = store.AddInstanceVLESSGroupMembers(ctx, instance.ID, groupKey, domain.VLESSGroupMembershipRequest{
			ClientIDs: []string{client.ID},
			Mode:      "add_or_move",
		})
		if err == nil {
			t.Fatalf("inactive group %s accepted, want error", groupKey)
		}
		if !strings.Contains(err.Error(), "not available") {
			t.Fatalf("inactive group %s error = %q, want not available", groupKey, err.Error())
		}
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where client_account_id=$1 and instance_id=$2`, 0, client.ID, instance.ID)
}

func TestPostgresIntegrationClientAccessGroupsVLESSBulkMaterialization(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	nodeA, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-access-group-node-a-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.140",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node a: %v", err)
	}
	nodeB, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-access-group-node-b-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.141",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node b: %v", err)
	}
	instanceA, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       nodeA.ID,
		ServiceCode:  "xray-core",
		Name:         "it-access-group-a-" + suffix,
		Slug:         "it-access-group-a-" + suffix,
		EndpointHost: "access-group-a.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("access-group-a.example.test"),
	})
	if err != nil {
		t.Fatalf("create xray instance a: %v", err)
	}
	instanceB, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       nodeB.ID,
		ServiceCode:  "xray-core",
		Name:         "it-access-group-b-" + suffix,
		Slug:         "it-access-group-b-" + suffix,
		EndpointHost: "access-group-b.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("access-group-b.example.test"),
	})
	if err != nil {
		t.Fatalf("create xray instance b: %v", err)
	}
	outUSASF := clientAccessGroupByKey(t, ctx, store, "vless", "out_usa_sf")
	defaultGroup := clientAccessGroupByKey(t, ctx, store, "vless", "default")

	clientIDs := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		client, err := store.CreateClient(ctx, domain.Client{Username: fmt.Sprintf("it-access-group-%s-%04d", suffix, i)})
		if err != nil {
			t.Fatalf("create client %d: %v", i, err)
		}
		clientIDs = append(clientIDs, client.ID)
	}

	var jobsBefore int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsBefore); err != nil {
		t.Fatalf("count jobs before: %v", err)
	}
	result, err := store.BulkMoveClientAccessGroupMembers(ctx, outUSASF.ID, domain.ClientAccessGroupMembershipRequest{
		ClientIDs:  clientIDs,
		Mode:       "add_or_move",
		QueueApply: true,
	})
	if err != nil {
		t.Fatalf("bulk move access group: %v", err)
	}
	if result.CreatedMemberships != 1000 || result.MovedMemberships != 0 || result.SkippedExisting != 0 || len(result.Failed) != 0 {
		t.Fatalf("bulk access group result = %#v, want 1000 created", result)
	}
	if result.AffectedInstances != 2 || result.ApplyJobCount != 2 {
		t.Fatalf("bulk access group apply = affected:%d jobs:%d, want 2/2", result.AffectedInstances, result.ApplyJobCount)
	}
	var jobsAfter int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsAfter); err != nil {
		t.Fatalf("count jobs after: %v", err)
	}
	if got := jobsAfter - jobsBefore; got > 3 {
		t.Fatalf("bulk 1000 generic access group queued %d jobs, want bounded <= 3", got)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from client_access_group_memberships where group_id=$1 and status='active'`, 1000, outUSASF.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where instance_id in ($1,$2)`, 2000, instanceA.ID, instanceB.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where client_account_id=$1 and instance_id=$2`, 1, clientIDs[0], instanceA.ID)

	accessBeforeMove := xrayServiceAccessByInstance(t, ctx, store, clientIDs[0], instanceA.ID)
	beforeMetadata, err := store.EnsureXrayServiceAccessUUID(ctx, accessBeforeMove.ID)
	if err != nil {
		t.Fatalf("ensure uuid before move: %v", err)
	}
	stableUUID := firstString(beforeMetadata["xray_uuid"])
	if stableUUID == "" {
		t.Fatal("stable uuid is empty")
	}

	var jobsBeforeScope int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsBeforeScope); err != nil {
		t.Fatalf("count jobs before scope: %v", err)
	}
	scope, err := store.UpdateClientAccessGroupScope(ctx, outUSASF.ID, domain.ClientAccessGroupScope{
		ScopeMode:             "selected_instances",
		AutoApplyNewInstances: true,
		IncludeInstanceIDs:    []string{instanceA.ID},
	}, nil)
	if err != nil {
		t.Fatalf("update access group scope: %v", err)
	}
	if scope.AffectedInstances != 1 || scope.MaterializedDisabled != 1000 || scope.ApplyJobCount != 2 {
		t.Fatalf("scope update result = %#v, want one affected instance, 1000 disabled projections and two apply jobs", scope)
	}
	var jobsAfterScope int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsAfterScope); err != nil {
		t.Fatalf("count jobs after scope: %v", err)
	}
	if got := jobsAfterScope - jobsBeforeScope; got > 2 {
		t.Fatalf("scope update queued %d jobs, want bounded <= 2", got)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where instance_id=$1 and coalesce(metadata_json->>'access_group_id','')=$2 and status='disabled'`, 1000, instanceB.ID, outUSASF.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where instance_id=$1 and coalesce(metadata_json->>'access_group_id','')=$2 and status in ('pending','active')`, 1000, instanceA.ID, outUSASF.ID)

	var jobsBeforePolicy int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsBeforePolicy); err != nil {
		t.Fatalf("count jobs before policy update: %v", err)
	}
	autoApplyOut := outUSASF.AutoApplyNewInstances
	if _, err := store.UpdateClientAccessGroup(ctx, outUSASF.ID, domain.ClientAccessGroupInput{
		ServiceCode:           outUSASF.ServiceCode,
		GroupKey:              outUSASF.GroupKey,
		DisplayName:           outUSASF.DisplayName,
		Description:           outUSASF.Description,
		Status:                "active",
		PolicyJSON:            []byte(`{"access_mode":"selected_egress","egress_mode":"selected_egress","outbound_tag":"egress-out_usa_sf","rules":[{"type":"field","domain":["geosite:private"],"outbound_tag":"direct"}]}`),
		ScopeMode:             "selected_instances",
		AutoApplyNewInstances: &autoApplyOut,
	}, nil); err != nil {
		t.Fatalf("update access group policy: %v", err)
	}
	var jobsAfterPolicy int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsAfterPolicy); err != nil {
		t.Fatalf("count jobs after policy update: %v", err)
	}
	if got := jobsAfterPolicy - jobsBeforePolicy; got != 1 {
		t.Fatalf("policy update queued %d jobs, want one apply for the scoped instance", got)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from client_access_group_sync_state where group_id=$1 and instance_id=$2 and status='queued'`, 1, outUSASF.ID, instanceA.ID)

	noApplyClient, err := store.CreateClient(ctx, domain.Client{Username: "it-access-group-no-apply-" + suffix})
	if err != nil {
		t.Fatalf("create no-apply client: %v", err)
	}
	var jobsBeforeNoApply int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsBeforeNoApply); err != nil {
		t.Fatalf("count jobs before no-apply: %v", err)
	}
	noApply, err := store.BulkMoveClientAccessGroupMembers(ctx, defaultGroup.ID, domain.ClientAccessGroupMembershipRequest{
		ClientIDs:  []string{noApplyClient.ID},
		Mode:       "add_or_move",
		QueueApply: false,
	})
	if err != nil {
		t.Fatalf("generic queue_apply false: %v", err)
	}
	if noApply.CreatedMemberships != 1 || noApply.ApplyJobCount != 0 || len(noApply.ApplyJobIDs) != 0 || noApply.MaterializedCreated != 2 {
		t.Fatalf("queue_apply false result = %#v, want materialized without apply jobs", noApply)
	}
	var jobsAfterNoApply int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsAfterNoApply); err != nil {
		t.Fatalf("count jobs after no-apply: %v", err)
	}
	if jobsAfterNoApply != jobsBeforeNoApply {
		t.Fatalf("queue_apply false created %d jobs, want 0", jobsAfterNoApply-jobsBeforeNoApply)
	}

	addOnly, err := store.BulkAddClientAccessGroupMembers(ctx, defaultGroup.ID, domain.ClientAccessGroupMembershipRequest{
		ClientIDs: []string{clientIDs[1]},
		Mode:      "add_only",
	})
	if err != nil {
		t.Fatalf("generic add_only: %v", err)
	}
	if addOnly.CreatedMemberships != 0 || addOnly.MovedMemberships != 0 || addOnly.SkippedExisting != 1 {
		t.Fatalf("generic add_only result = %#v, want skipped existing", addOnly)
	}
	if got := activeClientAccessGroupKey(t, ctx, store, clientIDs[1], "vless"); got != "out_usa_sf" {
		t.Fatalf("add_only active group = %q, want out_usa_sf", got)
	}

	moved, err := store.BulkMoveClientAccessGroupMembers(ctx, defaultGroup.ID, domain.ClientAccessGroupMembershipRequest{
		ClientIDs:  []string{clientIDs[0]},
		Mode:       "add_or_move",
		QueueApply: true,
	})
	if err != nil {
		t.Fatalf("generic move: %v", err)
	}
	if moved.CreatedMemberships != 0 || moved.MovedMemberships != 1 || moved.SkippedExisting != 0 {
		t.Fatalf("generic move result = %#v, want one moved", moved)
	}
	if got := activeClientAccessGroupKey(t, ctx, store, clientIDs[0], "vless"); got != "default" {
		t.Fatalf("moved active group = %q, want default", got)
	}
	accessAfterMove := xrayServiceAccessByInstance(t, ctx, store, clientIDs[0], instanceA.ID)
	afterMetadata, err := store.EnsureXrayServiceAccessUUID(ctx, accessAfterMove.ID)
	if err != nil {
		t.Fatalf("ensure uuid after move: %v", err)
	}
	if got := firstString(afterMetadata["xray_uuid"]); got != stableUUID {
		t.Fatalf("moved uuid = %q, want stable %q", got, stableUUID)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where client_account_id=$1 and instance_id=$2`, 1, clientIDs[0], instanceA.ID)

	var jobsBeforeDisable int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsBeforeDisable); err != nil {
		t.Fatalf("count jobs before disable: %v", err)
	}
	if _, err := store.SetClientAccessGroupStatus(ctx, defaultGroup.ID, "disabled", nil); err != nil {
		t.Fatalf("disable default access group: %v", err)
	}
	var jobsAfterDisable int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsAfterDisable); err != nil {
		t.Fatalf("count jobs after disable: %v", err)
	}
	if got := jobsAfterDisable - jobsBeforeDisable; got != 2 {
		t.Fatalf("disable group queued %d jobs, want two apply jobs for materialized instances", got)
	}
	var disabledProjections int
	if err := store.db.QueryRow(ctx, `select count(*)::int from service_accesses where coalesce(metadata_json->>'access_group_id','')=$1 and status='disabled'`, defaultGroup.ID).Scan(&disabledProjections); err != nil {
		t.Fatalf("count disabled default projections: %v", err)
	}
	if disabledProjections == 0 {
		t.Fatal("disable group left no disabled service access projections")
	}

	var jobsBeforeEnable int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsBeforeEnable); err != nil {
		t.Fatalf("count jobs before enable: %v", err)
	}
	if _, err := store.SetClientAccessGroupStatus(ctx, defaultGroup.ID, "active", nil); err != nil {
		t.Fatalf("enable default access group: %v", err)
	}
	var jobsAfterEnable int
	if err := store.db.QueryRow(ctx, `select count(*)::int from jobs`).Scan(&jobsAfterEnable); err != nil {
		t.Fatalf("count jobs after enable: %v", err)
	}
	if got := jobsAfterEnable - jobsBeforeEnable; got != 2 {
		t.Fatalf("enable group queued %d jobs, want two apply jobs for materialized instances", got)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where coalesce(metadata_json->>'access_group_id','')=$1 and status in ('pending','active')`, disabledProjections, defaultGroup.ID)

	nodeC, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-access-group-node-c-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.2.142",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node c: %v", err)
	}
	instanceC, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       nodeC.ID,
		ServiceCode:  "xray-core",
		Name:         "it-access-group-c-" + suffix,
		Slug:         "it-access-group-c-" + suffix,
		EndpointHost: "access-group-c.example.test",
		EndpointPort: 7080,
		Spec:         xraySharedClientIdentityTestSpec("access-group-c.example.test"),
	})
	if err != nil {
		t.Fatalf("create scoped xray instance c: %v", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where instance_id=$1 and coalesce(metadata_json->>'access_group_id','')=$2`, 0, instanceC.ID, outUSASF.ID)

	disabled, err := store.CreateClientAccessGroup(ctx, domain.ClientAccessGroupInput{
		ServiceCode: "vless",
		GroupKey:    "it_disabled_" + suffix,
		DisplayName: "Disabled " + suffix,
		Status:      "disabled",
		PolicyJSON:  []byte(`{}`),
	}, nil)
	if err != nil {
		t.Fatalf("create disabled group: %v", err)
	}
	if _, err := store.BulkMoveClientAccessGroupMembers(ctx, disabled.ID, domain.ClientAccessGroupMembershipRequest{ClientIDs: []string{clientIDs[2]}}); err == nil {
		t.Fatal("disabled client access group accepted membership, want error")
	}
	deleted, err := store.CreateClientAccessGroup(ctx, domain.ClientAccessGroupInput{
		ServiceCode: "vless",
		GroupKey:    "it_deleted_" + suffix,
		DisplayName: "Deleted " + suffix,
		PolicyJSON:  []byte(`{}`),
	}, nil)
	if err != nil {
		t.Fatalf("create deleted group: %v", err)
	}
	if _, err := store.DeleteClientAccessGroupSoft(ctx, deleted.ID, nil); err != nil {
		t.Fatalf("delete group: %v", err)
	}
	if _, err := store.BulkMoveClientAccessGroupMembers(ctx, deleted.ID, domain.ClientAccessGroupMembershipRequest{ClientIDs: []string{clientIDs[2]}}); err == nil {
		t.Fatal("deleted client access group accepted membership, want error")
	}
}

func TestPostgresIntegrationDefaultFirewallBaseline(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	inventory, err := store.FirewallInventory(ctx)
	if err != nil {
		t.Fatalf("firewall inventory: %v", err)
	}
	secondInventory, err := store.FirewallInventory(ctx)
	if err != nil {
		t.Fatalf("second firewall inventory: %v", err)
	}

	var nodeBase domain.FirewallPolicy
	for _, policy := range inventory.Policies {
		if policy.Key == "node_base" {
			nodeBase = policy
			break
		}
	}
	if nodeBase.ID == "" {
		t.Fatal("node_base firewall policy was not seeded")
	}
	if nodeBase.Label != "Default node firewall" {
		t.Fatalf("node_base label = %q, want Default node firewall", nodeBase.Label)
	}
	if nodeBase.DefaultInputPolicy != "drop" || nodeBase.DefaultForwardPolicy != "drop" || nodeBase.DefaultOutputPolicy != "accept" {
		t.Fatalf("node_base defaults = input:%s forward:%s output:%s, want drop/drop/accept",
			nodeBase.DefaultInputPolicy, nodeBase.DefaultForwardPolicy, nodeBase.DefaultOutputPolicy)
	}

	baselineRules := map[string]domain.FirewallRule{}
	for _, rule := range inventory.Rules {
		if rule.PolicyID != nodeBase.ID {
			continue
		}
		key, _ := rule.Metadata["baseline_key"].(string)
		if key != "" {
			baselineRules[key] = rule
		}
	}
	for _, key := range []string{
		"drop_invalid_input",
		"drop_invalid_forward",
		"allow_icmp_v4",
		"allow_icmp_v6",
		"allow_edge_http_https",
		"allow_ssh_trusted_operators",
		"allow_vpn_client_forward",
	} {
		if _, ok := baselineRules[key]; !ok {
			t.Fatalf("baseline firewall rule %q was not seeded; got keys %#v", key, baselineRules)
		}
	}
	if baselineRules["allow_icmp_v6"].Protocol != "icmpv6" {
		t.Fatalf("allow_icmp_v6 protocol = %q, want icmpv6", baselineRules["allow_icmp_v6"].Protocol)
	}
	if baselineRules["allow_ssh_trusted_operators"].Enabled {
		t.Fatal("trusted SSH baseline rule must be disabled until trusted_operators is populated")
	}

	vpnListID := ""
	for _, list := range inventory.AddressLists {
		if list.Key == "vpn_client_sources" {
			vpnListID = list.ID
			break
		}
	}
	if vpnListID == "" {
		t.Fatal("vpn_client_sources address list was not seeded")
	}
	values := map[string]bool{}
	for _, entry := range inventory.Entries {
		if entry.ListID == vpnListID {
			values[entry.Value] = true
		}
	}
	for _, value := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10", "fd00::/8"} {
		if !values[value] {
			t.Fatalf("vpn_client_sources entry %q missing; got %#v", value, values)
		}
	}
	if len(secondInventory.Rules) != len(inventory.Rules) || len(secondInventory.Entries) != len(inventory.Entries) {
		t.Fatalf("firewall seed is not idempotent: rules %d -> %d, entries %d -> %d",
			len(inventory.Rules), len(secondInventory.Rules), len(inventory.Entries), len(secondInventory.Entries))
	}

	if _, err := store.UpdateFirewallAddressList(ctx, vpnListID, domain.FirewallAddressList{Status: "disabled"}); err != nil {
		t.Fatalf("disable vpn_client_sources: %v", err)
	}
	disabledInventory, err := store.FirewallInventory(ctx)
	if err != nil {
		t.Fatalf("firewall inventory after disabled address list: %v", err)
	}
	preservedDisabled := false
	for _, list := range disabledInventory.AddressLists {
		if list.ID == vpnListID && list.Status == "disabled" {
			preservedDisabled = true
			break
		}
	}
	if !preservedDisabled {
		t.Fatal("firewall seed must preserve operator-disabled address-list status")
	}
}

func TestPostgresIntegrationFirewallApplyCreatesRevisionJobAndNodeState(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-fw-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.8.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	inventory, err := store.FirewallInventory(ctx)
	if err != nil {
		t.Fatalf("firewall inventory: %v", err)
	}
	var nodeBase domain.FirewallPolicy
	for _, policy := range inventory.Policies {
		if policy.Key == "node_base" {
			nodeBase = policy
			break
		}
	}
	if nodeBase.ID == "" {
		t.Fatal("node_base firewall policy was not seeded")
	}
	if err := store.SeedFirewallManagementCIDRs(ctx, []string{"10.50.0.10/32"}, nil); err != nil {
		t.Fatalf("seed firewall management CIDRs: %v", err)
	}

	previewJob, err := store.CreateFirewallPreviewJob(ctx, node.ID, nodeBase.ID, true)
	if err != nil {
		t.Fatalf("create firewall preview job by policy UUID: %v", err)
	}
	if err := store.CompleteJob(ctx, previewJob.ID, "succeeded", map[string]any{"rendered_hash": "sha256:test-firewall-preview"}); err != nil {
		t.Fatalf("complete firewall preview job: %v", err)
	}
	job, err := store.CreateFirewallApplyJob(ctx, node.ID, nodeBase.ID, true)
	if err != nil {
		t.Fatalf("create firewall apply job by policy UUID: %v", err)
	}
	if job.Type != "node.firewall.apply" || job.NodeID == nil || *job.NodeID != node.ID {
		t.Fatalf("firewall job = %#v, want node.firewall.apply for node %s", job, node.ID)
	}
	if got := firstString(job.Payload["policy_id"]); got != nodeBase.ID {
		t.Fatalf("firewall job policy_id = %q, want %s", got, nodeBase.ID)
	}
	if got, ok := job.Payload["enforce_default_policy"].(bool); !ok || !got {
		t.Fatalf("firewall job enforce_default_policy = %#v, want true", job.Payload["enforce_default_policy"])
	}

	var revisionID, desiredRevisionID, status, lastJobID string
	if err := store.db.QueryRow(ctx, `select coalesce(revision_id::text,''),coalesce(desired_revision_id::text,''),status,coalesce(last_job_id::text,'') from firewall_node_state where node_id=$1`, node.ID).
		Scan(&revisionID, &desiredRevisionID, &status, &lastJobID); err != nil {
		t.Fatalf("read firewall node state: %v", err)
	}
	if revisionID != "" || desiredRevisionID == "" || status != "pending" || lastJobID != job.ID {
		t.Fatalf("firewall node state = revision:%q desired:%q status:%q last_job:%q, want pending desired revision for job %s",
			revisionID, desiredRevisionID, status, lastJobID, job.ID)
	}

	if err := store.CompleteJob(ctx, job.ID, "succeeded", map[string]any{"rendered_hash": "sha256:test-firewall"}); err != nil {
		t.Fatalf("complete firewall apply job: %v", err)
	}
	var appliedRevisionID string
	if err := store.db.QueryRow(ctx, `select coalesce(revision_id::text,''),status from firewall_node_state where node_id=$1`, node.ID).
		Scan(&appliedRevisionID, &status); err != nil {
		t.Fatalf("read applied firewall node state: %v", err)
	}
	if appliedRevisionID != desiredRevisionID || status != "applied" {
		t.Fatalf("applied firewall node state = revision:%q status:%q, want revision %s applied", appliedRevisionID, status, desiredRevisionID)
	}

	disableJob, err := store.CreateFirewallDisableJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("create firewall disable job: %v", err)
	}
	if disableJob.Type != "node.firewall.disable" || disableJob.NodeID == nil || *disableJob.NodeID != node.ID {
		t.Fatalf("disable firewall job = %#v, want node.firewall.disable for node %s", disableJob, node.ID)
	}
	if err := store.db.QueryRow(ctx, `select coalesce(policy_id::text,''),coalesce(revision_id::text,''),coalesce(desired_revision_id::text,''),status,coalesce(last_job_id::text,'') from firewall_node_state where node_id=$1`, node.ID).
		Scan(&revisionID, &appliedRevisionID, &desiredRevisionID, &status, &lastJobID); err != nil {
		t.Fatalf("read pending disable firewall node state: %v", err)
	}
	if revisionID != "" || appliedRevisionID != "" || desiredRevisionID != "" || status != "pending_disable" || lastJobID != disableJob.ID {
		t.Fatalf("pending disable firewall node state = policy:%q revision:%q desired:%q status:%q last_job:%q, want pending_disable for job %s",
			revisionID, appliedRevisionID, desiredRevisionID, status, lastJobID, disableJob.ID)
	}
	if err := store.CompleteJob(ctx, disableJob.ID, "succeeded", map[string]any{"status": "disabled", "already_disabled": false}); err != nil {
		t.Fatalf("complete firewall disable job: %v", err)
	}
	var observed map[string]any
	if err := store.db.QueryRow(ctx, `select coalesce(policy_id::text,''),coalesce(revision_id::text,''),coalesce(desired_revision_id::text,''),status,observed_json from firewall_node_state where node_id=$1`, node.ID).
		Scan(&revisionID, &appliedRevisionID, &desiredRevisionID, &status, &observed); err != nil {
		t.Fatalf("read disabled firewall node state: %v", err)
	}
	if revisionID != "" || appliedRevisionID != "" || desiredRevisionID != "" || status != "disabled" || stringify(observed["status"]) != "disabled" {
		t.Fatalf("disabled firewall node state = policy:%q revision:%q desired:%q status:%q observed:%#v, want disabled without policy/revision refs",
			revisionID, appliedRevisionID, desiredRevisionID, status, observed)
	}
}

func TestPostgresIntegrationStrictFirewallApplyRequiresMatchingPreview(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-fw-preview-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.8.20",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	inventory, err := store.FirewallInventory(ctx)
	if err != nil {
		t.Fatalf("firewall inventory: %v", err)
	}
	var nodeBase domain.FirewallPolicy
	for _, policy := range inventory.Policies {
		if policy.Key == "node_base" {
			nodeBase = policy
			break
		}
	}
	if nodeBase.ID == "" {
		t.Fatal("node_base firewall policy was not seeded")
	}
	if err := store.SeedFirewallManagementCIDRs(ctx, []string{"10.50.0.10/32"}, nil); err != nil {
		t.Fatalf("seed firewall management CIDRs: %v", err)
	}

	_, err = store.CreateFirewallApplyJob(ctx, node.ID, nodeBase.ID, true)
	if err == nil {
		t.Fatal("strict apply without preview succeeded, want blocked apply")
	}
	if !strings.Contains(err.Error(), "requires a successful Preview") {
		t.Fatalf("strict apply without preview error = %q, want preview gate message", err.Error())
	}
}

func TestPostgresIntegrationStrictFirewallRejectsDNSOnlyAddressGroup(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-fw-dns-list-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.8.30",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	inventory, err := store.FirewallInventory(ctx)
	if err != nil {
		t.Fatalf("firewall inventory: %v", err)
	}
	var nodeBase domain.FirewallPolicy
	for _, policy := range inventory.Policies {
		if policy.Key == "node_base" {
			nodeBase = policy
			break
		}
	}
	if nodeBase.ID == "" {
		t.Fatal("node_base firewall policy was not seeded")
	}
	list, err := store.CreateFirewallAddressList(ctx, domain.FirewallAddressList{
		Key:    "it_dns_only_" + suffix,
		Label:  "Integration DNS-only list " + suffix,
		Scope:  "global",
		Status: "active",
	})
	if err != nil {
		t.Fatalf("create DNS-only address list: %v", err)
	}
	if _, err := store.CreateFirewallAddressEntry(ctx, list.ID, domain.FirewallAddressEntry{
		Value:     "ssh.example.invalid",
		ValueType: "dns",
		Label:     "DNS catalog context only",
		Status:    "active",
	}); err != nil {
		t.Fatalf("create DNS-only address entry: %v", err)
	}
	srcListID := list.ID
	_, err = store.CreateFirewallRule(ctx, nodeBase.ID, domain.FirewallRule{
		Priority:   175,
		Chain:      "input",
		Action:     "accept",
		Direction:  "in",
		Protocol:   "tcp",
		SrcListID:  &srcListID,
		DstPorts:   "22",
		StateMatch: []string{"new", "established"},
		Comment:    "DNS-only list must not satisfy strict nftables input allow.",
		Enabled:    true,
		Status:     "active",
	})
	if err != nil {
		t.Fatalf("create DNS-only source rule: %v", err)
	}

	_, err = store.CreateFirewallPreviewJob(ctx, node.ID, nodeBase.ID, true)
	if err == nil {
		t.Fatal("strict preview with DNS-only source list succeeded, want validation error")
	}
	if !strings.Contains(err.Error(), "DNS-only") || !strings.Contains(err.Error(), list.Key) {
		t.Fatalf("strict DNS-only preview error = %q, want DNS-only address group message", err.Error())
	}
}

func TestPostgresIntegrationStrictFirewallInputRequiresManagementCIDR(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-fw-no-mgmt-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.8.40",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	inventory, err := store.FirewallInventory(ctx)
	if err != nil {
		t.Fatalf("firewall inventory: %v", err)
	}
	var nodeBase domain.FirewallPolicy
	for _, policy := range inventory.Policies {
		if policy.Key == "node_base" {
			nodeBase = policy
			break
		}
	}
	if nodeBase.ID == "" {
		t.Fatal("node_base firewall policy was not seeded")
	}
	_, err = store.CreateFirewallPreviewJob(ctx, node.ID, nodeBase.ID, true)
	if err == nil {
		t.Fatal("strict preview without management CIDR succeeded, want validation error")
	}
	if !strings.Contains(err.Error(), "trusted_control_plane") {
		t.Fatalf("strict input error = %q, want trusted_control_plane guidance", err.Error())
	}
}

func TestPostgresIntegrationStrictFirewallInputWithManagementCIDRQueuesSSHProtectedPreview(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-fw-mgmt-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.8.41",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := store.SeedFirewallManagementCIDRs(ctx, []string{"10.50.0.10/32"}, []string{"10.50.0.11/32"}); err != nil {
		t.Fatalf("seed firewall management CIDRs: %v", err)
	}
	inventory, err := store.FirewallInventory(ctx)
	if err != nil {
		t.Fatalf("firewall inventory: %v", err)
	}
	var nodeBase domain.FirewallPolicy
	for _, policy := range inventory.Policies {
		if policy.Key == "node_base" {
			nodeBase = policy
			break
		}
	}
	if nodeBase.ID == "" {
		t.Fatal("node_base firewall policy was not seeded")
	}
	job, err := store.CreateFirewallPreviewJob(ctx, node.ID, nodeBase.ID, true)
	if err != nil {
		t.Fatalf("strict preview with management CIDR: %v", err)
	}
	if got := firstString(job.Payload["default_input_policy"]); got != "drop" {
		t.Fatalf("preview input policy = %q, want drop", got)
	}
	lists, _ := job.Payload["address_lists"].([]any)
	foundControlPlane := false
	for _, raw := range lists {
		list, _ := raw.(map[string]any)
		if firstString(list["key"]) == "trusted_control_plane" {
			entries, _ := list["entries"].([]any)
			for _, rawEntry := range entries {
				entry, _ := rawEntry.(map[string]any)
				if firstString(entry["value"]) == "10.50.0.10/32" || firstString(entry["value"]) == "10.50.0.11/32" {
					foundControlPlane = true
				}
			}
		}
	}
	if !foundControlPlane {
		t.Fatalf("preview payload did not include managed trusted_control_plane entries: %#v", job.Payload["address_lists"])
	}
}

func TestPostgresIntegrationStrictFirewallRejectsForwardDropWithoutSourceAllow(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-fw-forward-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.8.42",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	policy, err := store.CreateFirewallPolicy(ctx, domain.FirewallPolicy{
		Key:                  "it_forward_drop_" + suffix,
		Label:                "Integration forward drop " + suffix,
		Scope:                "node",
		DefaultInputPolicy:   "accept",
		DefaultForwardPolicy: "drop",
		DefaultOutputPolicy:  "accept",
		Status:               "active",
	})
	if err != nil {
		t.Fatalf("create firewall policy: %v", err)
	}
	_, err = store.CreateFirewallPreviewJob(ctx, node.ID, policy.ID, true)
	if err == nil {
		t.Fatal("strict forward drop on egress node without source allow succeeded, want validation error")
	}
	if !strings.Contains(err.Error(), "strict forward") {
		t.Fatalf("strict forward error = %q, want strict forward message", err.Error())
	}
}

func TestPostgresIntegrationStrictFirewallRejectsOutputDropWithoutControlPlaneEgress(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-fw-output-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "10.50.8.43",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	policy, err := store.CreateFirewallPolicy(ctx, domain.FirewallPolicy{
		Key:                  "it_output_drop_" + suffix,
		Label:                "Integration output drop " + suffix,
		Scope:                "node",
		DefaultInputPolicy:   "accept",
		DefaultForwardPolicy: "accept",
		DefaultOutputPolicy:  "drop",
		Status:               "active",
	})
	if err != nil {
		t.Fatalf("create firewall policy: %v", err)
	}
	_, err = store.CreateFirewallPreviewJob(ctx, node.ID, policy.ID, true)
	if err == nil {
		t.Fatal("strict output drop without control-plane egress succeeded, want validation error")
	}
	if !strings.Contains(err.Error(), "strict output") {
		t.Fatalf("strict output error = %q, want strict output message", err.Error())
	}
}

func TestPostgresIntegrationFirewallManagementCIDRsRejectDNSAndBroadSources(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	if err := store.SeedFirewallManagementCIDRs(ctx, []string{"control.example.test"}, nil); err == nil {
		t.Fatal("DNS management CIDR was accepted, want validation error")
	}
	if err := store.SeedFirewallManagementCIDRs(ctx, []string{"0.0.0.0/0"}, nil); err == nil {
		t.Fatal("broad management CIDR was accepted, want validation error")
	}
}

func TestPostgresIntegrationFirewallManagementCIDRsReplaceManagedEntries(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	if err := store.SeedFirewallManagementCIDRs(ctx, []string{"10.60.0.10/32"}, []string{"10.60.0.11/32"}); err != nil {
		t.Fatalf("initial seed firewall management CIDRs: %v", err)
	}
	if err := store.SeedFirewallManagementCIDRs(ctx, []string{"10.60.0.20/32"}, nil); err != nil {
		t.Fatalf("replace control-plane firewall CIDRs: %v", err)
	}
	settings, err := store.GetFirewallManagementSettings(ctx)
	if err != nil {
		t.Fatalf("get firewall management settings: %v", err)
	}
	if strings.Join(settings.ControlPlaneSourceCIDRs, ",") != "10.60.0.20/32" {
		t.Fatalf("control-plane CIDRs = %#v, want only replacement value", settings.ControlPlaneSourceCIDRs)
	}
	if strings.Join(settings.SSHBootstrapSourceCIDRs, ",") != "10.60.0.11/32" {
		t.Fatalf("SSH bootstrap CIDRs = %#v, want unchanged value when env set is empty", settings.SSHBootstrapSourceCIDRs)
	}
}

func TestPostgresIntegrationBootstrapBlockedByEnforcedFirewallWithoutSSHAllow(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-fw-bootstrap-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "draft",
		Address:       "10.50.9.10",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "ssh_bootstrap",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	secret, err := store.CreateSecretRef(ctx, "ssh_key", []byte("integration-ssh-key"), map[string]any{
		"scope": "node_bootstrap_test",
	})
	if err != nil {
		t.Fatalf("create ssh secret ref: %v", err)
	}
	secretID := secret.ID
	if _, err := store.ReplaceNodeAccessMethods(ctx, node.ID, []domain.NodeAccessMethod{{
		Method:           "ssh",
		IsEnabled:        true,
		SSHHost:          "203.0.113.10",
		SSHPort:          22,
		SSHUser:          "root",
		SSHHostKeySHA256: "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=",
		AuthType:         "ssh_key",
		SecretRefID:      &secretID,
	}}); err != nil {
		t.Fatalf("replace node access methods: %v", err)
	}

	inventory, err := store.FirewallInventory(ctx)
	if err != nil {
		t.Fatalf("firewall inventory: %v", err)
	}
	var nodeBase domain.FirewallPolicy
	for _, policy := range inventory.Policies {
		if policy.Key == "node_base" {
			nodeBase = policy
			break
		}
	}
	if nodeBase.ID == "" {
		t.Fatal("node_base firewall policy was not seeded")
	}
	if _, err := store.db.Exec(ctx, `insert into firewall_node_state(id,node_id,policy_id,status,observed_json,updated_at)
values($1,$2,$3,'applied','{"default_policy_enforcement":"enforced"}'::jsonb,now())`,
		id.New(), node.ID, nodeBase.ID); err != nil {
		t.Fatalf("insert enforced firewall state: %v", err)
	}

	_, _, err = store.CreateNodeBootstrapJob(ctx, node.ID, "ssh_bootstrap", map[string]any{"reinstall_agent": true})
	if err == nil {
		t.Fatal("expected enforced firewall without SSH allow to block bootstrap")
	}
	if !strings.Contains(err.Error(), "node firewall is enforced") || !strings.Contains(err.Error(), "22") {
		t.Fatalf("unexpected bootstrap block error: %v", err)
	}

	_, err = store.CreateFirewallRule(ctx, nodeBase.ID, domain.FirewallRule{
		Priority:   180,
		Chain:      "input",
		Action:     "accept",
		Direction:  "in",
		Protocol:   "tcp",
		SrcCIDR:    "10.50.0.10/32",
		DstPorts:   "22",
		StateMatch: []string{"new", "established"},
		Comment:    "Allow SSH bootstrap from controlled source.",
		Enabled:    true,
		Status:     "active",
	})
	if err != nil {
		t.Fatalf("create SSH allow firewall rule: %v", err)
	}
	job, run, err := store.CreateNodeBootstrapJob(ctx, node.ID, "ssh_bootstrap", map[string]any{"reinstall_agent": true})
	if err != nil {
		t.Fatalf("expected SSH allow rule to permit bootstrap: %v", err)
	}
	if job.Type != "node.bootstrap" || run.Status != "queued" {
		t.Fatalf("bootstrap job/run = %#v / %#v, want queued node.bootstrap", job, run)
	}
}

func TestPostgresIntegrationCreateNodeSSHAccessMethodAtomic(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	attachPostgresIntegrationSecretService(t, store)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-ssh-access-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "draft",
		Address:       "203.0.113.20",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "ssh_bootstrap",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	_, err = store.CreateNodeSSHAccessMethod(ctx, id.New(), domain.NodeSSHAccessMethodCreateInput{
		SSHHost:          "203.0.113.99",
		SSHUser:          "support",
		SSHHostKeySHA256: "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=",
		PrivateKey:       generatedOpenSSHPrivateKey(t),
		IsEnabled:        true,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing node error = %v, want pgx.ErrNoRows", err)
	}

	if _, err := store.ReplaceNodeAccessMethods(ctx, node.ID, []domain.NodeAccessMethod{{
		Method:    "manual_bundle",
		IsEnabled: true,
		AuthType:  "none",
	}}); err != nil {
		t.Fatalf("seed unrelated access method: %v", err)
	}

	privateKey := generatedOpenSSHPrivateKey(t)
	actorID := id.New()
	created, err := store.CreateNodeSSHAccessMethod(ctx, node.ID, domain.NodeSSHAccessMethodCreateInput{
		SSHHost:          "203.0.113.20",
		SSHUser:          "support",
		SSHHostKeySHA256: "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=",
		PrivateKey:       privateKey,
		IsEnabled:        true,
		ActorUserID:      &actorID,
	})
	if err != nil {
		t.Fatalf("create ssh access method: %v", err)
	}
	if created.Method != "ssh" || created.AuthType != "ssh_key" || created.SSHPort != 22 || created.SecretRefID == nil {
		t.Fatalf("created method = %#v", created)
	}

	methods, err := store.ListNodeAccessMethods(ctx, node.ID)
	if err != nil {
		t.Fatalf("list access methods: %v", err)
	}
	if len(methods) != 2 {
		t.Fatalf("methods len = %d, want unrelated + ssh: %#v", len(methods), methods)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from node_access_methods where node_id=$1 and method='manual_bundle'`, 1, node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from node_access_methods where node_id=$1 and method='ssh'`, 1, node.ID)

	var secretType string
	var ciphertext, metaRaw []byte
	if err := store.db.QueryRow(ctx, `select secret_type,ciphertext,meta_json from secret_refs where id=$1`, *created.SecretRefID).Scan(&secretType, &ciphertext, &metaRaw); err != nil {
		t.Fatalf("select secret ref: %v", err)
	}
	if secretType != "ssh_key" {
		t.Fatalf("secret_type = %q, want ssh_key", secretType)
	}
	if string(ciphertext) == privateKey {
		t.Fatal("stored ciphertext must not equal plaintext private key")
	}
	var meta map[string]any
	if err := decodeJSONField(metaRaw, &meta, "secret_refs.meta_json"); err != nil {
		t.Fatalf("decode secret meta: %v", err)
	}
	if meta["purpose"] != "node_ssh_bootstrap" || meta["node_id"] != node.ID || meta["access_method_id"] != created.ID || meta["auth_type"] != "ssh_key" {
		t.Fatalf("secret meta = %#v", meta)
	}
	ref, plaintext, err := store.ResolveSecretValue(ctx, *created.SecretRefID)
	if err != nil {
		t.Fatalf("resolve secret: %v", err)
	}
	if ref.SecretType != "ssh_key" || string(plaintext) != privateKey {
		t.Fatalf("resolved secret mismatch: ref=%#v", ref)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from audit_events where action='node.ssh_access_method.create' and resource_id=$1 and summary not like '%PRIVATE KEY%'`, 1, node.ID)

	_, err = store.CreateNodeSSHAccessMethod(ctx, node.ID, domain.NodeSSHAccessMethodCreateInput{
		SSHHost:          "203.0.113.20",
		SSHPort:          22,
		SSHUser:          "support",
		SSHHostKeySHA256: "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=",
		PrivateKey:       generatedOpenSSHPrivateKey(t),
		IsEnabled:        true,
	})
	if !errors.Is(err, domain.ErrNodeSSHAccessMethodDuplicate) {
		t.Fatalf("duplicate error = %v, want ErrNodeSSHAccessMethodDuplicate", err)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from node_access_methods where node_id=$1 and method='ssh'`, 1, node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where meta_json->>'node_id'=$1 and meta_json->>'purpose'='node_ssh_bootstrap'`, 1, node.ID)

	passwordRef, err := store.CreateSecretRef(ctx, "password", []byte("not-an-ssh-key"), map[string]any{"node_id": node.ID})
	if err != nil {
		t.Fatalf("create password secret: %v", err)
	}
	passwordRefID := passwordRef.ID
	_, err = store.ReplaceNodeAccessMethods(ctx, node.ID, []domain.NodeAccessMethod{{
		Method:           "ssh",
		IsEnabled:        true,
		SSHHost:          "203.0.113.21",
		SSHPort:          22,
		SSHUser:          "support",
		SSHHostKeySHA256: "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=",
		AuthType:         "ssh_key",
		SecretRefID:      &passwordRefID,
	}})
	if err == nil || !strings.Contains(err.Error(), "ssh_key secret") {
		t.Fatalf("expected incompatible secret type rejection, got %v", err)
	}
}

func TestPostgresIntegrationCreateNodeSSHAccessMethodRejectsRetiredNode(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	attachPostgresIntegrationSecretService(t, store)

	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-ssh-retired-" + strings.ReplaceAll(id.New(), "-", "")[:10],
		Kind:          "remote",
		Role:          "egress",
		Status:        "draft",
		Address:       "203.0.113.30",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "ssh_bootstrap",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	if _, err := store.RetireNode(ctx, node.ID); err != nil {
		t.Fatalf("retire node: %v", err)
	}
	_, err = store.CreateNodeSSHAccessMethod(ctx, node.ID, domain.NodeSSHAccessMethodCreateInput{
		SSHHost:          "203.0.113.30",
		SSHUser:          "support",
		SSHHostKeySHA256: "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=",
		PrivateKey:       generatedOpenSSHPrivateKey(t),
		IsEnabled:        true,
	})
	if !errors.Is(err, domain.ErrNodeNotManageable) {
		t.Fatalf("retired node error = %v, want ErrNodeNotManageable", err)
	}
}

func TestPostgresIntegrationCreateNodeSSHAccessMethodConcurrentDuplicate(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	attachPostgresIntegrationSecretService(t, store)

	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-ssh-concurrent-" + strings.ReplaceAll(id.New(), "-", "")[:10],
		Kind:          "remote",
		Role:          "egress",
		Status:        "draft",
		Address:       "203.0.113.40",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "ssh_bootstrap",
		AgentStatus:   "unknown",
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.CreateNodeSSHAccessMethod(ctx, node.ID, domain.NodeSSHAccessMethodCreateInput{
				SSHHost:          "203.0.113.40",
				SSHPort:          22,
				SSHUser:          "support",
				SSHHostKeySHA256: "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGH1234567890+/=",
				PrivateKey:       generatedOpenSSHPrivateKey(t),
				IsEnabled:        true,
			})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	duplicates := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrNodeSSHAccessMethodDuplicate):
			duplicates++
		default:
			t.Fatalf("unexpected concurrent create error: %v", err)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("concurrent results successes=%d duplicates=%d, want 1/1", successes, duplicates)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from node_access_methods where node_id=$1 and method='ssh'`, 1, node.ID)
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
			"drivers":       []any{backhaul.DriverWireGuard},
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

func TestPostgresIntegrationBackhaulPromoteRefreshesXrayBeforeRoutePolicy(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	ingress, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-ingress-xray-promote-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "198.51.100.31",
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
		Name:          "it-egress-xray-promote-" + suffix,
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "203.0.113.31",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create egress node: %v", err)
	}
	if err := store.upsertNodeCapability(ctx, ingress.ID, "xray-core", "1.8.0", "available", "manual"); err != nil {
		t.Fatalf("mark xray capability available: %v", err)
	}
	link, err := store.CreateBackhaulLink(ctx, domain.BackhaulLink{
		Name:          "it-backhaul-xray-promote-" + suffix,
		IngressNodeID: ingress.ID,
		EgressNodeID:  egress.ID,
		DesiredDriver: backhaul.DriverWireGuard,
		RouteMetric:   30,
		Metadata: map[string]any{
			"endpoint_host": egress.Address,
			"tunnel_cidr":   "10.240.252.4/30",
			"drivers":       []any{backhaul.DriverWireGuard, backhaul.DriverOpenVPNUDP},
		},
	})
	if err != nil {
		t.Fatalf("create backhaul link: %v", err)
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

	oldSendThrough := hostAddress(wireguardTransport.IngressAddress)
	newSendThrough := hostAddress(openVPNTransport.IngressAddress)
	xraySpec := xraySharedClientIdentityTestSpec("portal.example.invalid")
	xraySpec["xray_egress"] = map[string]any{
		"mode":            "remote_egress",
		"egress_node_id":  egress.ID,
		"link_id":         link.ID,
		"transport_id":    wireguardTransport.ID,
		"driver":          wireguardTransport.Driver,
		"interface":       wireguardTransport.InterfaceName,
		"ingress_address": wireguardTransport.IngressAddress,
		"send_through":    oldSendThrough,
		"routing_table":   link.RoutingTable,
		"route_metric":    link.RouteMetric,
	}
	xraySpec["xray_default_outbound"] = map[string]any{
		"tag":         "egress-default",
		"protocol":    "freedom",
		"sendThrough": oldSendThrough,
		"settings":    map[string]any{"domainStrategy": "UseIP"},
	}
	xray, err := store.CreateInstanceValidatedDraft(ctx, domain.Instance{
		NodeID:       ingress.ID,
		ServiceCode:  "xray-core",
		Name:         "it-xray-promote-" + suffix,
		Slug:         "it-xray-promote-" + suffix,
		Status:       "active",
		EndpointHost: "portal.example.invalid",
		EndpointPort: 7080,
		Spec:         xraySpec,
	})
	if err != nil {
		t.Fatalf("create xray instance: %v", err)
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

	promoted, jobs, err := store.PromoteBackhaulTransport(ctx, link.ID, openVPNTransport.ID)
	if err != nil {
		t.Fatalf("promote openvpn standby transport: %v", err)
	}
	if promoted.Status != "active" || promoted.SelectedTransportID == nil || *promoted.SelectedTransportID != openVPNTransport.ID {
		t.Fatalf("promoted link = status:%s selected:%v, want active openvpn %s", promoted.Status, promoted.SelectedTransportID, openVPNTransport.ID)
	}
	if len(jobs) != 1 || jobs[0].Type != "instance.apply" || jobs[0].InstanceID == nil || *jobs[0].InstanceID != xray.ID {
		t.Fatalf("promotion jobs = %#v, want xray instance apply before route policy", jobs)
	}
	revisions, err := store.ListInstanceRevisions(ctx, xray.ID, 1)
	if err != nil {
		t.Fatalf("list xray revisions: %v", err)
	}
	if len(revisions) != 1 {
		t.Fatalf("xray revisions = %#v, want latest revision", revisions)
	}
	egressSpec := mapFromAny(revisions[0].Spec["xray_egress"])
	if got := firstString(egressSpec["transport_id"]); got != openVPNTransport.ID {
		t.Fatalf("xray egress transport_id = %q, want openvpn transport %s: %#v", got, openVPNTransport.ID, egressSpec)
	}
	if got := firstString(egressSpec["send_through"]); got != newSendThrough {
		t.Fatalf("xray egress send_through = %q, want %s: %#v", got, newSendThrough, egressSpec)
	}
	outboundSpec := mapFromAny(revisions[0].Spec["xray_default_outbound"])
	if got := firstString(outboundSpec["sendThrough"]); got != newSendThrough {
		t.Fatalf("xray default outbound sendThrough = %q, want %s: %#v", got, newSendThrough, outboundSpec)
	}

	if err := store.CompleteJob(ctx, jobs[0].ID, "succeeded", map[string]any{"active_state": "active"}); err != nil {
		t.Fatalf("complete first xray convergence apply: %v", err)
	}
	staleSpec := cloneMap(revisions[0].Spec)
	staleEgress := mapFromAny(staleSpec["xray_egress"])
	staleEgress["transport_id"] = wireguardTransport.ID
	staleEgress["driver"] = wireguardTransport.Driver
	staleEgress["interface"] = wireguardTransport.InterfaceName
	staleEgress["ingress_address"] = wireguardTransport.IngressAddress
	staleEgress["send_through"] = oldSendThrough
	staleSpec["xray_egress"] = staleEgress
	staleOutbound := mapFromAny(staleSpec["xray_default_outbound"])
	staleOutbound["sendThrough"] = oldSendThrough
	staleSpec["xray_default_outbound"] = staleOutbound
	if _, err := store.ReplaceInstanceSpec(ctx, xray.ID, "test:stale-xray-egress", staleSpec); err != nil {
		t.Fatalf("restore stale xray egress revision: %v", err)
	}

	_, refreshJobs, err := store.PromoteBackhaulTransport(ctx, promoted.ID, openVPNTransport.ID)
	if err != nil {
		t.Fatalf("idempotent promote should refresh stale xray egress: %v", err)
	}
	if len(refreshJobs) != 1 || refreshJobs[0].Type != "instance.apply" || refreshJobs[0].InstanceID == nil || *refreshJobs[0].InstanceID != xray.ID {
		t.Fatalf("idempotent promotion jobs = %#v, want xray instance apply", refreshJobs)
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
		Architecture:  "amd64",
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
		Architecture:  "amd64",
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
		Architecture:  "amd64",
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

func TestPostgresIntegrationAgentJobCompletionRejectsForgedLeaseOwner(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-forged-job-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "10.50.1.20",
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
		Payload:   map[string]any{"node_id": node.ID},
	})
	if err != nil {
		t.Fatalf("create route policy job: %v", err)
	}
	claimed, ok, err := store.AgentNextJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("claim route policy job: %v", err)
	}
	if !ok || claimed.ID != job.ID || claimed.LockedBy == nil {
		t.Fatalf("claimed job = %#v, want locked running job %s", claimed, job.ID)
	}
	if err := store.CompleteAgentJob(ctx, claimed.ID, "agent:forged-node", "succeeded", map[string]any{"message": "forged result"}); err == nil {
		t.Fatal("forged agent completion with mismatched lease owner must be rejected")
	}
	afterForge, err := store.GetJob(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("get job after forged completion: %v", err)
	}
	if afterForge.Status != "running" || afterForge.FinishedAt != nil {
		t.Fatalf("job after forged completion = status:%s finished:%v, want still running", afterForge.Status, afterForge.FinishedAt)
	}
	if err := store.CompleteAgentJob(ctx, claimed.ID, "agent:"+node.ID, "succeeded", map[string]any{"message": "valid result"}); err != nil {
		t.Fatalf("complete with real owner: %v", err)
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

func TestPostgresIntegrationRevokeNodeEnrollmentTokenKeepsSecretHidden(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-token-revoke-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "draft",
		Address:       "10.50.2.11",
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
	if token.Token == "" || token.TokenHint == "" {
		t.Fatalf("new enrollment token should return one-time plaintext and hint: %#v", token)
	}

	revoked, err := store.RevokeNodeEnrollmentToken(ctx, node.ID, token.ID)
	if err != nil {
		t.Fatalf("revoke enrollment token: %v", err)
	}
	if revoked.Status != "revoked" {
		t.Fatalf("revoked status = %q, want revoked", revoked.Status)
	}
	if revoked.Token != "" {
		t.Fatalf("revoked token response must not include plaintext token")
	}

	tokens, err := store.ListNodeEnrollmentTokens(ctx, node.ID)
	if err != nil {
		t.Fatalf("list enrollment tokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Status != "revoked" || tokens[0].Token != "" {
		t.Fatalf("listed token metadata = %#v, want one revoked metadata-only token", tokens)
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
	createdByUser, _, err := store.EnsureBootstrapPlatformUser(ctx, "it-cleanup-admin-"+suffix, "it-cleanup-admin-"+suffix+"@example.invalid", "Integration Cleanup Admin", "integration-password-hash")
	if err != nil {
		t.Fatalf("create email delivery user: %v", err)
	}
	createdBy := createdByUser.ID
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

func TestPostgresIntegrationDeleteClientServiceAccessRemovesRows(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	store.SetArtifactRoot(t.TempDir())

	fixture := createClientServiceAccessCleanupFixture(t, ctx, store, "access-cleanup")
	result, err := store.DeleteClientServiceAccess(ctx, fixture.client.ID, fixture.access.ID)
	if err != nil {
		t.Fatalf("delete client service access: %v", err)
	}
	if !result.Deleted {
		t.Fatalf("service access delete result did not mark deleted: %#v", result)
	}
	if result.ServiceAccessesDeleted != 1 || result.AccessRoutesDeleted != 1 || result.ConfigCleanup.ArtifactsDeleted != 1 || result.ConfigCleanup.ShareLinksDeleted != 1 || result.SecretRefsDeleted != 1 {
		t.Fatalf("service access delete result = %#v, want one access/route/artifact/share/secret", result)
	}

	assertPostgresCount(t, ctx, store, `select count(*) from client_accounts where id=$1`, 1, fixture.client.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from instances where id=$1`, 1, fixture.instance.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where id=$1`, 0, fixture.access.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from client_access_routes where service_access_id=$1`, 0, fixture.access.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from artifacts where id=$1`, 0, fixture.artifact.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from share_links where id=$1`, 0, fixture.share.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 0, fixture.secret.ID)
	if _, err := os.Stat(fixture.artifact.StoragePath); !os.IsNotExist(err) {
		t.Fatalf("artifact file after service access delete error = %v, want not exist", err)
	}
}

func TestPostgresIntegrationShareLinkRejectsRevokedTokenReuse(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	store.SetArtifactRoot(t.TempDir())

	fixture := createClientServiceAccessCleanupFixture(t, ctx, store, "share-revoke")
	link, artifact, err := store.ResolveShareLinkArtifact(ctx, fixture.share.Token)
	if err != nil {
		t.Fatalf("resolve active share link: %v", err)
	}
	if link.ID != fixture.share.ID || artifact.ID != fixture.artifact.ID {
		t.Fatalf("resolved link/artifact = %s/%s, want %s/%s", link.ID, artifact.ID, fixture.share.ID, fixture.artifact.ID)
	}
	if _, err := store.RevokeShareLink(ctx, fixture.client.ID, fixture.share.ID); err != nil {
		t.Fatalf("revoke share link: %v", err)
	}
	if _, _, err := store.ResolveShareLinkArtifact(ctx, fixture.share.Token); err == nil {
		t.Fatal("revoked share token must not be reusable")
	}
}

func TestPostgresIntegrationShareLinkRejectsExpiredToken(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	store.SetArtifactRoot(t.TempDir())

	fixture := createClientServiceAccessCleanupFixture(t, ctx, store, "share-expired")
	if _, err := store.db.Exec(ctx, `update share_links set expires_at=$2 where id=$1`, fixture.share.ID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("expire share link: %v", err)
	}
	if _, _, err := store.ResolveShareLinkArtifact(ctx, fixture.share.Token); err == nil {
		t.Fatal("expired share token must be rejected")
	}
	var status string
	if err := store.db.QueryRow(ctx, `select status from share_links where id=$1`, fixture.share.ID).Scan(&status); err != nil {
		t.Fatalf("load expired share link status: %v", err)
	}
	if status != "expired" {
		t.Fatalf("share link status after expired resolve = %q, want expired", status)
	}
}

func TestPostgresIntegrationDeleteArtifactRemovesOnlyConfigAndShareLink(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	store.SetArtifactRoot(t.TempDir())

	fixture := createClientServiceAccessCleanupFixture(t, ctx, store, "artifact-cleanup")
	result, err := store.DeleteArtifact(ctx, fixture.client.ID, fixture.artifact.ID)
	if err != nil {
		t.Fatalf("delete artifact: %v", err)
	}
	if !result.Deleted {
		t.Fatalf("artifact delete result did not mark deleted: %#v", result)
	}
	if result.ArtifactID != fixture.artifact.ID || result.ClientID != fixture.client.ID {
		t.Fatalf("artifact delete refs = %#v", result)
	}
	if result.ShareLinksDeleted != 1 || result.FilesDeleted != 1 {
		t.Fatalf("artifact delete cleanup = %#v, want one share link and one file", result)
	}

	assertPostgresCount(t, ctx, store, `select count(*) from client_accounts where id=$1`, 1, fixture.client.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where id=$1`, 1, fixture.access.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from client_access_routes where service_access_id=$1`, 1, fixture.access.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from artifacts where id=$1`, 0, fixture.artifact.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from share_links where id=$1`, 0, fixture.share.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 1, fixture.secret.ID)
	if _, err := os.Stat(fixture.artifact.StoragePath); !os.IsNotExist(err) {
		t.Fatalf("artifact file after artifact delete error = %v, want not exist", err)
	}
}

func TestPostgresIntegrationInstanceDeleteRemovesClientServiceAccessRows(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	store.SetArtifactRoot(t.TempDir())

	fixture := createClientServiceAccessCleanupFixture(t, ctx, store, "instance-cleanup")
	deleting, err := store.DeleteInstance(ctx, fixture.instance.ID)
	if err != nil {
		t.Fatalf("delete instance with client access: %v", err)
	}
	if deleting.Status != "deleting" {
		t.Fatalf("delete instance status = %q, want deleting", deleting.Status)
	}
	deleteJob, ok, err := store.AgentNextJob(ctx, fixture.node.ID)
	if err != nil {
		t.Fatalf("claim instance delete job: %v", err)
	}
	if !ok {
		t.Fatal("expected queued instance delete job")
	}
	if deleteJob.Type != "instance.delete" {
		t.Fatalf("claimed job type = %q, want instance.delete", deleteJob.Type)
	}
	if err := store.CompleteJob(ctx, deleteJob.ID, "succeeded", map[string]any{"active_state": "inactive"}); err != nil {
		t.Fatalf("complete instance delete job: %v", err)
	}

	var status string
	if err := store.db.QueryRow(ctx, `select status from instances where id=$1`, fixture.instance.ID).Scan(&status); err != nil {
		t.Fatalf("get instance status after delete: %v", err)
	}
	if status != "deleted" {
		t.Fatalf("instance status after delete = %q, want deleted", status)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from client_accounts where id=$1`, 1, fixture.client.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where id=$1`, 0, fixture.access.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from client_access_routes where service_access_id=$1`, 0, fixture.access.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from artifacts where id=$1`, 0, fixture.artifact.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from share_links where id=$1`, 0, fixture.share.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 0, fixture.secret.ID)
	if _, err := os.Stat(fixture.artifact.StoragePath); !os.IsNotExist(err) {
		t.Fatalf("artifact file after instance delete error = %v, want not exist", err)
	}
}

func TestPostgresIntegrationForceRetireLostNodeCleansControlPlaneState(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	store.SetArtifactRoot(t.TempDir())

	fixture := createClientServiceAccessCleanupFixture(t, ctx, store, "force-retire")
	if _, err := store.ForceRetireNode(ctx, fixture.node.ID, "wrong-node-name", "integration test"); err == nil {
		t.Fatal("force retire should require exact node confirmation")
	}
	if _, err := store.DeleteInstance(ctx, fixture.instance.ID); err != nil {
		t.Fatalf("queue instance delete before lost-node cleanup: %v", err)
	}

	egress, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-force-retire-egress-" + strings.ReplaceAll(id.New(), "-", "")[:10],
		Kind:          "remote",
		Role:          "egress",
		Status:        "online",
		Address:       "203.0.113.46",
		OSFamily:      "linux",
		OSVersion:     "ubuntu-24.04",
		Architecture:  "amd64",
		ExecutionMode: "agent_managed",
		AgentStatus:   "online",
	})
	if err != nil {
		t.Fatalf("create egress node: %v", err)
	}
	backhaulID := id.New()
	if _, err := store.db.Exec(ctx, `insert into backhaul_links(id,name,ingress_node_id,egress_node_id,status,desired_driver)
		values($1,$2,$3,$4,'active','wireguard')`, backhaulID, "it-force-retire-backhaul", fixture.node.ID, egress.ID); err != nil {
		t.Fatalf("insert backhaul link: %v", err)
	}
	backhaulSecret, err := store.CreateSecretRef(ctx, "private_key", []byte("integration-backhaul-secret"), map[string]any{
		"scope":   "backhaul",
		"link_id": backhaulID,
	})
	if err != nil {
		t.Fatalf("create backhaul secret: %v", err)
	}

	if _, err := store.RetireNode(ctx, fixture.node.ID); err == nil || !strings.Contains(err.Error(), "active instances") {
		t.Fatalf("normal retire should block active/deleting instances, got %v", err)
	}
	retired, err := store.ForceRetireNode(ctx, fixture.node.ID, fixture.node.Name, "lost node integration test")
	if err != nil {
		t.Fatalf("force retire lost node: %v", err)
	}
	if retired.Status != "retired" || retired.AgentStatus != "offline" {
		t.Fatalf("retired node state = %s/%s, want retired/offline", retired.Status, retired.AgentStatus)
	}

	var instanceStatus string
	if err := store.db.QueryRow(ctx, `select status from instances where id=$1`, fixture.instance.ID).Scan(&instanceStatus); err != nil {
		t.Fatalf("get instance status after force retire: %v", err)
	}
	if instanceStatus != "deleted" {
		t.Fatalf("instance status after force retire = %q, want deleted", instanceStatus)
	}
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where id=$1`, 0, fixture.access.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from client_access_routes where service_access_id=$1`, 0, fixture.access.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from artifacts where id=$1`, 0, fixture.artifact.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from share_links where id=$1`, 0, fixture.share.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 0, fixture.secret.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from instance_runtime_states where instance_id=$1`, 0, fixture.instance.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where node_id=$1 and status in ('queued','running','retrying')`, 0, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from resource_locks rl join jobs j on j.id=rl.job_id where j.node_id=$1`, 0, fixture.node.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 0, backhaulSecret.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where type in ('client.provision','artifact.build') and status in ('queued','running','retrying') and payload_json->'instance_ids' ? $1`, 0, fixture.instance.ID)

	var backhaulStatus string
	if err := store.db.QueryRow(ctx, `select status from backhaul_links where id=$1`, backhaulID).Scan(&backhaulStatus); err != nil {
		t.Fatalf("get backhaul status after force retire: %v", err)
	}
	if backhaulStatus != "deleted" {
		t.Fatalf("backhaul status after force retire = %q, want deleted", backhaulStatus)
	}
	if _, err := os.Stat(fixture.artifact.StoragePath); !os.IsNotExist(err) {
		t.Fatalf("artifact file after force retire error = %v, want not exist", err)
	}
}

func TestPostgresIntegrationForceDeleteLostNodeInstanceCleansControlPlaneState(t *testing.T) {
	store, ctx := setupPostgresIntegrationStore(t)
	store.SetArtifactRoot(t.TempDir())

	fixture := createClientServiceAccessCleanupFixture(t, ctx, store, "force-delete-instance")
	instanceSecret, err := store.CreateSecretRef(ctx, "private_key", []byte("integration-instance-secret"), map[string]any{
		"scope":       "instance",
		"instance_id": fixture.instance.ID,
	})
	if err != nil {
		t.Fatalf("create instance secret: %v", err)
	}
	if _, err := store.CreateArtifactBuildJob(ctx, fixture.client.ID, "all", []string{fixture.instance.ID}); err != nil {
		t.Fatalf("queue artifact build job: %v", err)
	}
	if _, err := store.DeleteInstance(ctx, fixture.instance.ID); err != nil {
		t.Fatalf("queue instance delete before force delete: %v", err)
	}
	if _, err := store.ForceDeleteInstance(ctx, fixture.instance.ID, "wrong-instance-name", "integration test"); err == nil {
		t.Fatal("force delete should require exact instance confirmation")
	}

	deleted, err := store.ForceDeleteInstance(ctx, fixture.instance.ID, fixture.instance.Name, "lost node integration test")
	if err != nil {
		t.Fatalf("force delete lost-node instance: %v", err)
	}
	if deleted.Status != "deleted" || deleted.Enabled {
		t.Fatalf("force-deleted instance state = %s/enabled:%v, want deleted/false", deleted.Status, deleted.Enabled)
	}

	assertPostgresCount(t, ctx, store, `select count(*) from client_accounts where id=$1`, 1, fixture.client.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from service_accesses where id=$1`, 0, fixture.access.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from client_access_routes where service_access_id=$1`, 0, fixture.access.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from artifacts where id=$1`, 0, fixture.artifact.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from share_links where id=$1`, 0, fixture.share.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 0, fixture.secret.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from secret_refs where id=$1`, 0, instanceSecret.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from instance_runtime_states where instance_id=$1`, 0, fixture.instance.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where instance_id=$1 and status in ('queued','running','retrying')`, 0, fixture.instance.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from jobs where type in ('client.provision','artifact.build') and status in ('queued','running','retrying') and payload_json->'instance_ids' ? $1`, 0, fixture.instance.ID)
	assertPostgresCount(t, ctx, store, `select count(*) from resource_locks where resource_type='instance' and resource_id::text=$1`, 0, fixture.instance.ID)
	if _, err := os.Stat(fixture.artifact.StoragePath); !os.IsNotExist(err) {
		t.Fatalf("artifact file after force delete error = %v, want not exist", err)
	}
}

type clientServiceAccessCleanupFixture struct {
	node     domain.Node
	instance domain.Instance
	client   domain.Client
	access   domain.ServiceAccess
	artifact domain.Artifact
	share    domain.ShareLink
	secret   domain.SecretRef
}

func createClientServiceAccessCleanupFixture(t *testing.T, ctx context.Context, store *Store, prefix string) clientServiceAccessCleanupFixture {
	t.Helper()

	suffix := strings.ReplaceAll(id.New(), "-", "")[:10]
	node, err := store.CreateNode(ctx, domain.Node{
		Name:          "it-" + prefix + "-node-" + suffix,
		Kind:          "remote",
		Role:          "ingress",
		Status:        "online",
		Address:       "203.0.113.45",
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
		Name:         "it-" + prefix + "-wg-" + suffix,
		Slug:         "it-" + prefix + "-wg-" + suffix,
		EndpointHost: "198.51.100.45",
		EndpointPort: 51820,
		Spec: map[string]any{
			"config_content": "[Interface]\nAddress = 10.99.1.1/24\nListenPort = 51820\nPrivateKey = integration-test\n",
		},
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	applyJob, ok, err := store.AgentNextJob(ctx, node.ID)
	if err != nil {
		t.Fatalf("claim initial apply job: %v", err)
	}
	if !ok {
		t.Fatal("expected initial apply job")
	}
	if err := store.CompleteJob(ctx, applyJob.ID, "succeeded", map[string]any{"active_state": "active"}); err != nil {
		t.Fatalf("complete initial apply job: %v", err)
	}

	client, err := store.CreateClient(ctx, domain.Client{
		Username:    "it-" + prefix + "-client-" + suffix,
		DisplayName: "Integration Access Cleanup Client",
		Email:       "it-" + prefix + "-client-" + suffix + "@example.invalid",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if _, err := store.ProvisionClient(ctx, client.ID, []string{instance.ID}); err != nil {
		t.Fatalf("provision client: %v", err)
	}
	for {
		routeJob, ok, err := store.AgentNextJob(ctx, node.ID)
		if err != nil {
			t.Fatalf("claim post-provision route policy job: %v", err)
		}
		if !ok {
			break
		}
		if routeJob.Type != "node.route_policy.apply" {
			t.Fatalf("post-provision job type = %q, want node.route_policy.apply", routeJob.Type)
		}
		if err := store.CompleteJob(ctx, routeJob.ID, "succeeded", map[string]any{"active_state": "active"}); err != nil {
			t.Fatalf("complete post-provision route policy job: %v", err)
		}
	}
	accesses, err := store.ListServiceAccesses(ctx, client.ID)
	if err != nil {
		t.Fatalf("list service accesses: %v", err)
	}
	if len(accesses) != 1 {
		t.Fatalf("service access count = %d, want 1", len(accesses))
	}
	access := accesses[0]
	artifact, err := store.SaveArtifactContent(ctx, client.ID, &access.ID, "wg_conf", "client.conf", []byte("[Interface]\nPrivateKey = integration-test\n"))
	if err != nil {
		t.Fatalf("save artifact: %v", err)
	}
	share, err := store.PublishShareLink(ctx, client.ID, artifact.ID, time.Hour)
	if err != nil {
		t.Fatalf("publish share link: %v", err)
	}
	secret, err := store.CreateSecretRef(ctx, "private_key", []byte("integration-secret"), map[string]any{
		"scope":             "service_access",
		"service_access_id": access.ID,
	})
	if err != nil {
		t.Fatalf("create service access secret: %v", err)
	}
	return clientServiceAccessCleanupFixture{
		node:     node,
		instance: instance,
		client:   client,
		access:   access,
		artifact: artifact,
		share:    share,
		secret:   secret,
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

func xraySharedClientIdentityTestSpec(host string) map[string]any {
	return map[string]any{
		"listen":              "127.0.0.1",
		"security":            "none",
		"network":             "ws",
		"path":                "/assets/test-ws",
		"public_security":     "tls",
		"public_network":      "ws",
		"public_path":         "/assets/test-ws",
		"public_host_header":  host,
		"public_port":         443,
		"default_vless_group": "default",
		"vless_groups": []any{
			map[string]any{
				"key":          "default",
				"label":        "Default access",
				"egress_mode":  "default",
				"outbound_tag": "direct",
			},
			map[string]any{
				"key":          "out_usa_sf",
				"label":        "Outgoing USA San Francisco",
				"egress_mode":  "default",
				"outbound_tag": "direct",
			},
		},
		"config_mode": "0640",
	}
}

func vlessGroupMemberCount(overview domain.VLESSGroupMembersOverview, key string) int {
	key = normalizeXrayVLESSGroupKey(key)
	for _, group := range overview.Groups {
		if group.Key == key {
			return group.MemberCount
		}
	}
	return 0
}

func clientAccessGroupByKey(t *testing.T, ctx context.Context, store *Store, serviceCode, groupKey string) domain.ClientAccessGroup {
	t.Helper()

	groups, err := store.ListClientAccessGroups(ctx, serviceCode)
	if err != nil {
		t.Fatalf("list client access groups: %v", err)
	}
	for _, group := range groups {
		if group.ServiceCode == normalizeClientAccessGroupServiceCode(serviceCode) && group.GroupKey == normalizeVLESSGroupTemplateKey(groupKey) {
			return group
		}
	}
	if normalizeClientAccessGroupServiceCode(serviceCode) == "vless" {
		template, err := store.getActiveVLESSGroupTemplate(ctx, groupKey)
		if err != nil {
			t.Fatalf("load vless template %s: %v", groupKey, err)
		}
		group, err := store.ensureClientAccessGroupFromVLESSTemplate(ctx, template)
		if err != nil {
			t.Fatalf("ensure client access group from vless template %s: %v", groupKey, err)
		}
		return group
	}
	t.Fatalf("client access group %s/%s not found: %#v", serviceCode, groupKey, groups)
	return domain.ClientAccessGroup{}
}

func activeClientAccessGroupKey(t *testing.T, ctx context.Context, store *Store, clientID, serviceCode string) string {
	t.Helper()

	var key string
	err := store.db.QueryRow(ctx, `select cag.group_key
		from client_access_group_memberships m
		join client_access_groups cag on cag.id=m.group_id
		where m.client_account_id=$1 and m.service_code=$2 and m.status='active'`,
		clientID, normalizeClientAccessGroupServiceCode(serviceCode)).Scan(&key)
	if err != nil {
		t.Fatalf("active client access group key for client %s: %v", clientID, err)
	}
	return key
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(fmt.Sprint(item)); value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func xrayServiceAccessByInstance(t *testing.T, ctx context.Context, store *Store, clientID, instanceID string) domain.ServiceAccess {
	t.Helper()

	accesses, err := store.ListServiceAccesses(ctx, clientID)
	if err != nil {
		t.Fatalf("list service accesses: %v", err)
	}
	for _, access := range accesses {
		if access.InstanceID == instanceID {
			return access
		}
	}
	t.Fatalf("service access for instance %s not found: %#v", instanceID, accesses)
	return domain.ServiceAccess{}
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
	store := New(pool)
	attachPostgresIntegrationSecretService(t, store)
	seedPostgresIntegrationVLESSGroups(t, ctx, store)
	return store, ctx
}

func attachPostgresIntegrationSecretService(t *testing.T, store *Store) {
	t.Helper()

	keyPath := filepath.Join(t.TempDir(), "master.key")
	rawKey := make([]byte, 32)
	if _, err := rand.Read(rawKey); err != nil {
		t.Fatalf("generate test master key: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(rawKey)), 0o600); err != nil {
		t.Fatalf("write test master key: %v", err)
	}
	secretSvc, err := secrets.LoadFromFile(keyPath, "test-v1")
	if err != nil {
		t.Fatalf("load test secret service: %v", err)
	}
	store.SetSecretService(secretSvc)
}

func seedPostgresIntegrationVLESSGroups(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()

	templates := []domain.VLESSGroupTemplate{
		{
			Key:          "default",
			Label:        "Default access",
			Description:  "Use the VLESS instance default route.",
			AccessMode:   "instance_default",
			EgressMode:   "default",
			OutboundTag:  "direct",
			Status:       "active",
			Source:       "default",
			Version:      1,
			DisplayOrder: 10,
		},
		{
			Key:          "out_usa_sf",
			Label:        "Outgoing USA San Francisco",
			Description:  "Integration-test managed outbound group.",
			AccessMode:   "instance_default",
			EgressMode:   "default",
			OutboundTag:  "direct",
			Status:       "active",
			Source:       "default",
			Version:      1,
			DisplayOrder: 20,
		},
		{
			Key:          "blocked",
			Label:        "Blocked",
			Description:  "Deny all traffic for clients assigned to this group.",
			AccessMode:   "block",
			EgressMode:   "block",
			OutboundTag:  "block",
			Status:       "active",
			Source:       "default",
			Version:      1,
			DisplayOrder: 90,
		},
	}
	if err := store.EnsureDefaultVLESSGroupTemplates(ctx, templates); err != nil {
		t.Fatalf("seed default vless templates: %v", err)
	}
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
