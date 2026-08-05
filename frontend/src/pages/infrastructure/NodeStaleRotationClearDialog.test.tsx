import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { APIError } from '../../shared/api/client';
import type { NodeDetail, NodeStaleRotationPreview } from '../../shared/api/types';
import i18n from '../../shared/i18n';
import { deriveNodeStaleRotationClearContext } from './nodeStaleRotationClear';
import { NodeStaleRotationClearDialog } from './NodeStaleRotationClearDialog';

const node: NodeDetail = { id: 'node-1', name: 'Edge One', status: 'offline' };
const preview: NodeStaleRotationPreview = {
  node_id: 'node-1',
  stale_rotation_detected: true,
  token_rotation_status: 'rotating',
  evaluated_at: '2026-07-16T08:00:00Z',
  candidates: [
    {
      job_id: 'job-safe-rotation-1',
      status: 'running',
      created_at: '2026-07-16T07:00:00Z',
      started_at: '2026-07-16T07:01:00Z',
      age_seconds: 3600,
      stale_reason: 'claimed_without_result_and_agent_inactive',
      safe_to_clear: true,
    },
    {
      job_id: 'job-unsafe-rotation-2',
      status: 'running',
      created_at: '2026-07-16T07:45:00Z',
      age_seconds: 900,
      stale_reason: 'evidence_ambiguous',
      safe_to_clear: false,
    },
  ],
};

function fingerprint(value = preview) {
  const result = deriveNodeStaleRotationClearContext(node.id, value);
  if (!result.valid) throw new Error('fixture must have a valid clear context');
  return result.context.fingerprint;
}

function renderDialog(overrides: Partial<Parameters<typeof NodeStaleRotationClearDialog>[0]> = {}) {
  const onRefreshPreview = vi.fn().mockResolvedValue(undefined);
  const onCancel = vi.fn();
  const onConfirm = vi.fn().mockResolvedValue(undefined);
  const props = {
    open: true,
    node,
    preview,
    capturedFingerprint: fingerprint(),
    previewFetching: false,
    previewError: undefined,
    pending: false,
    mutationError: undefined,
    canBootstrapNode: true,
    lifecycleDataCurrent: true,
    onRefreshPreview,
    onCancel,
    onConfirm,
    ...overrides,
  };
  const result = render(<NodeStaleRotationClearDialog {...props} />);
  return { ...result, onRefreshPreview, onCancel, onConfirm, props };
}

async function completeForm() {
  await userEvent.type(screen.getByLabelText('Exact node name confirmation'), ' Edge One ');
  await userEvent.type(screen.getByLabelText('Operator reason'), ' reviewed stale rotation ');
  await userEvent.click(screen.getByLabelText(/all listed stale rotation jobs will be cancelled/));
}

describe('NodeStaleRotationClearDialog', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
    window.localStorage.clear();
    window.sessionStorage.clear();
  });

  it('renders complete safe and excluded evidence with accurate semantic boundaries', () => {
    renderDialog();
    const dialog = screen.getByRole('dialog', { name: 'Clear stale token rotation' });
    expect(dialog).toHaveTextContent('request cancellation of every listed stale token-rotation job');
    expect(dialog).toHaveTextContent('preserve the active agent identity');
    expect(dialog).toHaveTextContent('preserve the current active agent token');
    expect(dialog).toHaveTextContent('rotate the agent token');
    expect(dialog).toHaveTextContent('issue a new agent token');
    expect(dialog).toHaveTextContent('restore agent connectivity');
    expect(dialog).toHaveTextContent('Safe candidates included in cleanup');
    expect(dialog).toHaveTextContent('Unsafe or ambiguous candidates not included');
    expect(dialog).toHaveTextContent('job-safe...');
    expect(dialog).toHaveTextContent('job-unsa...');
    expect(dialog).not.toHaveTextContent('token_hash');
    expect(dialog).not.toHaveTextContent('request_payload');
    expect(within(dialog).queryAllByRole('checkbox')).toHaveLength(1);
  });

  it('submits the exact complete immutable safe set and context fingerprint once', async () => {
    let resolveRequest: (() => void) | undefined;
    const onConfirm = vi.fn(() => new Promise<void>((resolve) => {
      resolveRequest = resolve;
    }));
    renderDialog({ onConfirm });
    await completeForm();
    const submit = screen.getByRole('button', { name: 'Clear stale rotation state' });
    expect(submit).toBeEnabled();
    await userEvent.dblClick(submit);

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    expect(onConfirm).toHaveBeenCalledWith({
      confirmation: 'Edge One',
      reason: 'reviewed stale rotation',
      acknowledge_cancel_rotation: true,
      expected_job_ids: ['job-safe-rotation-1'],
    }, fingerprint());
    expect(screen.getByRole('button', { name: 'Clearing stale rotation state' })).toBeDisabled();
    resolveRequest?.();
  });

  it('validates exact confirmation, secret-safe reason and explicit acknowledgement', async () => {
    const { onConfirm } = renderDialog();
    await userEvent.type(screen.getByLabelText('Exact node name confirmation'), 'edge one');
    await userEvent.type(screen.getByLabelText('Operator reason'), 'Authorization: Bearer token');
    expect(screen.getByText(/case-sensitive/)).toBeInTheDocument();
    expect(screen.getByText(/must not contain tokens, credentials/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Clear stale rotation state' })).toBeDisabled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('invalidates confirmation and acknowledgement when refreshed evidence changes while preserving a safe reason', async () => {
    const first = renderDialog();
    await completeForm();
    const refreshed: NodeStaleRotationPreview = {
      ...preview,
      evaluated_at: '2026-07-16T08:05:00Z',
      candidates: [{
        ...preview.candidates[0],
        job_id: 'job-safe-rotation-new',
        stale_reason: 'unclaimed_without_agent_progress',
      }],
    };
    first.rerender(<NodeStaleRotationClearDialog {...first.props} preview={refreshed} />);

    await waitFor(() => expect(screen.getByText('Stale rotation evidence changed. Review the refreshed candidates and confirm again.')).toBeInTheDocument());
    expect(screen.getByLabelText('Exact node name confirmation')).toHaveValue('');
    expect(screen.getByLabelText('Operator reason')).toHaveValue(' reviewed stale rotation ');
    expect(screen.getByLabelText(/all listed stale rotation jobs will be cancelled/)).not.toBeChecked();
    expect(screen.getByRole('button', { name: 'Clear stale rotation state' })).toBeDisabled();
    expect(first.onConfirm).not.toHaveBeenCalled();
  });

  it('maps backend conflict safely and never renders raw backend response text', async () => {
    renderDialog({
      mutationError: new APIError('Authorization: Bearer token_hash raw SQL', 409, {
        code: 'node_stale_rotation_evidence_ambiguous',
        error: 'Authorization: Bearer token_hash raw SQL',
      }),
    });
    expect(screen.getByText('Backend evidence is ambiguous and does not permit clearing. No force override is available.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Clear stale rotation state' })).toBeDisabled();
    expect(document.body).not.toHaveTextContent('raw SQL');
    expect(document.body).not.toHaveTextContent('token_hash');
    expect(document.body).not.toHaveTextContent('Authorization: Bearer');
  });

  it('blocks mutation without permission/current ownership and refreshes only through the supplied action', async () => {
    const noPermission = renderDialog({ canBootstrapNode: false, lifecycleDataCurrent: false });
    expect(screen.getByText(/node.bootstrap permission is required/)).toBeInTheDocument();
    expect(screen.getByText(/Lifecycle data does not belong to the selected node/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Clear stale rotation state' })).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'Refresh preview' }));
    expect(noPermission.onRefreshPreview).toHaveBeenCalledTimes(1);
    expect(noPermission.onConfirm).not.toHaveBeenCalled();
  });

  it('clears transient state on close and never writes browser storage', async () => {
    const storageSet = vi.spyOn(Storage.prototype, 'setItem');
    const result = renderDialog();
    await completeForm();
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(result.onCancel).toHaveBeenCalledTimes(1);
    result.rerender(<NodeStaleRotationClearDialog {...result.props} open={false} />);
    result.rerender(<NodeStaleRotationClearDialog {...result.props} open />);
    await waitFor(() => expect(screen.getByLabelText('Exact node name confirmation')).toHaveValue(''));
    expect(screen.getByLabelText('Operator reason')).toHaveValue('');
    expect(storageSet).not.toHaveBeenCalled();
    expect(window.localStorage.length).toBe(0);
    expect(window.sessionStorage.length).toBe(0);
  });
});
