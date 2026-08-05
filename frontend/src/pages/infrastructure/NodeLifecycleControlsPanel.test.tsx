import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ComponentProps } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { APIError } from '../../shared/api/client';
import i18n from '../../shared/i18n';
import type { NodeDetail, NodeDiagnostics, NodeStaleRotationPreview } from '../../shared/api/types';
import { NodeLifecycleControlsPanel } from './NodeLifecycleControlsPanel';

const node: NodeDetail = {
  id: 'node-1',
  name: 'Edge One',
  status: 'online',
  agent_status: 'online',
  agent_channel_status: 'connected',
};

const diagnostics: NodeDiagnostics = {
  heartbeat_state: 'healthy',
  communication_state: 'connected',
  agent: {
    node_id: 'node-1',
    status: 'active',
    token_hint: 'agent-token-hint-not-for-lifecycle-panel',
    token_rotation_status: 'active',
    last_job_result_job_id: 'job-last-result-safe-id',
  },
};

const preview: NodeStaleRotationPreview = {
  node_id: 'node-1',
  stale_rotation_detected: true,
  token_rotation_status: 'rotating',
  evaluated_at: '2026-07-14T08:00:00Z',
  candidates: [{
    job_id: 'job-stale-preview-1',
    status: 'running',
    created_at: '2026-07-14T07:45:00Z',
    started_at: '2026-07-14T07:46:00Z',
    last_claim_at: '2026-07-14T07:47:00Z',
    last_result_at: '2026-07-14T07:48:00Z',
    last_seen_at: '2026-07-14T07:30:00Z',
    last_poll_at: '2026-07-14T07:31:00Z',
    age_seconds: 900,
    stale_reason: 'claimed_without_result_and_agent_inactive',
    safe_to_clear: true,
  }],
};

function renderPanel(overrides: Partial<ComponentProps<typeof NodeLifecycleControlsPanel>> = {}) {
  const onRefreshStaleRotationPreview = vi.fn();
  const onOpenRevokeDialog = vi.fn();
  const onOpenRebootDialog = vi.fn();
  const onOpenEmergencyCleanupDialog = vi.fn();
  const onOpenStaleRotationClearDialog = vi.fn();
  const result = render(
    <NodeLifecycleControlsPanel
      node={node}
      diagnostics={diagnostics}
      staleRotationPreview={preview}
      staleRotationPreviewLoading={false}
      staleRotationPreviewFetching={false}
      staleRotationPreviewError={undefined}
      canReadNode
      canBootstrapNode
      lifecycleDataCurrent
      revokePending={false}
      rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
      onOpenRevokeDialog={onOpenRevokeDialog}
      onOpenRebootDialog={onOpenRebootDialog}
      onOpenEmergencyCleanupDialog={onOpenEmergencyCleanupDialog}
      onOpenStaleRotationClearDialog={onOpenStaleRotationClearDialog}
      onRefreshStaleRotationPreview={onRefreshStaleRotationPreview}
      {...overrides}
    />,
  );
  return { onOpenEmergencyCleanupDialog, onOpenRebootDialog, onOpenRevokeDialog, onOpenStaleRotationClearDialog, onRefreshStaleRotationPreview, rerender: result.rerender, unmount: result.unmount };
}

describe('NodeLifecycleControlsPanel', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
  });

  it('renders lifecycle status and stale rotation candidates without secret or payload fields', () => {
    renderPanel();

    expect(screen.getByRole('heading', { name: 'Lifecycle status' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Stale rotation preview' })).toBeInTheDocument();
    expect(screen.getAllByText('Claimed rotation without result while agent is inactive.').length).toBeGreaterThan(0);
    expect(screen.getAllByText('job-stal...').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Yes').length).toBeGreaterThan(0);
    const bodyText = document.body.textContent || '';
    expect(bodyText).not.toContain('agent-token-hint-not-for-lifecycle-panel');
    expect(bodyText).not.toContain('last_job_result_job_id');
    expect(bodyText).not.toContain('request_payload');
    expect(bodyText).not.toContain('result_payload');
    expect(bodyText).not.toContain('secret_ref');
    expect(screen.getByRole('button', { name: 'Queue reboot' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Revoke agent identity' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Emergency Cleanup' })).toBeInTheDocument();
    expect(screen.getByText('Emergency Cleanup requires the node to already be in maintenance mode. Use the existing maintenance control first.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /configure emergency cleanup/i })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Clear stale rotation state' })).toBeEnabled();
  });

  it('uses safe permission, loading, empty and error states', () => {
    const { rerender } = render(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={diagnostics}
        staleRotationPreview={undefined}
        staleRotationPreviewLoading
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('Loading stale-rotation preview')).toBeInTheDocument();

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={diagnostics}
        staleRotationPreview={{ ...preview, stale_rotation_detected: false, candidates: [] }}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('No stale rotation candidates')).toBeInTheDocument();

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={diagnostics}
        staleRotationPreview={undefined}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={new APIError('secret_ref raw backend message', 403, { error: 'secret_ref raw backend message' })}
        canReadNode
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('Stale-rotation preview requires node.read permission.')).toBeInTheDocument();
    expect(screen.queryByText(/secret_ref raw backend message/)).not.toBeInTheDocument();

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={diagnostics}
        staleRotationPreview={undefined}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode={false}
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('Permission required: node.read')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Refresh preview' })).toBeDisabled();
  });

  it('refreshes preview explicitly and flags unknown reasons without changing backend safe_to_clear', async () => {
    const { onRefreshStaleRotationPreview } = renderPanel({
      staleRotationPreview: {
        ...preview,
        candidates: [{ ...preview.candidates[0], stale_reason: 'new_backend_reason', safe_to_clear: true }],
      },
    });

    expect(screen.getAllByText('Backend returned an unrecognized stale-rotation reason.').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Unrecognized backend reason').length).toBeGreaterThan(0);
    expect(screen.getByText('A backend-safe candidate has an unrecognized reason. Cleanup is disabled until the contract is reviewed.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Clear stale rotation state' })).not.toBeInTheDocument();
    const table = screen.getByRole('table');
    expect(within(table).getAllByText('Yes').length).toBeGreaterThan(0);
    await userEvent.click(screen.getByRole('button', { name: 'Refresh preview' }));
    expect(onRefreshStaleRotationPreview).toHaveBeenCalledTimes(1);
  });

  it('offers the exact backend-safe remediation without maintenance or online-channel gating', async () => {
    const onOpenStaleRotationClearDialog = vi.fn();
    renderPanel({
      node: { ...node, status: 'offline' },
      diagnostics: { ...diagnostics, communication_state: 'offline' },
      onOpenStaleRotationClearDialog,
    });

    const button = screen.getByRole('button', { name: 'Clear stale rotation state' });
    expect(button).toBeEnabled();
    await userEvent.click(button);
    expect(onOpenStaleRotationClearDialog).toHaveBeenCalledTimes(1);
  });

  it('keeps preview readable but hides mutation without permission or a safe complete set', () => {
    const first = renderPanel({ canBootstrapNode: false });
    expect(screen.getAllByText('Claimed rotation without result while agent is inactive.').length).toBeGreaterThan(0);
    expect(screen.getByText('node.bootstrap permission is required to clear stale rotation state. The preview remains read-only.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Clear stale rotation state' })).not.toBeInTheDocument();
    first.unmount();

    renderPanel({
      staleRotationPreview: {
        ...preview,
        candidates: [{ ...preview.candidates[0], safe_to_clear: false, stale_reason: 'evidence_ambiguous' }],
      },
    });
    expect(screen.getByText('The backend currently reports no safe stale-rotation candidates. No cleanup action is available.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Clear stale rotation state' })).not.toBeInTheDocument();
  });

  it('gates node reboot by permission, identity, channel and terminal states', async () => {
    const { onOpenRebootDialog, rerender } = renderPanel();
    await userEvent.click(screen.getByRole('button', { name: 'Queue reboot' }));
    expect(onOpenRebootDialog).toHaveBeenCalledTimes(1);

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={{ ...diagnostics, agent: { ...(diagnostics.agent || {}), status: 'revoked' } }}
        staleRotationPreview={preview}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('Agent identity is revoked. Reboot queueing is disabled.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Queue reboot' })).not.toBeInTheDocument();

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={{ ...diagnostics, agent: { ...(diagnostics.agent || {}), status: 'missing' } }}
        staleRotationPreview={preview}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('No active agent identity is visible. Reboot queueing is disabled until identity state is recovered.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Queue reboot' })).not.toBeInTheDocument();

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={{ ...diagnostics, communication_state: 'offline' }}
        staleRotationPreview={preview}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('Agent communication is clearly unavailable. Reboot queueing is disabled until the channel recovers.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Queue reboot' })).not.toBeInTheDocument();

    rerender(
      <NodeLifecycleControlsPanel
        node={{ ...node, status: 'retired' }}
        diagnostics={diagnostics}
        staleRotationPreview={preview}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('This node is in a terminal or retired state. Reboot queueing is disabled.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Queue reboot' })).not.toBeInTheDocument();

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={{ ...diagnostics, communication_state: 'unknown' }}
        staleRotationPreview={preview}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('Current lifecycle state is incomplete; backend validation remains authoritative.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Queue reboot' })).toBeInTheDocument();

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={diagnostics}
        staleRotationPreview={preview}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode={false}
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('node.bootstrap permission is required to queue a node reboot.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Queue reboot' })).not.toBeInTheDocument();
  });

  it('gates Emergency Cleanup by maintenance, permission, current lifecycle, active identity and channel', async () => {
    const eligibleNode = { ...node, status: 'maintenance' };
    const { onOpenEmergencyCleanupDialog, rerender } = renderPanel({ node: eligibleNode });
    const open = screen.getByRole('button', { name: 'Configure Emergency Cleanup' });
    await userEvent.click(open);
    expect(onOpenEmergencyCleanupDialog).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Queue reboot' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Revoke agent identity' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Clear stale rotation state' })).toBeEnabled();

    const rerenderPanel = (overrides: {
      node?: NodeDetail;
      diagnostics?: NodeDiagnostics;
      canBootstrapNode?: boolean;
      lifecycleDataCurrent?: boolean;
    }) => rerender(
      <NodeLifecycleControlsPanel
        node={overrides.node || eligibleNode}
        diagnostics={overrides.diagnostics || diagnostics}
        staleRotationPreview={preview}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode={overrides.canBootstrapNode ?? true}
        lifecycleDataCurrent={overrides.lifecycleDataCurrent ?? true}
        revokePending={false}
        rebootPending={false}
        emergencyCleanupPending={false}
        staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );

    rerenderPanel({ canBootstrapNode: false });
    expect(screen.getByText('node.bootstrap permission is required to configure Emergency Cleanup.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Configure Emergency Cleanup' })).not.toBeInTheDocument();

    rerenderPanel({ lifecycleDataCurrent: false });
    expect(screen.getByText('Lifecycle data does not belong to the selected node. Refresh before configuring Emergency Cleanup.')).toBeInTheDocument();

    rerenderPanel({ diagnostics: { ...diagnostics, agent: { node_id: 'node-1', status: 'missing' } } });
    expect(screen.getByText('No active agent identity is visible. Emergency Cleanup queueing is disabled.')).toBeInTheDocument();

    rerenderPanel({ diagnostics: { ...diagnostics, agent: { node_id: 'node-1', status: 'revoked', revoked_at: '2026-07-16T08:00:00Z' } } });
    expect(screen.getByText('Agent identity is revoked. Emergency Cleanup cannot be delivered.')).toBeInTheDocument();

    rerenderPanel({ diagnostics: { ...diagnostics, communication_state: 'offline' } });
    expect(screen.getByText('Agent communication is clearly unavailable. Emergency Cleanup queueing is disabled.')).toBeInTheDocument();

    rerenderPanel({ diagnostics: { ...diagnostics, communication_state: 'auth_failed' } });
    expect(screen.getByText('Diagnostics show a clear agent authentication failure. Recover authentication before queueing.')).toBeInTheDocument();

    rerenderPanel({ diagnostics: { ...diagnostics, communication_state: 'unknown' } });
    expect(screen.getByText('Agent or communication state is incomplete or ambiguous. Emergency Cleanup remains disabled until current evidence is available.')).toBeInTheDocument();

    rerenderPanel({ node: { ...eligibleNode, status: 'retired' } });
    expect(screen.getByText('This node is retired, deleted or terminal. Emergency Cleanup queueing is disabled.')).toBeInTheDocument();

  });

  it('gates agent identity revoke by node.bootstrap and identity state only', async () => {
    const { onOpenRevokeDialog, rerender } = renderPanel();
    await userEvent.click(screen.getByRole('button', { name: 'Revoke agent identity' }));
    expect(onOpenRevokeDialog).toHaveBeenCalledTimes(1);

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={{ ...diagnostics, agent: { ...(diagnostics.agent || {}), status: 'revoked' } }}
        staleRotationPreview={preview}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('Agent identity is already revoked. Refresh diagnostics to inspect recovery state.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Revoke agent identity' })).not.toBeInTheDocument();

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={{ ...diagnostics, agent: { ...(diagnostics.agent || {}), status: 'missing' } }}
        staleRotationPreview={preview}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('No active agent identity is visible. Backend may still reject or confirm exact state after refresh.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Revoke agent identity' })).not.toBeInTheDocument();

    rerender(
      <NodeLifecycleControlsPanel
        node={node}
        diagnostics={diagnostics}
        staleRotationPreview={preview}
        staleRotationPreviewLoading={false}
        staleRotationPreviewFetching={false}
        staleRotationPreviewError={undefined}
        canReadNode
        canBootstrapNode={false}
        lifecycleDataCurrent
        revokePending={false}
        rebootPending={false}
      emergencyCleanupPending={false}
      staleRotationClearPending={false}
        onOpenRevokeDialog={vi.fn()}
        onOpenRebootDialog={vi.fn()}
        onOpenEmergencyCleanupDialog={vi.fn()}
        onOpenStaleRotationClearDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByText('Permission required: node.bootstrap')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Revoke agent identity' })).not.toBeInTheDocument();
  });
});
