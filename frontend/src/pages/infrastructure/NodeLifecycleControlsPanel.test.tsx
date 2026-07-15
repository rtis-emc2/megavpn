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
      onOpenRevokeDialog={onOpenRevokeDialog}
      onRefreshStaleRotationPreview={onRefreshStaleRotationPreview}
      {...overrides}
    />,
  );
  return { onOpenRevokeDialog, onRefreshStaleRotationPreview, rerender: result.rerender };
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
    expect(screen.getByRole('button', { name: 'Revoke agent identity' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /reboot/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /emergency cleanup/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /clear/i })).not.toBeInTheDocument();
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
        onOpenRevokeDialog={vi.fn()}
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
        onOpenRevokeDialog={vi.fn()}
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
        onOpenRevokeDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('Stale-rotation preview requires node.read permission.');
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
        onOpenRevokeDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('Permission required: node.read');
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
    const table = screen.getByRole('table');
    expect(within(table).getAllByText('Yes').length).toBeGreaterThan(0);
    await userEvent.click(screen.getByRole('button', { name: 'Refresh preview' }));
    expect(onRefreshStaleRotationPreview).toHaveBeenCalledTimes(1);
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
        onOpenRevokeDialog={vi.fn()}
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
        onOpenRevokeDialog={vi.fn()}
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
        onOpenRevokeDialog={vi.fn()}
        onRefreshStaleRotationPreview={vi.fn()}
      />,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('Permission required: node.bootstrap');
    expect(screen.queryByRole('button', { name: 'Revoke agent identity' })).not.toBeInTheDocument();
  });
});
