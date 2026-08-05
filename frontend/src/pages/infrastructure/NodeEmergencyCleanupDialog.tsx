import { ShieldAlert } from 'lucide-react';
import type { FormEvent } from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { NodeDetail, NodeDiagnostics, NodeEmergencyCleanupInput, NodeEmergencyCleanupScope } from '../../shared/api/types';
import { Button, Checkbox, ConfirmDialog, FormField, FormGrid, Select, StatusBadge, Textarea, TextField, Toolbar } from '../../shared/ui';
import { text } from '../../shared/utils/format';
import { normalizeLifecycleStatus } from './nodeLifecycleControls';
import {
  nodeEmergencyCleanupErrorKey,
  resetNodeEmergencyCleanupScopeRelationship,
  validateNodeEmergencyCleanupForm,
  type NodeEmergencyCleanupForm,
} from './nodeEmergencyCleanup';

type NodeEmergencyCleanupDialogProps = {
  open: boolean;
  node: NodeDetail;
  diagnostics?: NodeDiagnostics;
  pending: boolean;
  error?: unknown;
  canBootstrapNode: boolean;
  onCancel: () => void;
  onConfirm: (input: NodeEmergencyCleanupInput) => Promise<void> | void;
};

const initialForm: NodeEmergencyCleanupForm = {
  cleanupScope: '',
  includeAgent: false,
  confirmation: '',
  reason: '',
  acknowledgeDestructiveCleanup: false,
  acknowledgeAgentRemoval: false,
};

const alwaysWarningKeys = [
  'asynchronousJob',
  'servicesUnavailable',
  'trafficInterrupted',
  'adminSessionsInterrupted',
  'managedStateRemoved',
  'activeOperationConflict',
  'reviewJobs',
  'queueOnly',
] as const;

const nonConsequenceKeys = [
  'deleteNodeRecord',
  'eraseInventory',
  'eraseAudit',
  'proveRemoteCompletion',
  'retireNode',
  'revokeAgentIdentity',
  'rebootNode',
] as const;

export function NodeEmergencyCleanupDialog({
  open,
  node,
  diagnostics,
  pending,
  error,
  canBootstrapNode,
  onCancel,
  onConfirm,
}: NodeEmergencyCleanupDialogProps) {
  const { t } = useTranslation();
  const [form, setForm] = useState<NodeEmergencyCleanupForm>(initialForm);
  const [submitted, setSubmitted] = useState(false);
  const [localSubmitting, setLocalSubmitting] = useState(false);
  const wasOpenRef = useRef(false);
  const validation = useMemo(() => validateNodeEmergencyCleanupForm(node, form), [form, node]);
  const safeErrorKey = error ? nodeEmergencyCleanupErrorKey(error) : '';
  const showErrors = submitted
    || Boolean(form.cleanupScope)
    || Boolean(form.confirmation)
    || Boolean(form.reason)
    || form.acknowledgeDestructiveCleanup
    || form.acknowledgeAgentRemoval;
  const disabled = pending || localSubmitting || !canBootstrapNode || !validation.valid;
  const agentStatus = normalizeLifecycleStatus(diagnostics?.agent?.status || node.agent_status || 'unknown');
  const communicationStatus = normalizeLifecycleStatus(diagnostics?.communication_state || node.agent_channel_status || 'unknown');

  const resetForm = useCallback(() => {
    setForm(initialForm);
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
    if (safeErrorKey === 'nodes.lifecycleControls.emergencyCleanup.errors.confirmationMismatchBackend') {
      const timeout = window.setTimeout(() => setForm((current) => ({ ...current, confirmation: '' })), 0);
      return () => window.clearTimeout(timeout);
    }
    return undefined;
  }, [safeErrorKey]);

  const setScope = (cleanupScope: NodeEmergencyCleanupScope | '') => {
    setForm((current) => resetNodeEmergencyCleanupScopeRelationship(current, cleanupScope));
  };

  const setIncludeAgent = (includeAgent: boolean) => {
    setForm((current) => ({
      ...current,
      includeAgent,
      acknowledgeAgentRemoval: includeAgent ? current.acknowledgeAgentRemoval : false,
    }));
  };

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

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    submit();
  };

  const submitting = pending || localSubmitting;
  return (
    <ConfirmDialog open={open} onClose={handleCancel} title={t('nodes.lifecycleControls.emergencyCleanup.dialogTitle')}>
      <form className="page-stack" onSubmit={handleSubmit}>
        <div role="alert" className="error-state-inline">
          <ShieldAlert size={18} aria-hidden="true" />
          <span>{t('nodes.lifecycleControls.emergencyCleanup.emergencyWarning')}</span>
        </div>

        <div className="definition-grid">
          <span>{t('nodes.lifecycleControls.emergencyCleanup.nodeLabel')}</span><strong>{text(node.name || node.id)}</strong>
          <span>{t('nodes.lifecycleControls.emergencyCleanup.maintenanceState')}</span><strong><StatusBadge status={normalizeLifecycleStatus(node.status)} /></strong>
          <span>{t('nodes.lifecycleControls.emergencyCleanup.agentStatus')}</span><strong><StatusBadge status={agentStatus} /></strong>
          <span>{t('nodes.lifecycleControls.emergencyCleanup.communicationStatus')}</span><strong><StatusBadge status={communicationStatus} /></strong>
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.emergencyCleanup.consequencesTitle')}</strong>
          <ul>{alwaysWarningKeys.map((key) => <li key={key}>{t(`nodes.lifecycleControls.emergencyCleanup.consequences.${key}`)}</li>)}</ul>
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.emergencyCleanup.stageScope')}</strong>
          <FormField label={t('nodes.lifecycleControls.emergencyCleanup.scopeLabel')} full>
            <Select
              aria-label={t('nodes.lifecycleControls.emergencyCleanup.scopeLabel')}
              aria-invalid={Boolean(showErrors && validation.errors.cleanupScope)}
              value={form.cleanupScope}
              disabled={submitting}
              onChange={(event) => setScope(event.target.value as NodeEmergencyCleanupScope | '')}
            >
              <option value="">{t('nodes.lifecycleControls.emergencyCleanup.scopePlaceholder')}</option>
              <option value="services_only">{t('nodes.lifecycleControls.emergencyCleanup.servicesOnlyOption')}</option>
              <option value="full_node">{t('nodes.lifecycleControls.emergencyCleanup.fullNodeOption')}</option>
            </Select>
            {showErrors && validation.errors.cleanupScope ? <span role="alert">{t(validation.errors.cleanupScope)}</span> : null}
          </FormField>
          {form.cleanupScope ? (
            <div role="status" aria-live="polite" className="page-stack">
              <strong>{t(`nodes.lifecycleControls.emergencyCleanup.${form.cleanupScope === 'services_only' ? 'servicesOnlyOption' : 'fullNodeOption'}`)}</strong>
              <span>{t(`nodes.lifecycleControls.emergencyCleanup.${form.cleanupScope === 'services_only' ? 'servicesOnlyDescription' : 'fullNodeDescription'}`)}</span>
              <span className="muted">{t('nodes.lifecycleControls.emergencyCleanup.backendPlanBoundary')}</span>
            </div>
          ) : null}
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.emergencyCleanup.stageAgent')}</strong>
          {form.cleanupScope === 'full_node' ? (
            <Checkbox
              checked={form.includeAgent}
              disabled={submitting}
              onChange={(event) => setIncludeAgent(event.target.checked)}
              label={t('nodes.lifecycleControls.emergencyCleanup.includeAgentLabel')}
            />
          ) : (
            <span className="muted">{t('nodes.lifecycleControls.emergencyCleanup.includeAgentUnavailable')}</span>
          )}
          <span className="muted">{t('nodes.lifecycleControls.emergencyCleanup.includeAgentDescription')}</span>
          {showErrors && validation.errors.includeAgent ? <span role="alert">{t(validation.errors.includeAgent)}</span> : null}
          {form.includeAgent ? (
            <div role="alert" aria-live="assertive" className="error-state-inline">
              {t('nodes.lifecycleControls.emergencyCleanup.agentSelfRemovalBoundary')}
            </div>
          ) : null}
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.emergencyCleanup.stageConfirmation')}</strong>
          <FormGrid>
            <FormField label={t('nodes.lifecycleControls.emergencyCleanup.confirmationLabel')}>
              <TextField
                aria-label={t('nodes.lifecycleControls.emergencyCleanup.confirmationLabel')}
                aria-invalid={Boolean(showErrors && validation.errors.confirmation)}
                value={form.confirmation}
                maxLength={513}
                placeholder={validation.expectedConfirmation}
                autoComplete="off"
                disabled={submitting}
                onChange={(event) => setForm((current) => ({ ...current, confirmation: event.target.value }))}
              />
              <span className="muted">{t('nodes.lifecycleControls.emergencyCleanup.confirmationHelp', { value: validation.expectedConfirmation })}</span>
              {showErrors && validation.errors.confirmation ? <span role="alert">{t(validation.errors.confirmation)}</span> : null}
            </FormField>
            <FormField label={t('nodes.lifecycleControls.emergencyCleanup.reasonLabel')}>
              <Textarea
                aria-label={t('nodes.lifecycleControls.emergencyCleanup.reasonLabel')}
                aria-invalid={Boolean(showErrors && validation.errors.reason)}
                value={form.reason}
                maxLength={501}
                rows={4}
                autoComplete="off"
                disabled={submitting}
                onChange={(event) => setForm((current) => ({ ...current, reason: event.target.value }))}
              />
              <span className="muted">{t('nodes.lifecycleControls.emergencyCleanup.reasonHelp')}</span>
              {showErrors && validation.errors.reason ? <span role="alert">{t(validation.errors.reason)}</span> : null}
            </FormField>
          </FormGrid>
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.emergencyCleanup.stageAcknowledgements')}</strong>
          <Checkbox
            checked={form.acknowledgeDestructiveCleanup}
            disabled={submitting}
            onChange={(event) => setForm((current) => ({ ...current, acknowledgeDestructiveCleanup: event.target.checked }))}
            label={t('nodes.lifecycleControls.emergencyCleanup.destructiveAcknowledgement')}
          />
          {showErrors && validation.errors.acknowledgeDestructiveCleanup ? <span role="alert">{t(validation.errors.acknowledgeDestructiveCleanup)}</span> : null}
          {form.includeAgent ? (
            <>
              <Checkbox
                checked={form.acknowledgeAgentRemoval}
                disabled={submitting}
                onChange={(event) => setForm((current) => ({ ...current, acknowledgeAgentRemoval: event.target.checked }))}
                label={t('nodes.lifecycleControls.emergencyCleanup.agentRemovalAcknowledgement')}
              />
              {showErrors && validation.errors.acknowledgeAgentRemoval ? <span role="alert">{t(validation.errors.acknowledgeAgentRemoval)}</span> : null}
            </>
          ) : null}
        </div>

        <div className="inline-panel">
          <strong>{t('nodes.lifecycleControls.emergencyCleanup.nonConsequencesTitle')}</strong>
          <ul>{nonConsequenceKeys.map((key) => <li key={key}>{t(`nodes.lifecycleControls.emergencyCleanup.nonConsequences.${key}`)}</li>)}</ul>
        </div>

        {form.cleanupScope === 'full_node' ? <p className="muted">{t('nodes.lifecycleControls.emergencyCleanup.nginxSafetyNote')}</p> : null}
        {!canBootstrapNode ? <div role="alert" className="error-state-inline">{t('nodes.lifecycleControls.emergencyCleanup.errors.permissionRequired')}</div> : null}
        {safeErrorKey ? <div role="alert" className="error-state-inline">{t(safeErrorKey)}</div> : null}
        {submitting ? <div role="status" aria-live="polite">{t('nodes.lifecycleControls.emergencyCleanup.pending')}</div> : null}
        <p className="muted">{t('nodes.lifecycleControls.emergencyCleanup.queueOnlyDisclaimer')}</p>

        <Toolbar>
          <Button type="submit" variant="danger" disabled={disabled} icon={<ShieldAlert size={16} />}>
            {submitting
              ? t('nodes.lifecycleControls.emergencyCleanup.pending')
              : t(form.includeAgent
                ? 'nodes.lifecycleControls.emergencyCleanup.queueWithAgentRemoval'
                : 'nodes.lifecycleControls.emergencyCleanup.queueButton')}
          </Button>
          <Button type="button" disabled={submitting} onClick={handleCancel}>{t('common.cancel')}</Button>
        </Toolbar>
      </form>
    </ConfirmDialog>
  );
}
