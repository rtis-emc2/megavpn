import { describe, expect, it } from 'vitest';
import type { EnrollmentToken, NodeBootstrapRun, NodeDetail, NodeDiagnostics, NodeInventorySnapshot } from '../../shared/api/types';
import { deriveNodeOnboardingModel } from './nodeOnboarding';

const node: NodeDetail = {
  id: 'node-1',
  name: 'Edge One',
  status: 'online',
  address: '203.0.113.10',
  execution_mode: 'agent_managed',
  created_at: '2026-07-09T08:00:00Z',
  updated_at: '2026-07-09T08:01:00Z',
};

const activeToken: EnrollmentToken = {
  id: 'token-1',
  node_id: 'node-1',
  token_hint: 'enro...hint',
  status: 'active',
  expires_at: '2026-07-10T08:00:00Z',
  created_at: '2026-07-09T08:02:00Z',
};

const queuedRun: NodeBootstrapRun = {
  id: 'run-queued',
  node_id: 'node-1',
  status: 'queued',
  bootstrap_mode: 'ssh_bootstrap',
  created_at: '2026-07-09T08:03:00Z',
};

const runningRun: NodeBootstrapRun = {
  ...queuedRun,
  id: 'run-running',
  status: 'running',
  created_at: '2026-07-09T08:04:00Z',
};

const successRun: NodeBootstrapRun = {
  ...queuedRun,
  id: 'run-success',
  status: 'succeeded',
  finished_at: '2026-07-09T08:06:00Z',
  created_at: '2026-07-09T08:05:00Z',
};

const failedRun: NodeBootstrapRun = {
  ...queuedRun,
  id: 'run-failed',
  status: 'failed',
  finished_at: '2026-07-09T08:07:00Z',
  created_at: '2026-07-09T08:06:00Z',
};

const inventory: NodeInventorySnapshot = {
  id: 'inventory-1',
  node_id: 'node-1',
  created_at: '2026-07-09T08:09:00Z',
};

function diagnostics(overrides: Partial<NodeDiagnostics> = {}): NodeDiagnostics {
  return {
    node,
    heartbeat_state: 'unknown',
    communication_state: 'missing_agent_identity',
    agent: {
      node_id: 'node-1',
      status: 'missing',
      token_rotation_status: 'missing',
    },
    ...overrides,
  };
}

function registeredDiagnostics(overrides: Partial<NodeDiagnostics> = {}): NodeDiagnostics {
  return diagnostics({
    heartbeat_state: 'unknown',
    communication_state: 'healthy',
    agent: {
      node_id: 'node-1',
      status: 'active',
      agent_version: '8.0.0-agent',
      protocol_version: 'v1',
      registered_at: '2026-07-09T08:07:00Z',
      token_rotation_status: 'active',
    },
    ...overrides,
  });
}

function stepStatus(model: ReturnType<typeof deriveNodeOnboardingModel>, key: string) {
  return model.steps.find((step) => step.key === key)?.status;
}

function stepEvidence(model: ReturnType<typeof deriveNodeOnboardingModel>, key: string) {
  return model.steps.find((step) => step.key === key)?.evidenceCode;
}

describe('node onboarding model', () => {
  it('classifies empty safe data as not started and profile as current', () => {
    const model = deriveNodeOnboardingModel({});

    expect(model.overallStatus).toBe('not_started');
    expect(model.currentStep).toBe('profile');
    expect(stepStatus(model, 'profile')).toBe('current');
  });

  it('completes the profile step from a loaded node and blocks retired nodes', () => {
    const loaded = deriveNodeOnboardingModel({ node });
    expect(stepStatus(loaded, 'profile')).toBe('complete');
    expect(stepEvidence(loaded, 'profile')).toBe('profile_loaded');

    const retired = deriveNodeOnboardingModel({ node: { ...node, status: 'retired' } });
    expect(retired.overallStatus).toBe('blocked');
    expect(retired.currentStep).toBe('profile');
    expect(retired.steps.every((step) => step.status === 'blocked')).toBe(true);
  });

  it('advances credential with an active redacted enrollment token and ignores plaintext token fields', () => {
    const model = deriveNodeOnboardingModel({
      node,
      enrollmentTokens: [{ ...activeToken, token: 'plain-secret-token', enrollment_token: 'enroll-secret-token' }],
    });

    expect(stepStatus(model, 'credential')).toBe('complete');
    expect(stepEvidence(model, 'credential')).toBe('credential_active_token');
    expect(model.overallStatus).toBe('in_progress');
    expect(JSON.stringify(model)).not.toContain('plain-secret-token');
    expect(JSON.stringify(model)).not.toContain('enroll-secret-token');
  });

  it('treats active token without registration as awaiting registration', () => {
    const model = deriveNodeOnboardingModel({ node, diagnostics: diagnostics({ active_enrollment_token: activeToken }), enrollmentTokens: [] });

    expect(stepStatus(model, 'credential')).toBe('complete');
    expect(stepStatus(model, 'registration')).toBe('current');
    expect(model.currentStep).toBe('registration');
    expect(model.overallStatus).toBe('in_progress');
  });

  it('maps queued and running bootstrap to the current bootstrap step', () => {
    const queued = deriveNodeOnboardingModel({ node, bootstrapRuns: [queuedRun] });
    expect(stepStatus(queued, 'bootstrap')).toBe('current');
    expect(stepEvidence(queued, 'bootstrap')).toBe('bootstrap_queued');

    const running = deriveNodeOnboardingModel({ node, bootstrapRuns: [runningRun] });
    expect(stepStatus(running, 'bootstrap')).toBe('current');
    expect(stepEvidence(running, 'bootstrap')).toBe('bootstrap_running');
  });

  it('completes successful bootstrap and keeps older failures from overriding a newer success', () => {
    const model = deriveNodeOnboardingModel({ node, bootstrapRuns: [failedRun, { ...successRun, created_at: '2026-07-09T08:08:00Z' }] });

    expect(stepStatus(model, 'bootstrap')).toBe('complete');
    expect(stepEvidence(model, 'bootstrap')).toBe('bootstrap_successful');
    expect(model.overallStatus).toBe('in_progress');
  });

  it('maps latest failed bootstrap to warning and action required', () => {
    const model = deriveNodeOnboardingModel({ node, bootstrapRuns: [successRun, failedRun] });

    expect(stepStatus(model, 'bootstrap')).toBe('warning');
    expect(model.overallStatus).toBe('action_required');
  });

  it('completes registration from registered_at and treats revoked agent as action required', () => {
    const registered = deriveNodeOnboardingModel({ node, diagnostics: registeredDiagnostics() });
    expect(stepStatus(registered, 'registration')).toBe('complete');
    expect(registered.registered).toBe(true);

    const revoked = deriveNodeOnboardingModel({
      node,
      diagnostics: registeredDiagnostics({
        communication_state: 'auth_failure',
        agent: {
          node_id: 'node-1',
          status: 'revoked',
          registered_at: '2026-07-09T08:07:00Z',
          revoked_at: '2026-07-09T08:08:00Z',
          token_rotation_status: 'revoked',
        },
      }),
    });
    expect(stepStatus(revoked, 'registration')).toBe('warning');
    expect(revoked.overallStatus).toBe('action_required');
  });

  it('keeps heartbeat current after registration until a heartbeat is observed', () => {
    const model = deriveNodeOnboardingModel({ node, diagnostics: registeredDiagnostics() });

    expect(stepStatus(model, 'heartbeat')).toBe('current');
    expect(model.heartbeatObserved).toBe(false);
  });

  it('completes online heartbeat and warns on degraded or offline heartbeat', () => {
    const online = deriveNodeOnboardingModel({
      node: { ...node, last_heartbeat_at: '2026-07-09T08:08:00Z' },
      diagnostics: registeredDiagnostics({ heartbeat_state: 'online' }),
    });
    expect(stepStatus(online, 'heartbeat')).toBe('complete');

    const degraded = deriveNodeOnboardingModel({
      node: { ...node, last_heartbeat_at: '2026-07-09T08:08:00Z' },
      diagnostics: registeredDiagnostics({ heartbeat_state: 'degraded', communication_state: 'degraded' }),
    });
    expect(stepStatus(degraded, 'heartbeat')).toBe('warning');

    const offline = deriveNodeOnboardingModel({
      node: { ...node, last_heartbeat_at: '2026-07-09T08:08:00Z' },
      diagnostics: registeredDiagnostics({ heartbeat_state: 'offline', communication_state: 'channel_offline' }),
    });
    expect(stepStatus(offline, 'heartbeat')).toBe('warning');
    expect(offline.overallStatus).toBe('action_required');
  });

  it('maps unhealthy communication states conservatively', () => {
    for (const state of ['heartbeat_stalled', 'channel_offline', 'auth_failure', 'job_result_stalled']) {
      const model = deriveNodeOnboardingModel({
        node: { ...node, last_heartbeat_at: '2026-07-09T08:08:00Z' },
        diagnostics: registeredDiagnostics({ heartbeat_state: 'offline', communication_state: state }),
      });
      expect(model.overallStatus).toBe('action_required');
    }
  });

  it('keeps job_running in progress', () => {
    const model = deriveNodeOnboardingModel({
      node: { ...node, last_heartbeat_at: '2026-07-09T08:08:00Z' },
      diagnostics: registeredDiagnostics({ heartbeat_state: 'online', communication_state: 'job_running' }),
    });

    expect(model.overallStatus).toBe('in_progress');
  });

  it('completes inventory from snapshot or last_inventory_sync_at but not from a queued job alone', () => {
    const snapshot = deriveNodeOnboardingModel({
      node: { ...node, last_heartbeat_at: '2026-07-09T08:08:00Z' },
      diagnostics: registeredDiagnostics({ heartbeat_state: 'online', latest_inventory: inventory }),
    });
    expect(stepStatus(snapshot, 'inventory')).toBe('complete');

    const lastSync = deriveNodeOnboardingModel({
      node: { ...node, last_heartbeat_at: '2026-07-09T08:08:00Z' },
      diagnostics: registeredDiagnostics({
        heartbeat_state: 'online',
        agent: {
          node_id: 'node-1',
          status: 'active',
          registered_at: '2026-07-09T08:07:00Z',
          last_inventory_sync_at: '2026-07-09T08:09:00Z',
          token_rotation_status: 'active',
        },
      }),
    });
    expect(stepStatus(lastSync, 'inventory')).toBe('complete');

    const queuedOnly = deriveNodeOnboardingModel({
      node: { ...node, last_heartbeat_at: '2026-07-09T08:08:00Z' },
      diagnostics: registeredDiagnostics({
        heartbeat_state: 'online',
        communication_state: 'job_running',
        agent: {
          node_id: 'node-1',
          status: 'active',
          registered_at: '2026-07-09T08:07:00Z',
          last_job_claim_type: 'node.inventory.sync',
          token_rotation_status: 'active',
        },
      }),
    });
    expect(stepStatus(queuedOnly, 'inventory')).toBe('current');
    expect(queuedOnly.inventoryObserved).toBe(false);
  });

  it('requires registration, heartbeat and inventory evidence before ready', () => {
    for (const state of ['inventory_ok', 'discovery_ok', 'healthy']) {
      const ready = deriveNodeOnboardingModel({
        node: { ...node, last_heartbeat_at: '2026-07-09T08:08:00Z' },
        diagnostics: registeredDiagnostics({ heartbeat_state: 'online', communication_state: state, latest_inventory: inventory }),
      });
      expect(ready.overallStatus).toBe('ready');
    }

    const communicationOnly = deriveNodeOnboardingModel({
      node,
      diagnostics: diagnostics({ communication_state: 'healthy' }),
    });
    expect(communicationOnly.overallStatus).not.toBe('ready');
  });

  it('marks complete milestones degraded when current communication is degraded', () => {
    const model = deriveNodeOnboardingModel({
      node: { ...node, last_heartbeat_at: '2026-07-09T08:08:00Z' },
      diagnostics: registeredDiagnostics({ heartbeat_state: 'degraded', communication_state: 'degraded', latest_inventory: inventory }),
    });

    expect(model.overallStatus).toBe('degraded');
  });

  it('keeps unknown backend statuses safely pending and does not mutate source arrays', () => {
    const runs = [successRun, { ...queuedRun, id: 'run-unknown', status: 'mystery', created_at: '2026-07-09T08:10:00Z' }];
    const tokens = [{ ...activeToken }];
    const runOrder = runs.map((run) => run.id).join(',');
    const tokenOrder = tokens.map((token) => token.id).join(',');
    const model = deriveNodeOnboardingModel({ node, enrollmentTokens: tokens, bootstrapRuns: runs });

    expect(stepStatus(model, 'bootstrap')).toBe('pending');
    expect(stepEvidence(model, 'bootstrap')).toBe('bootstrap_unknown_status');
    expect(runs.map((run) => run.id).join(',')).toBe(runOrder);
    expect(tokens.map((token) => token.id).join(',')).toBe(tokenOrder);
  });
});
