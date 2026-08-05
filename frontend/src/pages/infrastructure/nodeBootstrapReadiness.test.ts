import { describe, expect, it } from 'vitest';
import type { NodeAccessMethod, NodeBootstrapRun, NodeDetail } from '../../shared/api/types';
import { deriveNodeBootstrapReadiness, NODE_BOOTSTRAP_MODES } from './nodeBootstrapReadiness';

const node: NodeDetail = {
  id: 'node-1',
  name: 'Edge One',
  status: 'online',
  execution_mode: 'ssh_bootstrap',
  created_at: '2026-07-09T08:00:00Z',
};

const sshMethod: NodeAccessMethod = {
  id: 'ssh-1',
  node_id: 'node-1',
  method: 'ssh',
  is_enabled: true,
  ssh_host: 'edge-one.example.test',
  ssh_port: 22,
  ssh_user: 'ubuntu',
  ssh_host_key_sha256: 'SHA256:pinned',
  secret_configured: true,
  secret_ref_id: 'opaque-value-not-for-ui',
};

const queuedRun: NodeBootstrapRun = {
  id: 'run-queued',
  node_id: 'node-1',
  status: 'queued',
  bootstrap_mode: 'ssh_bootstrap',
  created_at: '2026-07-09T08:02:00Z',
};

const successRun: NodeBootstrapRun = {
  id: 'run-success',
  node_id: 'node-1',
  status: 'succeeded',
  bootstrap_mode: 'manual_bundle',
  created_at: '2026-07-09T08:04:00Z',
  finished_at: '2026-07-09T08:05:00Z',
};

const failedRun: NodeBootstrapRun = {
  id: 'run-failed',
  node_id: 'node-1',
  status: 'failed',
  bootstrap_mode: 'ssh_bootstrap',
  created_at: '2026-07-09T08:06:00Z',
  finished_at: '2026-07-09T08:07:00Z',
};

function mode(reason: ReturnType<typeof deriveNodeBootstrapReadiness>, name: 'ssh_bootstrap' | 'manual_bundle') {
  const found = reason.modes.find((item) => item.mode === name);
  if (!found) throw new Error(`missing mode ${name}`);
  return found;
}

describe('node bootstrap readiness', () => {
  it('supports exactly the backend bootstrap modes', () => {
    expect(NODE_BOOTSTRAP_MODES).toEqual(['ssh_bootstrap', 'manual_bundle']);
  });

  it('blocks retired, deleted and local-managed nodes', () => {
    for (const status of ['retired', 'deleted']) {
      const readiness = deriveNodeBootstrapReadiness({ node: { ...node, status }, accessMethods: [sshMethod] });
      expect(readiness.modes.every((item) => !item.available)).toBe(true);
      expect(readiness.modes.every((item) => item.reasonCode === 'node_inactive')).toBe(true);
    }
    const local = deriveNodeBootstrapReadiness({ node: { ...node, execution_mode: 'local_managed' }, accessMethods: [sshMethod] });
    expect(local.modes.every((item) => !item.available)).toBe(true);
    expect(local.modes.every((item) => item.reasonCode === 'local_managed')).toBe(true);
  });

  it('derives SSH readiness from safe SSH prerequisites', () => {
    const ready = deriveNodeBootstrapReadiness({ node, accessMethods: [sshMethod] });
    expect(mode(ready, 'ssh_bootstrap')).toMatchObject({ available: true, reasonCode: 'available' });
    expect(mode(ready, 'ssh_bootstrap').sshTarget).toEqual({
      host: 'edge-one.example.test',
      port: 22,
      user: 'ubuntu',
      enabled: true,
      hostKeyConfigured: true,
      secretConfigured: true,
    });

    expect(mode(deriveNodeBootstrapReadiness({ node, accessMethods: [{ ...sshMethod, is_enabled: false }] }), 'ssh_bootstrap').reasonCode).toBe('ssh_access_disabled');
    expect(mode(deriveNodeBootstrapReadiness({ node, accessMethods: [{ ...sshMethod, ssh_host: '' }] }), 'ssh_bootstrap').reasonCode).toBe('ssh_host_missing');
    expect(mode(deriveNodeBootstrapReadiness({ node, accessMethods: [{ ...sshMethod, ssh_user: '' }] }), 'ssh_bootstrap').reasonCode).toBe('ssh_user_missing');
    expect(mode(deriveNodeBootstrapReadiness({ node, accessMethods: [{ ...sshMethod, ssh_host_key_sha256: '' }] }), 'ssh_bootstrap').reasonCode).toBe('ssh_host_key_missing');
    expect(mode(deriveNodeBootstrapReadiness({ node, accessMethods: [{ ...sshMethod, secret_configured: false, secret_ref_id: undefined }] }), 'ssh_bootstrap').reasonCode).toBe('ssh_credential_missing');
  });

  it('does not return secret reference identifiers in readiness output', () => {
    const readiness = deriveNodeBootstrapReadiness({ node, accessMethods: [sshMethod] });
    expect(JSON.stringify(readiness)).not.toContain('opaque-value-not-for-ui');
  });

  it('allows manual bundle without SSH and blocks duplicate active bootstrap', () => {
    const manual = deriveNodeBootstrapReadiness({ node, accessMethods: [] });
    expect(mode(manual, 'manual_bundle')).toMatchObject({ available: true, reasonCode: 'available' });
    expect(mode(manual, 'ssh_bootstrap').available).toBe(false);

    const active = deriveNodeBootstrapReadiness({ node, accessMethods: [sshMethod], bootstrapRuns: [queuedRun] });
    expect(active.hasRunningBootstrap).toBe(true);
    expect(active.modes.every((item) => !item.available)).toBe(true);
    expect(active.modes.every((item) => item.reasonCode === 'bootstrap_already_active')).toBe(true);
  });

  it('fails closed on unknown bootstrap state', () => {
    const unknown = deriveNodeBootstrapReadiness({
      node,
      accessMethods: [sshMethod],
      bootstrapRuns: [{ ...queuedRun, id: 'run-unknown', status: 'mystery', created_at: '2026-07-09T08:10:00Z' }],
    });

    expect(unknown.hasUnknownBootstrap).toBe(true);
    expect(unknown.modes.every((item) => !item.available)).toBe(true);
    expect(unknown.modes.every((item) => item.reasonCode === 'bootstrap_state_unknown')).toBe(true);
    expect(unknown.defaultMode).toBeUndefined();
  });

  it('selects latest runs deterministically and recognizes newer failures', () => {
    const runs = [successRun, failedRun];
    const originalOrder = runs.map((run) => run.id).join(',');
    const readiness = deriveNodeBootstrapReadiness({ node, accessMethods: [sshMethod], bootstrapRuns: runs });
    expect(readiness.latestRun?.id).toBe('run-failed');
    expect(readiness.latestSuccessfulRun?.id).toBe('run-success');
    expect(readiness.latestFailedRun?.id).toBe('run-failed');
    expect(runs.map((run) => run.id).join(',')).toBe(originalOrder);
  });

  it('keeps newer success from being overridden by older failure', () => {
    const readiness = deriveNodeBootstrapReadiness({
      node,
      accessMethods: [sshMethod],
      bootstrapRuns: [failedRun, { ...successRun, created_at: '2026-07-09T08:08:00Z', finished_at: '2026-07-09T08:09:00Z' }],
    });

    expect(readiness.latestRun?.id).toBe('run-success');
    expect(readiness.latestSuccessfulRun?.id).toBe('run-success');
    expect(readiness.latestFailedRun?.id).toBe('run-failed');
  });

  it('recommends conservatively by execution mode and requires selection for agent managed nodes', () => {
    const ssh = deriveNodeBootstrapReadiness({ node: { ...node, execution_mode: 'ssh_bootstrap' }, accessMethods: [sshMethod] });
    expect(ssh.defaultMode).toBe('ssh_bootstrap');
    expect(mode(ssh, 'ssh_bootstrap').recommended).toBe(true);

    const manual = deriveNodeBootstrapReadiness({ node: { ...node, execution_mode: 'manual_bundle' }, accessMethods: [sshMethod] });
    expect(manual.defaultMode).toBe('manual_bundle');
    expect(mode(manual, 'manual_bundle').recommended).toBe(true);

    const agent = deriveNodeBootstrapReadiness({ node: { ...node, execution_mode: 'agent_managed' }, accessMethods: [sshMethod] });
    expect(agent.defaultMode).toBeUndefined();
    expect(agent.selectionRequired).toBe(true);
  });
});
