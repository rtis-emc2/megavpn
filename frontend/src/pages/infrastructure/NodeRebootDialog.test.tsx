import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { APIError } from '../../shared/api/client';
import type { NodeDetail, NodeDiagnostics } from '../../shared/api/types';
import i18n from '../../shared/i18n';
import { NodeRebootDialog } from './NodeRebootDialog';

const node: NodeDetail = {
  id: 'node-1',
  name: 'Edge One',
  status: 'online',
  agent_status: 'active',
  agent_channel_status: 'connected',
};

const diagnostics: NodeDiagnostics = {
  heartbeat_state: 'healthy',
  communication_state: 'connected',
  communication_hint: 'raw-command-output-not-rendered',
  agent: {
    node_id: 'node-1',
    status: 'active',
    token_hint: 'agent-token-hint-not-rendered',
    secret_ref_id: 'secret-ref-not-rendered',
    last_seen_at: '2026-07-15T08:00:00Z',
  },
};

function renderDialog(overrides: Partial<Parameters<typeof NodeRebootDialog>[0]> = {}) {
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
  const result = render(<NodeRebootDialog {...props} />);
  return { ...result, onCancel, onConfirm };
}

describe('NodeRebootDialog', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
    window.localStorage.clear();
    window.sessionStorage.clear();
  });

  it('renders accessible asynchronous queue-only warnings without secret or command diagnostics', () => {
    renderDialog();

    const dialog = screen.getByRole('dialog', { name: 'Reboot node' });
    expect(dialog).toHaveTextContent('Queueing a reboot may interrupt traffic');
    expect(dialog).toHaveTextContent('Create an asynchronous node.reboot job.');
    expect(dialog).toHaveTextContent('May interrupt VPN and proxy traffic handled by this node.');
    expect(dialog).toHaveTextContent('May interrupt active administrative sessions.');
    expect(dialog).toHaveTextContent('Require manual investigation if the node does not return.');
    expect(dialog).toHaveTextContent('Prove that the reboot command executed.');
    expect(dialog).toHaveTextContent('Prove that a fresh heartbeat was received.');
    expect(dialog).toHaveTextContent('Revoke the agent identity.');
    expect(dialog).toHaveTextContent('Automatically change maintenance mode.');
    expect(dialog).toHaveTextContent('This confirms queueing only. It does not confirm that the node rebooted or returned online.');
    expect(dialog).toHaveTextContent('Edge One');
    expect(within(dialog).getByRole('button', { name: 'Queue reboot' })).toBeInTheDocument();

    const body = document.body.textContent || '';
    expect(body).not.toContain('agent-token-hint-not-rendered');
    expect(body).not.toContain('secret-ref-not-rendered');
    expect(body).not.toContain('raw-command-output-not-rendered');
    expect(body).not.toContain('token_hint');
    expect(body).not.toContain('secret_ref');
  });

  it('validates exact confirmation, reason and acknowledgement before submitting typed input', async () => {
    const { onConfirm } = renderDialog();
    const submit = screen.getByRole('button', { name: 'Queue reboot' });

    expect(submit).toBeDisabled();
    await userEvent.type(screen.getByLabelText('Typed node name'), 'edge one');
    await userEvent.type(screen.getByLabelText('Operator reason'), 'bad');
    await userEvent.click(screen.getByLabelText(/I understand that this queues a reboot job only/));
    expect(screen.getByText('Confirmation must match the selected node name exactly.')).toBeInTheDocument();
    expect(screen.getByText('Reason must be at least 5 characters.')).toBeInTheDocument();
    expect(submit).toBeDisabled();

    await userEvent.clear(screen.getByLabelText('Typed node name'));
    await userEvent.type(screen.getByLabelText('Typed node name'), ' Edge One ');
    await userEvent.clear(screen.getByLabelText('Operator reason'));
    await userEvent.type(screen.getByLabelText('Operator reason'), ' maintenance window ');
    expect(submit).toBeEnabled();
    await userEvent.click(submit);

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(onConfirm).toHaveBeenCalledWith({ confirmation: 'Edge One', reason: 'maintenance window' });
  });

  it('rejects unsafe reasons and keeps acknowledgement UI-only', async () => {
    const { onConfirm } = renderDialog();
    await userEvent.type(screen.getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(screen.getByLabelText('Operator reason'), 'Authorization: Bearer token');
    await userEvent.click(screen.getByLabelText(/I understand that this queues a reboot job only/));

    expect(screen.getByRole('button', { name: 'Queue reboot' })).toBeDisabled();
    expect(screen.getByText('Reason must not contain tokens, credentials, headers, secret references, command output or request-body-like content.')).toBeInTheDocument();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('prevents duplicate submit while the async request is active', async () => {
    let resolveRequest: (() => void) | undefined;
    const onConfirm = vi.fn(() => new Promise<void>((resolve) => {
      resolveRequest = resolve;
    }));
    renderDialog({ onConfirm });

    await userEvent.type(screen.getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(screen.getByLabelText('Operator reason'), 'maintenance window');
    await userEvent.click(screen.getByLabelText(/I understand that this queues a reboot job only/));
    await userEvent.dblClick(screen.getByRole('button', { name: 'Queue reboot' }));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Queueing reboot' })).toBeDisabled();
    resolveRequest?.();
  });

  it('uses safe backend error text and clears confirmation on backend mismatch', () => {
    renderDialog({
      error: new APIError('raw SQL token_hash command output Authorization: Bearer', 409, {
        code: 'node_reboot_confirmation_mismatch',
        error: 'raw SQL token_hash command output Authorization: Bearer',
      }),
    });

    const dialog = screen.getByRole('dialog', { name: 'Reboot node' });
    expect(within(dialog).getByText('Backend rejected the confirmation. Type the current node name again.')).toBeInTheDocument();
    expect(dialog).not.toHaveTextContent('raw SQL');
    expect(dialog).not.toHaveTextContent('token_hash');
    expect(dialog).not.toHaveTextContent('Authorization: Bearer');
  });

  it('clears transient form state when closed and does not write browser storage', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const { rerender } = renderDialog();
    await userEvent.type(screen.getByLabelText('Typed node name'), 'Edge One');
    await userEvent.type(screen.getByLabelText('Operator reason'), 'maintenance window');
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    rerender(
      <NodeRebootDialog
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
      <NodeRebootDialog
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
    expect(storageSet).not.toHaveBeenCalledWith(expect.stringMatching(/reboot|reason|confirmation|job|token|agent/i), expect.anything());
    expect(window.localStorage.getItem('node_reboot')).toBeNull();
    expect(window.sessionStorage.getItem('node_reboot')).toBeNull();
  });
});
