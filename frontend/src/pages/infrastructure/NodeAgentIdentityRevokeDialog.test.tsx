import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { APIError } from '../../shared/api/client';
import type { NodeDetail, NodeDiagnostics } from '../../shared/api/types';
import i18n from '../../shared/i18n';
import { NodeAgentIdentityRevokeDialog } from './NodeAgentIdentityRevokeDialog';

const node: NodeDetail = {
  id: 'node-1',
  name: 'Edge One',
  status: 'online',
  agent_status: 'active',
  agent_last_seen_at: '2026-07-14T08:00:00Z',
};

const diagnostics: NodeDiagnostics = {
  heartbeat_state: 'healthy',
  communication_state: 'connected',
  agent: {
    node_id: 'node-1',
    status: 'active',
    token_rotation_status: 'active',
    token_hint: 'agent-token-hint-not-rendered',
    secret_ref_id: 'secret-ref-not-rendered',
    last_seen_at: '2026-07-14T08:00:00Z',
  },
};

function renderDialog(overrides: Partial<Parameters<typeof NodeAgentIdentityRevokeDialog>[0]> = {}) {
  const onCancel = vi.fn();
  const onConfirm = vi.fn().mockResolvedValue(undefined);
  const props = {
    open: true,
    node,
    diagnostics,
    pending: false,
    error: undefined,
    canBootstrapNode: true,
    onCancel,
    onConfirm,
    ...overrides,
  };
  const result = render(<NodeAgentIdentityRevokeDialog {...props} />);
  return { ...result, onCancel, onConfirm };
}

describe('NodeAgentIdentityRevokeDialog', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
    window.localStorage.clear();
    window.sessionStorage.clear();
  });

  it('renders accurate destructive warnings without token-like diagnostic material', () => {
    renderDialog();

    expect(screen.getByRole('dialog', { name: 'Revoke agent identity' })).toBeInTheDocument();
    expect(screen.getByText('Invalidate the current agent identity.')).toBeInTheDocument();
    expect(screen.getByText('Prevent the current agent token from authenticating.')).toBeInTheDocument();
    expect(screen.getByText('Revoke active unused enrollment tokens for this node.')).toBeInTheDocument();
    expect(screen.getByText('Require explicit issuance of a new enrollment token before re-enrollment.')).toBeInTheDocument();
    expect(screen.getByText('Preserve the node record.')).toBeInTheDocument();
    expect(screen.getByText('Preserve inventory and audit history.')).toBeInTheDocument();
    expect(screen.getByText('Preserve queued jobs for later operator review.')).toBeInTheDocument();
    expect(screen.getByText('Uninstall the agent.')).toBeInTheDocument();
    expect(screen.getByText('Stop the agent service directly.')).toBeInTheDocument();
    expect(screen.getByText('Automatically create a replacement enrollment token.')).toBeInTheDocument();
    expect(screen.getByText('Automatically re-enroll the node.')).toBeInTheDocument();

    const body = document.body.textContent || '';
    expect(body).not.toContain('agent-token-hint-not-rendered');
    expect(body).not.toContain('secret-ref-not-rendered');
    expect(body).not.toContain('token_hint');
    expect(body).not.toContain('secret_ref');
  });

  it('validates exact confirmation, reason and acknowledgement before submitting typed input', async () => {
    const { onConfirm } = renderDialog();
    const submit = screen.getByRole('button', { name: 'Revoke identity' });

    expect(submit).toBeDisabled();
    await userEvent.type(screen.getByLabelText('Typed node name'), 'edge one');
    await userEvent.type(screen.getByLabelText('Operator reason'), 'bad');
    await userEvent.click(screen.getByLabelText(/I understand that identity revocation/));
    expect(screen.getByText('Confirmation must match the selected node name exactly.')).toBeInTheDocument();
    expect(screen.getByText('Reason must be at least 5 characters.')).toBeInTheDocument();
    expect(submit).toBeDisabled();

    await userEvent.clear(screen.getByLabelText('Typed node name'));
    await userEvent.type(screen.getByLabelText('Typed node name'), ' Edge One ');
    await userEvent.clear(screen.getByLabelText('Operator reason'));
    await userEvent.type(screen.getByLabelText('Operator reason'), ' incident response ');
    expect(submit).toBeEnabled();
    await userEvent.click(submit);

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(onConfirm).toHaveBeenCalledWith({ confirmation: 'Edge One', reason: 'incident response' });
  });

  it('rejects unsafe reasons and keeps acknowledgement UI-only', async () => {
    const { onConfirm } = renderDialog();
    await userEvent.type(screen.getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(screen.getByLabelText('Operator reason'), 'Authorization: Bearer token');
    await userEvent.click(screen.getByLabelText(/I understand that identity revocation/));

    expect(screen.getByRole('button', { name: 'Revoke identity' })).toBeDisabled();
    expect(screen.getByText('Reason must not contain tokens, credentials, headers, secret references or request-body-like content.')).toBeInTheDocument();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('prevents duplicate submit while the async request is active', async () => {
    let resolveRequest: (() => void) | undefined;
    const onConfirm = vi.fn(() => new Promise<void>((resolve) => {
      resolveRequest = resolve;
    }));
    renderDialog({ onConfirm });

    await userEvent.type(screen.getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(screen.getByLabelText('Operator reason'), 'incident response');
    await userEvent.click(screen.getByLabelText(/I understand that identity revocation/));
    await userEvent.dblClick(screen.getByRole('button', { name: 'Revoke identity' }));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Revoking identity' })).toBeDisabled();
    resolveRequest?.();
  });

  it('uses safe backend error text and clears confirmation on backend mismatch', () => {
    renderDialog({
      error: new APIError('raw SQL token_hash secret_ref Authorization: Bearer', 409, {
        code: 'node_agent_revoke_confirmation_mismatch',
        error: 'raw SQL token_hash secret_ref Authorization: Bearer',
      }),
    });

    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('Backend rejected the confirmation. Type the current node name again.')).toBeInTheDocument();
    expect(dialog).not.toHaveTextContent('raw SQL');
    expect(dialog).not.toHaveTextContent('token_hash');
    expect(dialog).not.toHaveTextContent('Authorization: Bearer');
  });

  it('clears transient form state when closed and does not write browser storage', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const { rerender } = renderDialog();
    await userEvent.type(screen.getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(screen.getByLabelText('Operator reason'), 'incident response');
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    rerender(
      <NodeAgentIdentityRevokeDialog
        open={false}
        node={node}
        diagnostics={diagnostics}
        pending={false}
        canBootstrapNode={true}
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );
    rerender(
      <NodeAgentIdentityRevokeDialog
        open
        node={node}
        diagnostics={diagnostics}
        pending={false}
        canBootstrapNode={true}
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    await waitFor(() => expect(screen.getByLabelText('Typed node name')).toHaveValue(''));
    expect(screen.getByLabelText('Operator reason')).toHaveValue('');
    expect(storageSet).not.toHaveBeenCalledWith(expect.stringMatching(/agent|token|revoke|reason|confirmation/i), expect.anything());
    expect(window.localStorage.getItem('node_agent_revoke')).toBeNull();
    expect(window.sessionStorage.getItem('node_agent_revoke')).toBeNull();
  });
});
