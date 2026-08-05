import { ShieldAlert } from 'lucide-react';
import type { FormEvent } from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { NodeAgentIdentityRevokeInput, NodeDetail, NodeDiagnostics } from '../../shared/api/types';
import { Button, Checkbox, ConfirmDialog, FormField, FormGrid, StatusBadge, Textarea, TextField, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import {
  nodeAgentIdentityRevokeErrorKey,
  normalizeLifecycleStatus,
  validateNodeAgentIdentityRevokeForm,
} from './nodeLifecycleControls';

type NodeAgentIdentityRevokeDialogProps = {
  open: boolean;
  node: NodeDetail;
  diagnostics?: NodeDiagnostics;
  pending: boolean;
  error?: unknown;
  canBootstrapNode: boolean;
  onCancel: () => void;
  onConfirm: (input: NodeAgentIdentityRevokeInput) => Promise<void> | void;
};

const consequenceKeys = [
  'invalidateIdentity',
  'rejectCurrentToken',
  'revokeEnrollmentTokens',
  'requiresNewEnrollmentToken',
  'preserveNodeRecord',
  'preserveInventoryAudit',
  'preserveQueuedJobs',
] as const;

const nonConsequenceKeys = [
  'uninstallAgent',
  'stopAgent',
  'deleteNode',
  'deleteInstances',
  'cleanHost',
  'rebootNode',
  'createReplacementToken',
  'reenrollNode',
] as const;

export function NodeAgentIdentityRevokeDialog({
  open,
  node,
  diagnostics,
  pending,
  error,
  canBootstrapNode,
  onCancel,
  onConfirm,
}: NodeAgentIdentityRevokeDialogProps) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const [confirmation, setConfirmation] = useState('');
  const [reason, setReason] = useState('');
  const [acknowledged, setAcknowledged] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [localSubmitting, setLocalSubmitting] = useState(false);
  const wasOpenRef = useRef(false);
  const agentStatus = diagnostics?.agent?.status || node.agent_status || 'unknown';
  const tokenRotationStatus = diagnostics?.agent?.token_rotation_status || 'unknown';
  const validation = useMemo(
    () => validateNodeAgentIdentityRevokeForm(node, { confirmation, reason, acknowledged }),
    [acknowledged, confirmation, node, reason],
  );
  const showErrors = submitted || Boolean(confirmation) || Boolean(reason) || acknowledged;
  const disabled = pending || localSubmitting || !canBootstrapNode || !validation.valid;
  const safeErrorKey = error ? nodeAgentIdentityRevokeErrorKey(error) : '';

  const resetForm = useCallback(() => {
    setConfirmation('');
    setReason('');
    setAcknowledged(false);
    setSubmitted(false);
    setLocalSubmitting(false);
  }, []);

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
    if (safeErrorKey === 'nodes.lifecycleControls.agentIdentityRevoke.errors.confirmationMismatchBackend') {
      const timeout = window.setTimeout(() => setConfirmation(''), 0);
      return () => window.clearTimeout(timeout);
    }
    return undefined;
  }, [safeErrorKey]);

  const submit = () => {
    setSubmitted(true);
    if (disabled || !validation.input) return;
    setLocalSubmitting(true);
    void Promise.resolve(onConfirm(validation.input))
      .catch(() => undefined)
      .finally(() => setLocalSubmitting(false));
  };

  const handleCancel = () => {
    resetForm();
    onCancel();
  };

  const preventEnterBypass = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    submit();
  };

  return (
    <ConfirmDialog open={open} onClose={handleCancel} title={t('nodes.lifecycleControls.agentIdentityRevoke.dialogTitle')}>
      <form className="page-stack" onSubmit={preventEnterBypass}>
        <div role="alert" className="error-state-inline">
          <ShieldAlert size={16} aria-hidden="true" />
          <span>{t('nodes.lifecycleControls.agentIdentityRevoke.destructiveWarning')}</span>
        </div>

        <div className="definition-grid">
          <span>{t('common.name')}</span><strong>{text(node.name || node.id)}</strong>
          <span>{t('common.id')}</span><strong>{shortID(node.id)}</strong>
          <span>{t('nodes.lifecycleControls.agentIdentityRevoke.identityStatus')}</span><strong><StatusBadge status={normalizeLifecycleStatus(agentStatus)} /></strong>
          <span>{t('nodes.lifecycleControls.tokenRotationStatus')}</span><strong><StatusBadge status={normalizeLifecycleStatus(tokenRotationStatus)} /></strong>
          <span>{t('nodes.lifecycleControls.agentIdentityRevoke.lastSeen')}</span><strong>{fmt.date(diagnostics?.agent?.last_seen_at || node.agent_last_seen_at || node.last_heartbeat_at)}</strong>
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.agentIdentityRevoke.doesTitle')}</strong>
          <ul>
            {consequenceKeys.map((key) => <li key={key}>{t(`nodes.lifecycleControls.agentIdentityRevoke.does.${key}`)}</li>)}
          </ul>
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.agentIdentityRevoke.doesNotTitle')}</strong>
          <ul>
            {nonConsequenceKeys.map((key) => <li key={key}>{t(`nodes.lifecycleControls.agentIdentityRevoke.doesNot.${key}`)}</li>)}
          </ul>
        </div>

        <FormGrid>
          <FormField label={t('nodes.lifecycleControls.agentIdentityRevoke.confirmationLabel')}>
            <TextField
              aria-label={t('nodes.lifecycleControls.agentIdentityRevoke.confirmationLabel')}
              aria-invalid={Boolean(showErrors && validation.errors.confirmation)}
              value={confirmation}
              maxLength={513}
              placeholder={validation.expectedConfirmation}
              autoComplete="off"
              onChange={(event) => setConfirmation(event.target.value)}
            />
            <span className="muted">{t('nodes.lifecycleControls.agentIdentityRevoke.confirmationHelp', { value: validation.expectedConfirmation })}</span>
            {showErrors && validation.errors.confirmation ? <span role="alert">{t(validation.errors.confirmation)}</span> : null}
          </FormField>
          <FormField label={t('nodes.lifecycleControls.agentIdentityRevoke.reasonLabel')}>
            <Textarea
              aria-label={t('nodes.lifecycleControls.agentIdentityRevoke.reasonLabel')}
              aria-invalid={Boolean(showErrors && validation.errors.reason)}
              value={reason}
              maxLength={501}
              rows={4}
              autoComplete="off"
              onChange={(event) => setReason(event.target.value)}
            />
            <span className="muted">{t('nodes.lifecycleControls.agentIdentityRevoke.reasonHelp')}</span>
            {showErrors && validation.errors.reason ? <span role="alert">{t(validation.errors.reason)}</span> : null}
          </FormField>
        </FormGrid>

        <Checkbox
          checked={acknowledged}
          onChange={(event) => setAcknowledged(event.target.checked)}
          label={t('nodes.lifecycleControls.agentIdentityRevoke.acknowledgementLabel')}
        />
        {showErrors && validation.errors.acknowledgement ? <span role="alert">{t(validation.errors.acknowledgement)}</span> : null}
        {!canBootstrapNode ? <div role="alert" className="error-state-inline">{t('common.permissionRequired', { permission: 'node.bootstrap' })}</div> : null}
        {safeErrorKey ? <div role="alert" className="error-state-inline">{t(safeErrorKey)}</div> : null}
        {(pending || localSubmitting) ? <div role="status" aria-live="polite">{t('nodes.lifecycleControls.agentIdentityRevoke.pending')}</div> : null}
        <p className="muted">{t('nodes.lifecycleControls.agentIdentityRevoke.reenrollmentGuidance')}</p>

        <Toolbar>
          <Button type="submit" variant="danger" disabled={disabled} icon={<ShieldAlert size={16} />}>
            {(pending || localSubmitting) ? t('nodes.lifecycleControls.agentIdentityRevoke.pending') : t('nodes.lifecycleControls.agentIdentityRevoke.submit')}
          </Button>
          <Button type="button" onClick={handleCancel}>{t('common.cancel')}</Button>
        </Toolbar>
      </form>
    </ConfirmDialog>
  );
}
