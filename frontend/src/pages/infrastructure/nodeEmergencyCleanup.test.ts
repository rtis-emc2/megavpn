import { describe, expect, it } from 'vitest';
import { APIError } from '../../shared/api/client';
import type { NodeDetail, NodeDiagnostics, NodeEmergencyCleanupInput } from '../../shared/api/types';
import {
  deriveNodeEmergencyCleanupActionState,
  nodeEmergencyCleanupErrorKey,
  reasonLooksUnsafeForNodeEmergencyCleanup,
  resetNodeEmergencyCleanupScopeRelationship,
  validateNodeEmergencyCleanupForm,
  validateQueuedNodeEmergencyCleanupResult,
  type NodeEmergencyCleanupForm,
} from './nodeEmergencyCleanup';

const node: NodeDetail = {
  id: 'node-1',
  name: 'Edge One',
  status: 'maintenance',
  agent_status: 'active',
  agent_channel_status: 'connected',
};

const diagnostics: NodeDiagnostics = {
  node,
  heartbeat_state: 'healthy',
  communication_state: 'connected',
  agent: { node_id: 'node-1', status: 'active' },
};

const servicesForm: NodeEmergencyCleanupForm = {
  cleanupScope: 'services_only',
  includeAgent: false,
  confirmation: ' Edge One ',
  reason: ' maintenance window ',
  acknowledgeDestructiveCleanup: true,
  acknowledgeAgentRemoval: false,
};

function resultFor(input: NodeEmergencyCleanupInput): unknown {
  return {
    status: 'queued',
    message: 'raw backend message is ignored',
    job: {
      id: 'job-cleanup-1',
      type: 'node.emergency_cleanup',
      status: 'queued',
      node_id: 'node-1',
      scope_id: 'node-1',
      created_at: '2026-07-16T08:00:00Z',
      payload: { private_key: 'must-not-copy' },
      result: { command_output: 'must-not-copy' },
    },
    plan_summary: {
      cleanup_scope: input.cleanup_scope,
      include_agent: input.include_agent,
      instance_target_count: 3,
      service_counts: { openvpn: 1, xray: 2 },
      node_runtime_cleanup: input.cleanup_scope === 'full_node',
      agent_removal_requested: input.include_agent,
    },
  };
}

describe('Emergency Cleanup form validation', () => {
  it('creates the exact normalized services-only input', () => {
    const validation = validateNodeEmergencyCleanupForm(node, servicesForm);
    expect(validation).toEqual({
      valid: true,
      expectedConfirmation: 'Edge One',
      input: {
        cleanup_scope: 'services_only',
        include_agent: false,
        confirmation: 'Edge One',
        reason: 'maintenance window',
        acknowledge_destructive_cleanup: true,
        acknowledge_agent_removal: false,
      },
      errors: {},
    });
  });

  it('creates exact full-node input with and without agent removal', () => {
    const fullNode = validateNodeEmergencyCleanupForm(node, { ...servicesForm, cleanupScope: 'full_node' });
    expect(fullNode.input).toMatchObject({
      cleanup_scope: 'full_node',
      include_agent: false,
      acknowledge_agent_removal: false,
    });

    const withAgent = validateNodeEmergencyCleanupForm(node, {
      ...servicesForm,
      cleanupScope: 'full_node',
      includeAgent: true,
      acknowledgeAgentRemoval: true,
    });
    expect(withAgent.input).toEqual({
      cleanup_scope: 'full_node',
      include_agent: true,
      confirmation: 'Edge One',
      reason: 'maintenance window',
      acknowledge_destructive_cleanup: true,
      acknowledge_agent_removal: true,
    });
  });

  it('rejects missing, unsupported and legacy scope values and invalid agent relationships', () => {
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, cleanupScope: '' }).errors.cleanupScope).toBeTruthy();
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, cleanupScope: 'wipe' as 'services_only' }).errors.cleanupScope).toBeTruthy();
    const invalidAgent = validateNodeEmergencyCleanupForm(node, { ...servicesForm, includeAgent: true, acknowledgeAgentRemoval: true });
    expect(invalidAgent.errors.includeAgent).toBeTruthy();

    const reset = resetNodeEmergencyCleanupScopeRelationship({
      ...servicesForm,
      cleanupScope: 'full_node',
      includeAgent: true,
      acknowledgeAgentRemoval: true,
    }, 'services_only');
    expect(reset).toMatchObject({ cleanupScope: 'services_only', includeAgent: false, acknowledgeAgentRemoval: false });
  });

  it('enforces confirmation bounds, exact case, control-character and node-id fallback', () => {
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, confirmation: '' }).errors.confirmation).toBeTruthy();
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, confirmation: 'edge one' }).errors.confirmation).toBeTruthy();
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, confirmation: `${'x'.repeat(513)}` }).errors.confirmation).toBeTruthy();
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, confirmation: 'Edge\nOne' }).errors.confirmation).toBeTruthy();
    expect(validateNodeEmergencyCleanupForm({ id: 'node-1', name: '' }, { ...servicesForm, confirmation: 'node-1' }).valid).toBe(true);
  });

  it('enforces reason bounds and rejects control, credential and request markers', () => {
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, reason: '' }).errors.reason).toBeTruthy();
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, reason: 'bad' }).errors.reason).toBeTruthy();
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, reason: 'x'.repeat(501) }).errors.reason).toBeTruthy();
    for (const unsafe of [
      'line\nbreak',
      'Authorization: Basic value',
      'Bearer value',
      'agent_token value',
      'enrollment_token value',
      'token_hash value',
      'private_key value',
      'secret_ref value',
      'credential value',
      '{"request":"body"}',
    ]) {
      expect(reasonLooksUnsafeForNodeEmergencyCleanup(unsafe)).toBe(true);
      expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, reason: unsafe }).errors.reason).toBeTruthy();
    }
  });

  it('requires destructive acknowledgement and conditional agent-removal acknowledgement', () => {
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, acknowledgeDestructiveCleanup: false }).errors.acknowledgeDestructiveCleanup).toBeTruthy();
    expect(validateNodeEmergencyCleanupForm(node, {
      ...servicesForm,
      cleanupScope: 'full_node',
      includeAgent: true,
      acknowledgeAgentRemoval: false,
    }).errors.acknowledgeAgentRemoval).toBeTruthy();
    expect(validateNodeEmergencyCleanupForm(node, { ...servicesForm, acknowledgeAgentRemoval: true }).errors.acknowledgeAgentRemoval).toBeTruthy();
  });
});

describe('Emergency Cleanup lifecycle gating', () => {
  it('enables only a current maintenance node with active identity and available channel', () => {
    expect(deriveNodeEmergencyCleanupActionState({ node, diagnostics, canBootstrapNode: true, lifecycleDataCurrent: true })).toEqual({ available: true, unknownState: false });
    expect(deriveNodeEmergencyCleanupActionState({ node, diagnostics, canBootstrapNode: false, lifecycleDataCurrent: true }).blockedKey).toContain('permissionRequired');
    expect(deriveNodeEmergencyCleanupActionState({ node, diagnostics, canBootstrapNode: true, lifecycleDataCurrent: false }).blockedKey).toContain('lifecycleDataStale');
    expect(deriveNodeEmergencyCleanupActionState({ node: { ...node, status: 'online' }, diagnostics, canBootstrapNode: true, lifecycleDataCurrent: true }).blockedKey).toContain('maintenanceRequired');
    expect(deriveNodeEmergencyCleanupActionState({ node: { ...node, status: 'retired' }, diagnostics, canBootstrapNode: true, lifecycleDataCurrent: true }).blockedKey).toContain('terminalNode');
  });

  it('blocks missing, revoked, inactive, offline, auth-failed and ambiguous agent evidence', () => {
    const state = (overrides: Partial<NodeDiagnostics>) => deriveNodeEmergencyCleanupActionState({
      node,
      diagnostics: { ...diagnostics, ...overrides },
      canBootstrapNode: true,
      lifecycleDataCurrent: true,
    });
    expect(state({ agent: { node_id: 'node-1', status: 'missing' } }).blockedKey).toContain('identityMissing');
    expect(state({ agent: { node_id: 'node-1', status: 'revoked', revoked_at: '2026-07-16T08:00:00Z' } }).blockedKey).toContain('identityRevoked');
    expect(state({ agent: { node_id: 'node-1', status: 'pending' } }).blockedKey).toContain('agentUnavailable');
    expect(state({ communication_state: 'offline' }).blockedKey).toContain('channelUnavailable');
    expect(state({ communication_state: 'auth_failed' }).blockedKey).toContain('authenticationFailure');
    expect(state({ communication_state: 'unknown' }).blockedKey).toContain('stateIncomplete');
  });
});

describe('Emergency Cleanup response validation', () => {
  const input = validateNodeEmergencyCleanupForm(node, servicesForm).input as NodeEmergencyCleanupInput;

  it('copies only safe queued job metadata and validated summary fields', () => {
    const safe = validateQueuedNodeEmergencyCleanupResult(resultFor(input), 'node-1', input);
    expect(safe).toEqual({
      job: {
        id: 'job-cleanup-1',
        type: 'node.emergency_cleanup',
        status: 'queued',
        created_at: '2026-07-16T08:00:00Z',
        node_id: 'node-1',
        scope_id: 'node-1',
      },
      planSummary: {
        cleanup_scope: 'services_only',
        include_agent: false,
        instance_target_count: 3,
        service_counts: { openvpn: 1, xray: 2 },
        node_runtime_cleanup: false,
        agent_removal_requested: false,
      },
    });
    expect(JSON.stringify(safe)).not.toMatch(/private_key|command_output|payload|result/);
  });

  it('rejects wrong queue, job, ownership, scope, flags, counts and unsafe service identifiers', () => {
    const mutate = (change: (value: Record<string, any>) => void) => {
      const value = resultFor(input) as Record<string, any>;
      change(value);
      return validateQueuedNodeEmergencyCleanupResult(value, 'node-1', input);
    };
    expect(mutate((value) => { value.status = 'completed'; })).toBeNull();
    expect(mutate((value) => { value.job.type = 'node.reboot'; })).toBeNull();
    expect(mutate((value) => { value.job.status = 'running'; })).toBeNull();
    expect(mutate((value) => { value.job.id = ''; })).toBeNull();
    expect(mutate((value) => { value.job.node_id = 'node-2'; })).toBeNull();
    expect(mutate((value) => { value.job.node_id = 123; })).toBeNull();
    expect(mutate((value) => { value.job.scope_id = 'node-2'; })).toBeNull();
    expect(mutate((value) => { value.job.created_at = 'not-a-timestamp'; })).toBeNull();
    expect(mutate((value) => { value.plan_summary.cleanup_scope = 'full_node'; })).toBeNull();
    expect(mutate((value) => { value.plan_summary.include_agent = true; })).toBeNull();
    expect(mutate((value) => { value.plan_summary.agent_removal_requested = true; })).toBeNull();
    expect(mutate((value) => { value.plan_summary.node_runtime_cleanup = true; })).toBeNull();
    expect(mutate((value) => { value.plan_summary.instance_target_count = -1; })).toBeNull();
    expect(mutate((value) => { value.plan_summary.instance_target_count = 1.5; })).toBeNull();
    expect(mutate((value) => { value.plan_summary.service_counts.xray = -1; })).toBeNull();
    expect(mutate((value) => { value.plan_summary.service_counts['<script>'] = 1; })).toBeNull();
    expect(mutate((value) => { value.plan_summary.service_counts = []; })).toBeNull();
  });
});

describe('Emergency Cleanup error mapping', () => {
  it('maps exact backend codes and statuses without returning backend text', () => {
    const codes = [
      'node_emergency_cleanup_request_invalid',
      'node_emergency_cleanup_node_not_found',
      'node_emergency_cleanup_confirmation_mismatch',
      'node_emergency_cleanup_maintenance_required',
      'node_emergency_cleanup_agent_missing',
      'node_emergency_cleanup_agent_unavailable',
      'node_emergency_cleanup_scope_invalid',
      'node_emergency_cleanup_acknowledgement_required',
      'node_emergency_cleanup_plan_invalid',
      'node_emergency_cleanup_conflict',
      'node_emergency_cleanup_internal_error',
    ];
    for (const code of codes) {
      const key = nodeEmergencyCleanupErrorKey(new APIError('raw SQL token_hash', 409, { code, error: 'raw SQL token_hash' }));
      expect(key).toMatch(/^nodes\.lifecycleControls\.emergencyCleanup\.errors\./);
      expect(key).not.toContain('raw SQL');
    }
    expect(nodeEmergencyCleanupErrorKey(new APIError('forbidden', 403, {}))).toContain('permissionRequired');
    expect(nodeEmergencyCleanupErrorKey(new APIError('rate', 429, {}))).toContain('rateLimited');
    expect(nodeEmergencyCleanupErrorKey(new Error('network'))).toContain('generic');
  });
});
