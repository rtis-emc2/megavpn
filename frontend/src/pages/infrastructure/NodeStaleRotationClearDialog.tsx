import { Eraser, RefreshCw, ShieldAlert } from 'lucide-react';
import type { FormEvent } from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { NodeDetail, NodeStaleRotationCandidate, NodeStaleRotationClearInput, NodeStaleRotationPreview } from '../../shared/api/types';
import { Badge, Button, Checkbox, ConfirmDialog, DataTable, FormField, FormGrid, StatusBadge, Textarea, TextField, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { describeStaleRotationReason, formatAgeSeconds } from './nodeLifecycleControls';
import {
  deriveNodeStaleRotationClearContext,
  nodeStaleRotationClearErrorCode,
  nodeStaleRotationClearErrorKey,
  reasonLooksUnsafeForNodeStaleRotationClear,
  validateNodeStaleRotationClearForm,
} from './nodeStaleRotationClear';

type NodeStaleRotationClearDialogProps = {
  open: boolean;
  node: NodeDetail;
  preview?: NodeStaleRotationPreview;
  capturedFingerprint: string;
  previewFetching: boolean;
  previewError?: unknown;
  pending: boolean;
  mutationError?: unknown;
  canBootstrapNode: boolean;
  lifecycleDataCurrent: boolean;
  onRefreshPreview: () => Promise<unknown> | void;
  onCancel: () => void;
  onConfirm: (input: NodeStaleRotationClearInput, fingerprint: string) => Promise<void> | void;
};

function CandidateReason({ candidate }: { candidate: NodeStaleRotationCandidate }) {
  const { t } = useTranslation();
  const descriptor = describeStaleRotationReason(candidate.stale_reason);
  return (
    <div className="page-stack">
      <span>{t(descriptor.labelKey)}</span>
      {!descriptor.known ? <Badge>{t('nodes.lifecycleControls.unknownReasonBadge')}</Badge> : null}
    </div>
  );
}

export function NodeStaleRotationClearDialog({
  open,
  node,
  preview,
  capturedFingerprint,
  previewFetching,
  previewError,
  pending,
  mutationError,
  canBootstrapNode,
  lifecycleDataCurrent,
  onRefreshPreview,
  onCancel,
  onConfirm,
}: NodeStaleRotationClearDialogProps) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const [confirmation, setConfirmation] = useState('');
  const [reason, setReason] = useState('');
  const [acknowledged, setAcknowledged] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [localSubmitting, setLocalSubmitting] = useState(false);
  const [previewChanged, setPreviewChanged] = useState(false);
  const wasOpenRef = useRef(false);
  const submittingRef = useRef(false);
  const lastPreviewSignatureRef = useRef(capturedFingerprint);
  const contextResult = useMemo(() => deriveNodeStaleRotationClearContext(node.id, preview), [node.id, preview]);
  const contextSignature = contextResult.valid
    ? contextResult.context.fingerprint
    : `invalid:${contextResult.errorKey}:${preview?.evaluated_at || ''}`;
  const validation = useMemo(
    () => validateNodeStaleRotationClearForm(node, contextResult, { confirmation, reason, acknowledged }),
    [acknowledged, confirmation, contextResult, node, reason],
  );
  const previewCandidates = preview?.node_id === node.id ? preview.candidates : [];
  const safeCandidates = previewCandidates.filter((candidate) => candidate.safe_to_clear === true);
  const excludedCandidates = previewCandidates.filter((candidate) => candidate.safe_to_clear !== true);
  const safeErrorKey = mutationError ? nodeStaleRotationClearErrorKey(mutationError) : '';
  const safeErrorCode = nodeStaleRotationClearErrorCode(mutationError);
  const mutationBlocksSubmit = [
    'node_stale_rotation_not_found',
    'node_stale_rotation_preview_changed',
    'node_stale_rotation_evidence_ambiguous',
    'node_stale_rotation_pending_state_ambiguous',
    'node_stale_rotation_conflict',
  ].includes(safeErrorCode);
  const showErrors = submitted || Boolean(confirmation) || Boolean(reason) || acknowledged;
  const disabled = pending
    || localSubmitting
    || previewFetching
    || Boolean(previewError)
    || !canBootstrapNode
    || !lifecycleDataCurrent
    || mutationBlocksSubmit
    || !contextResult.valid
    || !validation.valid;

  const resetForm = useCallback(() => {
    setConfirmation('');
    setReason('');
    setAcknowledged(false);
    setSubmitted(false);
    setLocalSubmitting(false);
    submittingRef.current = false;
    setPreviewChanged(false);
    lastPreviewSignatureRef.current = capturedFingerprint;
  }, [capturedFingerprint]);

  useEffect(() => {
    const wasOpen = wasOpenRef.current;
    wasOpenRef.current = open;
    if (!open || !wasOpen) {
      const timeout = window.setTimeout(resetForm, 0);
      return () => window.clearTimeout(timeout);
    }
    return undefined;
  }, [open, resetForm]);

  useEffect(() => {
    const timeout = window.setTimeout(resetForm, 0);
    return () => window.clearTimeout(timeout);
  }, [node.id, resetForm]);

  useEffect(() => {
    if (!open || !lastPreviewSignatureRef.current || contextSignature === lastPreviewSignatureRef.current) return;
    lastPreviewSignatureRef.current = contextSignature;
    const timeout = window.setTimeout(() => {
      setConfirmation('');
      setAcknowledged(false);
      setReason((current) => reasonLooksUnsafeForNodeStaleRotationClear(current) ? '' : current);
      setSubmitted(false);
      setPreviewChanged(true);
    }, 0);
    return () => window.clearTimeout(timeout);
  }, [contextSignature, open]);

  useEffect(() => {
    const code = safeErrorCode;
    if (!code) return;
    if (code === 'node_stale_rotation_confirmation_mismatch') {
      const timeout = window.setTimeout(() => setConfirmation(''), 0);
      return () => window.clearTimeout(timeout);
    }
    if ([
      'node_stale_rotation_not_found',
      'node_stale_rotation_preview_changed',
      'node_stale_rotation_evidence_ambiguous',
      'node_stale_rotation_pending_state_ambiguous',
    ].includes(code)) {
      const timeout = window.setTimeout(() => {
        setConfirmation('');
        setAcknowledged(false);
        setReason((current) => reasonLooksUnsafeForNodeStaleRotationClear(current) ? '' : current);
        setPreviewChanged(true);
      }, 0);
      return () => window.clearTimeout(timeout);
    }
    return undefined;
  }, [mutationError, safeErrorCode]);

  const submit = () => {
    setSubmitted(true);
    if (submittingRef.current || disabled || !validation.input || !contextResult.valid) return;
    submittingRef.current = true;
    setLocalSubmitting(true);
    void Promise.resolve(onConfirm(validation.input, contextResult.context.fingerprint))
      .catch(() => undefined)
      .finally(() => {
        submittingRef.current = false;
        setLocalSubmitting(false);
      });
  };

  const handleCancel = () => {
    resetForm();
    onCancel();
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    submit();
  };

  const columns = [
    { key: 'job', header: t('nodes.lifecycleControls.staleRotationClear.jobId'), render: (row: NodeStaleRotationCandidate) => <code>{shortID(row.job_id)}</code> },
    { key: 'status', header: t('common.status'), render: (row: NodeStaleRotationCandidate) => <StatusBadge status={row.status} /> },
    { key: 'reason', header: t('nodes.lifecycleControls.reason'), render: (row: NodeStaleRotationCandidate) => <CandidateReason candidate={row} /> },
    { key: 'safe', header: t('nodes.lifecycleControls.safeToClear'), render: (row: NodeStaleRotationCandidate) => row.safe_to_clear ? t('common.yes') : t('common.no') },
    { key: 'age', header: t('nodes.lifecycleControls.age'), render: (row: NodeStaleRotationCandidate) => formatAgeSeconds(row.age_seconds) },
    { key: 'created', header: t('common.created'), render: (row: NodeStaleRotationCandidate) => fmt.date(row.created_at) },
    { key: 'started', header: t('nodes.lifecycleControls.startedAt'), render: (row: NodeStaleRotationCandidate) => fmt.date(row.started_at) },
  ];

  return (
    <ConfirmDialog open={open} onClose={handleCancel} title={t('nodes.lifecycleControls.staleRotationClear.dialogTitle')}>
      <form className="page-stack" onSubmit={handleSubmit}>
        <div role="alert" className="error-state-inline">
          <ShieldAlert size={18} aria-hidden="true" />
          <span>{t('nodes.lifecycleControls.staleRotationClear.remediationWarning')}</span>
        </div>

        <div className="definition-grid">
          <span>{t('nodes.lifecycleControls.staleRotationClear.nodeLabel')}</span><strong>{text(node.name || node.id)}</strong>
          <span>{t('nodes.lifecycleControls.staleRotationClear.previewEvaluatedAt')}</span><strong>{fmt.date(preview?.evaluated_at)}</strong>
          <span>{t('nodes.lifecycleControls.staleRotationClear.expectedJobCount')}</span><strong>{contextResult.valid ? contextResult.context.expectedJobIds.length : 0}</strong>
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.staleRotationClear.doesTitle')}</strong>
          <ul>
            {(['cancelJobs', 'releaseLocks', 'clearPendingState', 'retainHistory', 'createAudit', 'preserveIdentity', 'preserveToken'] as const)
              .map((key) => <li key={key}>{t(`nodes.lifecycleControls.staleRotationClear.does.${key}`)}</li>)}
          </ul>
        </div>
        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.staleRotationClear.doesNotTitle')}</strong>
          <ul>
            {(['rotateToken', 'issueToken', 'issueEnrollmentToken', 'revokeIdentity', 'restoreConnectivity', 'fixAuthentication', 'reenrollNode', 'rebootOrCleanup', 'deleteNode'] as const)
              .map((key) => <li key={key}>{t(`nodes.lifecycleControls.staleRotationClear.doesNot.${key}`)}</li>)}
          </ul>
        </div>

        {previewChanged ? <div role="alert" className="error-state-inline">{t('nodes.lifecycleControls.staleRotationClear.previewChangedLocally')}</div> : null}
        {previewFetching ? <div role="status" aria-live="polite">{t('nodes.lifecycleControls.staleRotationClear.refreshingPreview')}</div> : null}
        {previewError ? <div role="alert" className="error-state-inline">{t('nodes.lifecycleControls.staleRotationClear.errors.previewUnavailable')}</div> : null}
        {!contextResult.valid ? <div role="alert" className="error-state-inline">{t(contextResult.errorKey)}</div> : null}

        <DataTable<NodeStaleRotationCandidate>
          rows={safeCandidates}
          title={t('nodes.lifecycleControls.staleRotationClear.safeCandidates')}
          columns={columns}
        />
        {excludedCandidates.length ? (
          <div className="page-stack">
            <div role="status" className="inline-panel">{t('nodes.lifecycleControls.staleRotationClear.excludedCandidatesNotice', { count: excludedCandidates.length })}</div>
            <DataTable<NodeStaleRotationCandidate>
              rows={excludedCandidates}
              title={t('nodes.lifecycleControls.staleRotationClear.excludedCandidates')}
              columns={columns}
            />
          </div>
        ) : null}

        <FormGrid>
          <FormField label={t('nodes.lifecycleControls.staleRotationClear.confirmationLabel')}>
            <TextField
              aria-label={t('nodes.lifecycleControls.staleRotationClear.confirmationLabel')}
              aria-invalid={Boolean(showErrors && validation.errors.confirmation)}
              value={confirmation}
              maxLength={513}
              placeholder={validation.expectedConfirmation}
              autoComplete="off"
              disabled={pending || localSubmitting}
              onChange={(event) => setConfirmation(event.target.value)}
            />
            <span className="muted">{t('nodes.lifecycleControls.staleRotationClear.confirmationHelp', { value: validation.expectedConfirmation })}</span>
            {showErrors && validation.errors.confirmation ? <span role="alert">{t(validation.errors.confirmation)}</span> : null}
          </FormField>
          <FormField label={t('nodes.lifecycleControls.staleRotationClear.reasonLabel')}>
            <Textarea
              aria-label={t('nodes.lifecycleControls.staleRotationClear.reasonLabel')}
              aria-invalid={Boolean(showErrors && validation.errors.reason)}
              value={reason}
              maxLength={501}
              rows={4}
              autoComplete="off"
              disabled={pending || localSubmitting}
              onChange={(event) => setReason(event.target.value)}
            />
            <span className="muted">{t('nodes.lifecycleControls.staleRotationClear.reasonHelp')}</span>
            {showErrors && validation.errors.reason ? <span role="alert">{t(validation.errors.reason)}</span> : null}
          </FormField>
        </FormGrid>

        <Checkbox
          checked={acknowledged}
          disabled={pending || localSubmitting}
          onChange={(event) => setAcknowledged(event.target.checked)}
          label={t('nodes.lifecycleControls.staleRotationClear.acknowledgementLabel')}
        />
        {showErrors && validation.errors.acknowledgement ? <span role="alert">{t(validation.errors.acknowledgement)}</span> : null}
        {!canBootstrapNode ? <div role="alert" className="error-state-inline">{t('nodes.lifecycleControls.staleRotationClear.errors.permissionRequired')}</div> : null}
        {!lifecycleDataCurrent ? <div role="alert" className="error-state-inline">{t('nodes.lifecycleControls.staleRotationClear.errors.lifecycleDataStale')}</div> : null}
        {safeErrorKey ? <div role="alert" className="error-state-inline">{t(safeErrorKey)}</div> : null}
        {(pending || localSubmitting) ? <div role="status" aria-live="polite">{t('nodes.lifecycleControls.staleRotationClear.pending')}</div> : null}

        <Toolbar>
          <Button
            type="button"
            icon={<RefreshCw size={16} />}
            disabled={pending || localSubmitting || previewFetching}
            onClick={() => void onRefreshPreview()}
          >
            {previewFetching ? t('nodes.lifecycleControls.staleRotationClear.refreshingPreview') : t('nodes.lifecycleControls.staleRotationClear.refreshPreview')}
          </Button>
          <Button type="submit" variant="danger" icon={<Eraser size={16} />} disabled={disabled}>
            {(pending || localSubmitting) ? t('nodes.lifecycleControls.staleRotationClear.pending') : t('nodes.lifecycleControls.staleRotationClear.submit')}
          </Button>
          <Button type="button" disabled={pending || localSubmitting} onClick={handleCancel}>{t('common.cancel')}</Button>
        </Toolbar>
      </form>
    </ConfirmDialog>
  );
}
