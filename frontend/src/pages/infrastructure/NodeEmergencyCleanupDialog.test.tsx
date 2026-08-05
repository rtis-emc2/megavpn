import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { APIError } from '../../shared/api/client';
import type { NodeDetail, NodeDiagnostics } from '../../shared/api/types';
import i18n from '../../shared/i18n';
import { NodeEmergencyCleanupDialog } from './NodeEmergencyCleanupDialog';

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
  communication_hint: 'raw-command-output-not-rendered',
  agent: {
    node_id: 'node-1',
    status: 'active',
    token_hint: 'token-hint-not-rendered',
    secret_ref_id: 'secret-ref-not-rendered',
  },
  recent_discoveries: [{
    id: 'discovery-1',
    systemd_unit: 'unit-not-rendered.service',
    config_path: '/path/not-rendered',
  }],
};

function renderDialog(overrides: Partial<Parameters<typeof NodeEmergencyCleanupDialog>[0]> = {}) {
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
  const result = render(<NodeEmergencyCleanupDialog {...props} />);
  return { ...result, onCancel, onConfirm };
}

async function completeBaseForm(scope: 'services_only' | 'full_node' = 'services_only') {
  await userEvent.selectOptions(screen.getByLabelText('Cleanup scope'), scope);
  await userEvent.type(screen.getByLabelText('Typed node name'), ' Edge One ');
  await userEvent.type(screen.getByLabelText('Operator reason'), ' maintenance window ');
  await userEvent.click(screen.getByLabelText(/I understand that this queues destructive managed-resource cleanup/));
}

describe('NodeEmergencyCleanupDialog', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
    window.localStorage.clear();
    window.sessionStorage.clear();
  });

  it('renders an accessible staged workflow with accurate queue-only and scope semantics', async () => {
    renderDialog();
    const dialog = screen.getByRole('dialog', { name: 'Configure Emergency Cleanup' });

    expect(dialog).toHaveTextContent('Emergency Cleanup is a destructive, disruptive remediation operation.');
    expect(dialog).toHaveTextContent('Currently managed VPN and proxy services may become unavailable.');
    expect(dialog).toHaveTextContent('Active client traffic may be interrupted.');
    expect(dialog).toHaveTextContent('Active administrative sessions may be interrupted.');
    expect(dialog).toHaveTextContent('The operator must review the existing Jobs tab after queueing.');
    expect(dialog).toHaveTextContent('Delete the control-plane database node record.');
    expect(dialog).toHaveTextContent('Erase inventory or job history.');
    expect(dialog).toHaveTextContent('Automatically revoke the control-plane agent identity.');
    expect(dialog).toHaveTextContent('Automatically reboot the node.');
    expect(dialog).toHaveTextContent('This confirms queueing only. It does not confirm that cleanup or agent removal completed.');
    expect(within(dialog).getByRole('button', { name: 'Queue Emergency Cleanup' })).toBeDisabled();
    expect(within(dialog).queryByLabelText(/Request MegaVPN agent self-removal/)).not.toBeInTheDocument();

    await userEvent.selectOptions(within(dialog).getByLabelText('Cleanup scope'), 'services_only');
    expect(dialog).toHaveTextContent('The agent is retained, full-node managed-runtime cleanup is not requested');
    expect(dialog).toHaveTextContent('The backend builds the final executable plan transactionally');

    await userEvent.selectOptions(within(dialog).getByLabelText('Cleanup scope'), 'full_node');
    expect(dialog).toHaveTextContent('managed route policy');
    expect(dialog).toHaveTextContent('managed Nginx snippets');
    expect(dialog).toHaveTextContent('The shared Nginx service is not intentionally stopped or disabled');
    expect(within(dialog).getByLabelText(/Request MegaVPN agent self-removal/)).toBeInTheDocument();

    const body = document.body.textContent || '';
    for (const forbidden of ['token-hint-not-rendered', 'secret-ref-not-rendered', 'raw-command-output-not-rendered', 'unit-not-rendered.service', '/path/not-rendered']) {
      expect(body).not.toContain(forbidden);
    }
  });

  it('submits the exact normalized services-only input with both acknowledgement fields', async () => {
    const { onConfirm } = renderDialog();
    await completeBaseForm();
    const submit = screen.getByRole('button', { name: 'Queue Emergency Cleanup' });
    expect(submit).toBeEnabled();
    await userEvent.click(submit);

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(onConfirm).toHaveBeenCalledWith({
      cleanup_scope: 'services_only',
      include_agent: false,
      confirmation: 'Edge One',
      reason: 'maintenance window',
      acknowledge_destructive_cleanup: true,
      acknowledge_agent_removal: false,
    });
  });

  it('requires an additional acknowledgement for full-node agent removal and resets it on scope change', async () => {
    const first = renderDialog();
    const { onConfirm } = first;
    await completeBaseForm('full_node');
    await userEvent.click(screen.getByLabelText(/Request MegaVPN agent self-removal/));

    expect(screen.getByText(/Agent self-removal is requested only after successful managed cleanup/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Queue cleanup and request agent removal' })).toBeDisabled();
    await userEvent.click(screen.getByLabelText(/I explicitly request agent self-removal/));
    const submit = screen.getByRole('button', { name: 'Queue cleanup and request agent removal' });
    expect(submit).toBeEnabled();
    await userEvent.click(submit);
    await waitFor(() => expect(onConfirm).toHaveBeenCalledWith(expect.objectContaining({
      cleanup_scope: 'full_node',
      include_agent: true,
      acknowledge_agent_removal: true,
    })));

    first.unmount();
    renderDialog();
    await userEvent.selectOptions(screen.getByLabelText('Cleanup scope'), 'full_node');
    await userEvent.click(screen.getByLabelText(/Request MegaVPN agent self-removal/));
    await userEvent.click(screen.getByLabelText(/I explicitly request agent self-removal/));
    await userEvent.selectOptions(screen.getByLabelText('Cleanup scope'), 'services_only');
    expect(screen.queryByLabelText(/Request MegaVPN agent self-removal/)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/I explicitly request agent self-removal/)).not.toBeInTheDocument();
  });

  it('rejects wrong confirmation, short and secret-like reasons before submission', async () => {
    const { onConfirm } = renderDialog();
    await userEvent.selectOptions(screen.getByLabelText('Cleanup scope'), 'services_only');
    await userEvent.type(screen.getByLabelText('Typed node name'), 'edge one');
    await userEvent.type(screen.getByLabelText('Operator reason'), 'bad');
    await userEvent.click(screen.getByLabelText(/I understand that this queues destructive managed-resource cleanup/));
    expect(screen.getByText('Confirmation must match the selected node name exactly.')).toBeInTheDocument();
    expect(screen.getByText('Reason must be at least 5 characters.')).toBeInTheDocument();

    await userEvent.clear(screen.getByLabelText('Operator reason'));
    await userEvent.type(screen.getByLabelText('Operator reason'), 'Authorization: Bearer value');
    expect(screen.getByText(/Reason must not contain tokens, credentials, headers/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Queue Emergency Cleanup' })).toBeDisabled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('prevents duplicate submission while the asynchronous request is active', async () => {
    let resolveRequest: (() => void) | undefined;
    const onConfirm = vi.fn(() => new Promise<void>((resolve) => {
      resolveRequest = resolve;
    }));
    renderDialog({ onConfirm });
    await completeBaseForm();
    await userEvent.dblClick(screen.getByRole('button', { name: 'Queue Emergency Cleanup' }));

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Queueing Emergency Cleanup' })).toBeDisabled();
    resolveRequest?.();
  });

  it('maps backend errors safely and clears confirmation after backend mismatch', async () => {
    renderDialog({
      error: new APIError('raw SQL token_hash target /etc/private', 409, {
        code: 'node_emergency_cleanup_confirmation_mismatch',
        error: 'raw SQL token_hash target /etc/private',
      }),
    });
    expect(screen.getByText('Backend rejected the confirmation. Type the current node name again.')).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent('raw SQL');
    expect(document.body).not.toHaveTextContent('token_hash');
    expect(document.body).not.toHaveTextContent('/etc/private');
    await waitFor(() => expect(screen.getByLabelText('Typed node name')).toHaveValue(''));
  });

  it('clears all transient form state on close and never writes browser storage', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const { rerender } = renderDialog();
    await completeBaseForm('full_node');
    await userEvent.click(screen.getByLabelText(/Request MegaVPN agent self-removal/));
    await userEvent.click(screen.getByLabelText(/I explicitly request agent self-removal/));
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    rerender(<NodeEmergencyCleanupDialog open={false} node={node} diagnostics={diagnostics} pending={false} canBootstrapNode onCancel={vi.fn()} onConfirm={vi.fn()} />);
    rerender(<NodeEmergencyCleanupDialog open node={node} diagnostics={diagnostics} pending={false} canBootstrapNode onCancel={vi.fn()} onConfirm={vi.fn()} />);

    await waitFor(() => expect(screen.getByLabelText('Cleanup scope')).toHaveValue(''));
    expect(screen.getByLabelText('Typed node name')).toHaveValue('');
    expect(screen.getByLabelText('Operator reason')).toHaveValue('');
    expect(storageSet.mock.calls.some(([key, value]) => /cleanup|reason|confirmation|agent|job/i.test(String(key)) || /maintenance window|Edge One/.test(String(value)))).toBe(false);
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
  });
});
