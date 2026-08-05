import { RotateCcw } from 'lucide-react';
import type { FormEvent } from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { NodeDetail, NodeDiagnostics, NodeRebootInput } from '../../shared/api/types';
import { Button, Checkbox, ConfirmDialog, FormField, FormGrid, StatusBadge, Textarea, TextField, Toolbar } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import {
  nodeRebootErrorKey,
  normalizeLifecycleStatus,
  validateNodeRebootForm,
} from './nodeLifecycleControls';

type NodeRebootDialogProps = {
  open: boolean;
  node: NodeDetail;
  diagnostics?: NodeDiagnostics;
  pending: boolean;
  error?: unknown;
  canBootstrapNode: boolean;
  onCancel: () => void;
  onConfirm: (input: NodeRebootInput) => Promise<void> | void;
};

const consequenceKeys = [
  'queueJob',
  'deliverViaAgent',
  'interruptTraffic',
  'interruptAdminSessions',
  'temporaryUnavailable',
  'delayQueuedWork',
  'manualInvestigation',
  'retainHistory',
] as const;

const nonConsequenceKeys = [
  'proveExecution',
  'proveShutdown',
  'proveServiceRestoration',
  'proveHeartbeatRecovery',
  'proveRuntimeRecovery',
  'revokeIdentity',
  'removeAgent',
  'deleteNode',
  'deleteInstances',
  'changeMaintenanceMode',
] as const;

export function NodeRebootDialog({
  open,
  node,
  diagnostics,
  pending,
  error,
  canBootstrapNode,
  onCancel,
  onConfirm,
}: NodeRebootDialogProps) {
  const { t } = useTranslation();
  const [confirmation, setConfirmation] = useState('');
  const [reason, setReason] = useState('');
  const [acknowledged, setAcknowledged] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [localSubmitting, setLocalSubmitting] = useState(false);
  const wasOpenRef = useRef(false);
  const agentStatus = diagnostics?.agent?.status || node.agent_status || 'unknown';
  const communicationStatus = diagnostics?.communication_state || node.agent_channel_status || 'unknown';
  const validation = useMemo(
    () => validateNodeRebootForm(node, { confirmation, reason, acknowledged }),
    [acknowledged, confirmation, node, reason],
  );
  const showErrors = submitted || Boolean(confirmation) || Boolean(reason) || acknowledged;
  const disabled = pending || localSubmitting || !canBootstrapNode || !validation.valid;
  const safeErrorKey = error ? nodeRebootErrorKey(error) : '';

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
    if (safeErrorKey === 'nodes.lifecycleControls.nodeReboot.errors.confirmationMismatchBackend') {
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
    <ConfirmDialog open={open} onClose={handleCancel} title={t('nodes.lifecycleControls.nodeReboot.dialogTitle')}>
      <form className="page-stack" onSubmit={preventEnterBypass}>
        <div role="alert" className="error-state-inline">
          <RotateCcw size={16} aria-hidden="true" />
          <span>{t('nodes.lifecycleControls.nodeReboot.disruptionWarning')}</span>
        </div>

        <div className="definition-grid">
          <span>{t('nodes.lifecycleControls.nodeReboot.nodeLabel')}</span><strong>{text(node.name || node.id)}</strong>
          <span>{t('nodes.lifecycleControls.nodeReboot.agentStatus')}</span><strong><StatusBadge status={normalizeLifecycleStatus(agentStatus)} /></strong>
          <span>{t('nodes.lifecycleControls.nodeReboot.communicationStatus')}</span><strong><StatusBadge status={normalizeLifecycleStatus(communicationStatus)} /></strong>
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.nodeReboot.doesTitle')}</strong>
          <ul>
            {consequenceKeys.map((key) => <li key={key}>{t(`nodes.lifecycleControls.nodeReboot.does.${key}`)}</li>)}
          </ul>
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.nodeReboot.doesNotTitle')}</strong>
          <ul>
            {nonConsequenceKeys.map((key) => <li key={key}>{t(`nodes.lifecycleControls.nodeReboot.doesNot.${key}`)}</li>)}
          </ul>
        </div>

        <FormGrid>
          <FormField label={t('nodes.lifecycleControls.nodeReboot.confirmationLabel')}>
            <TextField
              aria-label={t('nodes.lifecycleControls.nodeReboot.confirmationLabel')}
              aria-invalid={Boolean(showErrors && validation.errors.confirmation)}
              value={confirmation}
              maxLength={513}
              placeholder={validation.expectedConfirmation}
              autoComplete="off"
              onChange={(event) => setConfirmation(event.target.value)}
            />
            <span className="muted">{t('nodes.lifecycleControls.nodeReboot.confirmationHelp', { value: validation.expectedConfirmation })}</span>
            {showErrors && validation.errors.confirmation ? <span role="alert">{t(validation.errors.confirmation)}</span> : null}
          </FormField>
          <FormField label={t('nodes.lifecycleControls.nodeReboot.reasonLabel')}>
            <Textarea
              aria-label={t('nodes.lifecycleControls.nodeReboot.reasonLabel')}
              aria-invalid={Boolean(showErrors && validation.errors.reason)}
              value={reason}
              maxLength={501}
              rows={4}
              autoComplete="off"
              onChange={(event) => setReason(event.target.value)}
            />
            <span className="muted">{t('nodes.lifecycleControls.nodeReboot.reasonHelp')}</span>
            {showErrors && validation.errors.reason ? <span role="alert">{t(validation.errors.reason)}</span> : null}
          </FormField>
        </FormGrid>

        <Checkbox
          checked={acknowledged}
          onChange={(event) => setAcknowledged(event.target.checked)}
          label={t('nodes.lifecycleControls.nodeReboot.acknowledgementLabel')}
        />
        {showErrors && validation.errors.acknowledgement ? <span role="alert">{t(validation.errors.acknowledgement)}</span> : null}
        {!canBootstrapNode ? <div role="alert" className="error-state-inline">{t('common.permissionRequired', { permission: 'node.bootstrap' })}</div> : null}
        {safeErrorKey ? <div role="alert" className="error-state-inline">{t(safeErrorKey)}</div> : null}
        {(pending || localSubmitting) ? <div role="status" aria-live="polite">{t('nodes.lifecycleControls.nodeReboot.pending')}</div> : null}
        <p className="muted">{t('nodes.lifecycleControls.nodeReboot.queueOnlyDisclaimer')}</p>

        <Toolbar>
          <Button type="submit" variant="danger" disabled={disabled} icon={<RotateCcw size={16} />}>
            {(pending || localSubmitting) ? t('nodes.lifecycleControls.nodeReboot.pending') : t('nodes.lifecycleControls.nodeReboot.submit')}
          </Button>
          <Button type="button" onClick={handleCancel}>{t('common.cancel')}</Button>
        </Toolbar>
      </form>
    </ConfirmDialog>
  );
}
