package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rtis-emc2/megavpn/internal/platform/config"
)

func enrollWithRetry(ctx context.Context, log agentLogger, cfg config.Config, bootstrap bootstrapConfig) (*agentState, error) {
	c := newClient(bootstrap.ControlPlaneURL, "", cfg.Agent.StatePath)
	log.Info("starting agent enrollment", "node", bootstrap.NodeName, "node_id", bootstrap.NodeID, "control_plane", bootstrap.ControlPlaneURL, "version", appVersion)
	attempt := 0
	for {
		st, err := c.register(ctx, bootstrap)
		if err == nil {
			if err := saveState(cfg.Agent.StatePath, *st); err != nil {
				return nil, fmt.Errorf("agent state save failed: %w", err)
			}
			if err := removeBootstrapFile(cfg.Agent.BootstrapPath); err != nil {
				log.Error("agent bootstrap cleanup failed", "path", cfg.Agent.BootstrapPath, "error", err)
			} else {
				log.Info("agent bootstrap file removed", "path", cfg.Agent.BootstrapPath)
			}
			log.Info("agent enrolled", "node", st.NodeName, "node_id", st.NodeID, "state_path", cfg.Agent.StatePath)
			return st, nil
		}
		wait := retryDelay(attempt, 3*time.Second, 45*time.Second)
		log.Error("agent register failed; retry scheduled", "error", err, "retry_in", wait.String())
		if err := sleepContext(ctx, wait); err != nil {
			return nil, err
		}
		attempt++
	}
}

func runControlLoop(ctx context.Context, log agentLogger, pollInterval time.Duration, c *client, st *agentState) {
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}
	failures := 0
	for {
		select {
		case <-ctx.Done():
			log.Info("agent stopped")
			return
		default:
		}

		if err := syncWithCore(ctx, log, c, st); err != nil {
			failures++
			wait := retryDelay(failures-1, 3*time.Second, pollInterval)
			log.Error("control plane sync failed; retry scheduled", "error", err, "retry_in", wait.String())
			if err := sleepContext(ctx, wait); err != nil {
				log.Info("agent stopped")
				return
			}
			continue
		}

		failures = 0
		if err := sleepContext(ctx, pollInterval); err != nil {
			log.Info("agent stopped")
			return
		}
	}
}

func syncWithCore(ctx context.Context, log agentLogger, c *client, st *agentState) error {
	if err := c.heartbeat(ctx, st.NodeID, st.NodeName); err != nil {
		return fmt.Errorf("heartbeat failed: %w", err)
	}
	if err := c.reportInstanceRuntime(ctx, st.NodeID); err != nil {
		log.Error("agent runtime report failed", "error", err)
	}
	if err := c.reportTrafficAccounting(ctx, st.NodeID); err != nil {
		log.Error("agent traffic accounting report failed", "error", err)
	}
	j, ok, err := c.nextJob(ctx, st.NodeID)
	if err != nil {
		return fmt.Errorf("fetch job failed: %w", err)
	}
	if !ok {
		return nil
	}
	log.Info("agent job received", "job_id", j.ID, "type", j.Type)
	status, result := c.execute(ctx, j, st)
	logAgentJobResult(log, j, status, result)
	if err := c.submit(ctx, j.ID, status, result); err != nil {
		return fmt.Errorf("submit job result failed: %w", err)
	}
	return nil
}

func logAgentJobResult(log agentLogger, j job, status string, result map[string]any) {
	if status == "succeeded" {
		log.Info("agent job completed", "job_id", j.ID, "type", j.Type, "status", status, "message", jobResultText(result, "message"))
		return
	}
	log.Error(
		"agent job failed",
		"job_id", j.ID,
		"type", j.Type,
		"status", status,
		"message", jobResultText(result, "message"),
		"error", jobResultText(result, "error"),
		"last_failed_command", jobResultText(result, "last_failed_command"),
		"last_failed_exit_code", jobResultText(result, "last_failed_exit_code"),
	)
}

func jobResultText(result map[string]any, key string) string {
	if result == nil {
		return ""
	}
	return truncate(stringify(result[key]), 500)
}

func retryDelay(attempt int, minWait, maxWait time.Duration) time.Duration {
	if minWait <= 0 {
		minWait = 3 * time.Second
	}
	if maxWait <= 0 || maxWait < minWait {
		maxWait = minWait
	}
	wait := minWait
	for i := 0; i < attempt && wait < maxWait; i++ {
		wait *= 2
		if wait > maxWait {
			wait = maxWait
		}
	}
	return wait
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *client) execute(ctx context.Context, j job, st *agentState) (string, map[string]any) {
	switch j.Type {
	case "node.inventory", "node.inventory.sync", "node.services.discover":
		inv := collectInventory()
		source := "inventory"
		if j.Type == "node.services.discover" {
			source = "discovery"
		}
		if err := c.submitInventory(ctx, st.NodeID, source, inv); err != nil {
			return "failed", map[string]any{"error": err.Error(), "stage": "submit_inventory"}
		}
		return "succeeded", map[string]any{
			"message":       "node inventory collected",
			"agent_version": appVersion,
			"hostname":      inv["hostname"],
			"source":        source,
			"capabilities":  capabilitySummary(inv),
		}
	case "node.channel.probe":
		inv := collectInventory()
		return "succeeded", map[string]any{
			"message":       "agent channel probe acknowledged",
			"agent_version": appVersion,
			"hostname":      inv["hostname"],
			"collected_at":  inv["collected_at"],
			"capabilities":  capabilitySummary(inv),
		}
	case "node.capability.install":
		return c.installCapability(ctx, j, *st)
	case "node.capability.verify":
		return c.verifyCapability(ctx, j, *st)
	case "node.agent.rotate_token":
		return c.rotateAgentToken(ctx, j, st)
	case "node.emergency_cleanup":
		return c.emergencyCleanupNode(ctx, j, *st)
	case "node.reboot":
		return c.rebootNode(ctx, j, *st)
	case "node.backhaul.apply":
		return c.applyBackhaul(ctx, j, *st)
	case "node.backhaul.probe":
		return c.probeBackhaul(ctx, j, *st)
	case "node.backhaul.cleanup":
		return c.cleanupBackhaul(ctx, j, *st)
	case "node.external_egress.apply":
		return c.applyExternalEgress(ctx, j, *st)
	case "node.external_egress.probe":
		return c.probeExternalEgress(ctx, j, *st)
	case "node.external_egress.cleanup":
		return c.cleanupExternalEgress(ctx, j, *st)
	case "node.route_policy.apply":
		return c.applyRoutePolicy(ctx, j, *st)
	case "node.route_policy.cleanup":
		return c.cleanupRoutePolicy(ctx, j, *st)
	case "node.firewall.preview", "node.firewall.apply", "node.firewall.observe", "node.firewall.disable":
		return c.handleNodeFirewallJob(ctx, j, *st)
	case "instance.diagnose":
		return c.handleInstanceDiagnoseJob(ctx, j)
	case "instance.restart", "instance.apply", "instance.start", "instance.stop", "instance.enable", "instance.disable", "instance.delete":
		return c.handleInstanceJob(ctx, j)
	default:
		return "failed", map[string]any{"error": "job type is not whitelisted for agent"}
	}
}
