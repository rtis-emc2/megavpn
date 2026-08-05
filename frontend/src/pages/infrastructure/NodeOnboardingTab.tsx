import { DownloadCloud, KeyRound, Play, RefreshCw, RotateCcw } from 'lucide-react';
import { Fragment, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type {
  EnrollmentToken,
  NodeAccessMethod,
  NodeBootstrapRun,
  NodeDetail,
  NodeDiagnostics,
  NodeInventorySnapshot,
} from '../../shared/api/types';
import { Badge, Button, Card, CardBody, FormField, FormGrid, TextField, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import {
  ENROLLMENT_TOKEN_TTL_DEFAULT_HOURS,
  ENROLLMENT_TOKEN_TTL_MAX_HOURS,
  ENROLLMENT_TOKEN_TTL_MIN_HOURS,
  validateEnrollmentTokenTTL,
} from './enrollmentTokenControls';
import {
  deriveNodeOnboardingModel,
  type NodeInventoryJobState,
  type NodeOnboardingStep,
  type NodeOnboardingTargetTab,
} from './nodeOnboarding';
import {
  isNodeBootstrapRunningStatus,
  nodeBootstrapRunStatusGroup,
  type NodeBootstrapMode,
  type NodeBootstrapModeAvailability,
} from './nodeBootstrapReadiness';

type NodeOnboardingTabProps = {
  node: NodeDetail;
  diagnostics?: NodeDiagnostics;
  diagnosticsError?: unknown;
  enrollmentTokens?: EnrollmentToken[];
  enrollmentTokensError?: unknown;
  bootstrapRuns?: NodeBootstrapRun[];
  bootstrapRunsError?: unknown;
  accessMethods?: NodeAccessMethod[];
  accessMethodsError?: unknown;
  inventory?: NodeInventorySnapshot;
  inventoryError?: unknown;
  canBootstrap: boolean;
  canSyncInventory: boolean;
  busy: boolean;
  bootstrapBusy: boolean;
  inventorySyncBusy: boolean;
  trackedBootstrapJobIDs?: string[];
  trackedBootstrapRunIDs?: string[];
  trackedInventoryJobIDs?: string[];
  onOpenTab: (tab: NodeOnboardingTargetTab) => void;
  onRefresh: () => Promise<void>;
  onRequestEnrollmentToken: (input: { mode: 'issue' | 'reissue'; ttlHours: number }) => void;
  onRequestBootstrap: (input: { bootstrapMode: NodeBootstrapMode }) => void;
  onRequestInventorySync: () => void;
  formatError: (error: unknown) => string;
};

function statusLabelKey(status: string): string {
  return `nodes.onboarding.status.${status}`;
}

function stepStatusLabelKey(status: string): string {
  return `nodes.onboarding.stepStatus.${status}`;
}

function targetLabelKey(tab?: NodeOnboardingTargetTab): string {
  switch (tab) {
    case 'security':
      return 'nodes.onboarding.openSecurity';
    case 'bootstrap':
      return 'nodes.onboarding.openBootstrap';
    case 'runtime':
      return 'nodes.onboarding.openRuntime';
    case 'inventory':
      return 'nodes.onboarding.openInventory';
    case 'diagnostics':
      return 'nodes.onboarding.openDiagnostics';
    case 'jobs':
      return 'nodes.onboarding.openJobs';
    default:
      return 'nodes.onboarding.openOverview';
  }
}

function evidenceValue(key: string, value: string, fmt: ReturnType<typeof useLocaleFormat>): string {
  if (value === 'n/a') return value;
  if (key.endsWith('_at')) return fmt.date(value);
  if (key === 'inventory_id') return shortID(value);
  return text(value);
}

function bootstrapModeLabelKey(mode: NodeBootstrapMode): string {
  return mode === 'ssh_bootstrap' ? 'nodes.onboarding.sshBootstrap' : 'nodes.onboarding.manualBootstrapBundle';
}

function bootstrapReasonKey(reasonCode: string): string {
  return `nodes.onboarding.bootstrapReasons.${reasonCode}`;
}

function bootstrapWarningKey(warning: string): string {
  return `nodes.onboarding.bootstrapWarnings.${warning}`;
}

function latestRunTimestamp(run?: NodeBootstrapRun): string | undefined {
  return run?.finished_at || run?.started_at || run?.created_at || undefined;
}

function statusKey(status?: string): string {
  const group = nodeBootstrapRunStatusGroup(status);
  if (group === 'running') return isNodeBootstrapRunningStatus(status) && String(status || '').toLowerCase() === 'running'
    ? 'nodes.onboarding.bootstrapRunning'
    : 'nodes.onboarding.bootstrapQueued';
  if (group === 'succeeded') return 'nodes.onboarding.bootstrapSucceeded';
  if (group === 'failed') {
    const normalized = String(status || '').toLowerCase();
    return normalized === 'cancelled' || normalized === 'canceled'
      ? 'nodes.onboarding.bootstrapCancelled'
      : 'nodes.onboarding.bootstrapFailed';
  }
  return 'nodes.onboarding.bootstrapUnknown';
}

function inventoryJobStateLabelKey(state: NodeInventoryJobState): string {
  return `nodes.onboarding.inventoryJobStates.${state}`;
}

function StepStatusBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  return (
    <span className={`badge status-${status}`}>
      <span className="badge-dot" aria-hidden="true" />
      {t(stepStatusLabelKey(status))}
    </span>
  );
}

function OverallStatusBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  return (
    <span className={`badge status-${status}`}>
      <span className="badge-dot" aria-hidden="true" />
      {t(statusLabelKey(status))}
    </span>
  );
}

function OnboardingStepItem({ step, onOpenTab }: { step: NodeOnboardingStep; onOpenTab: (tab: NodeOnboardingTargetTab) => void }) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  return (
    <li className="page-stack">
      <div className="toolbar">
        <strong>{t(`nodes.onboarding.steps.${step.key}.title`)}</strong>
        <StepStatusBadge status={step.status} />
      </div>
      <p className="muted">{t(`nodes.onboarding.evidence.${step.evidenceCode}`)}</p>
      <div className="definition-grid">
        <span>{t('nodes.onboarding.safeEvidence')}</span>
        <strong>{step.evidence.length ? t('common.yes') : t('common.no')}</strong>
        {step.timestamp ? (
          <>
            <span>{t('nodes.onboarding.evidenceTime')}</span>
            <strong>{fmt.date(step.timestamp)}</strong>
          </>
        ) : null}
        {step.evidence.map((item) => (
          <Fragment key={`${step.key}:${item.key}`}>
            <span>{t(`nodes.onboarding.evidenceLabels.${item.key}`)}</span>
            <strong>{evidenceValue(item.key, item.value, fmt)}</strong>
          </Fragment>
        ))}
      </div>
      {step.targetTab ? (
        <Toolbar>
          <Button type="button" onClick={() => onOpenTab(step.targetTab as NodeOnboardingTargetTab)}>
            {t(targetLabelKey(step.targetTab))}
          </Button>
        </Toolbar>
      ) : null}
    </li>
  );
}

export function NodeOnboardingTab({
  node,
  diagnostics,
  diagnosticsError,
  enrollmentTokens,
  enrollmentTokensError,
  bootstrapRuns,
  bootstrapRunsError,
  accessMethods,
  accessMethodsError,
  inventory,
  inventoryError,
  canBootstrap,
  canSyncInventory,
  busy,
  bootstrapBusy,
  inventorySyncBusy,
  trackedBootstrapJobIDs = [],
  trackedBootstrapRunIDs = [],
  trackedInventoryJobIDs = [],
  onOpenTab,
  onRefresh,
  onRequestEnrollmentToken,
  onRequestBootstrap,
  onRequestInventorySync,
  formatError,
}: NodeOnboardingTabProps) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const [refreshStatus, setRefreshStatus] = useState('');
  const [refreshError, setRefreshError] = useState('');
  const [ttlInput, setTTLInput] = useState(String(ENROLLMENT_TOKEN_TTL_DEFAULT_HOURS));
  const [selectedBootstrapMode, setSelectedBootstrapMode] = useState<NodeBootstrapMode | ''>('');
  const model = useMemo(() => deriveNodeOnboardingModel({
    node,
    diagnostics,
    enrollmentTokens,
    bootstrapRuns,
    accessMethods,
    inventory,
    trackedInventoryJobIDs,
  }), [node, diagnostics, enrollmentTokens, bootstrapRuns, accessMethods, inventory, trackedInventoryJobIDs]);
  const ttlValidation = validateEnrollmentTokenTTL(ttlInput);
  const actionMode = model.recommendedAction === 'issue_enrollment_token'
    ? 'issue'
    : model.recommendedAction === 'reissue_enrollment_token'
      ? 'reissue'
      : null;
  const bootstrapModes = model.availableBootstrapModes;
  const availableBootstrapModes = bootstrapModes.filter((mode) => mode.available);
  const recommendedBootstrapAction = model.recommendedAction === 'start_ssh_bootstrap'
    ? 'ssh_bootstrap'
    : model.recommendedAction === 'start_manual_bundle'
      ? 'manual_bundle'
      : undefined;
  const defaultBootstrapMode = model.bootstrapSelectionRequired ? undefined : (recommendedBootstrapAction || model.recommendedBootstrapMode || availableBootstrapModes[0]?.mode);
  const effectiveBootstrapMode = selectedBootstrapMode && availableBootstrapModes.some((mode) => mode.mode === selectedBootstrapMode)
    ? selectedBootstrapMode
    : defaultBootstrapMode;
  const selectedBootstrapAvailability = bootstrapModes.find((mode) => mode.mode === effectiveBootstrapMode);
  const latestBootstrapRun = (bootstrapRuns || []).find((run) => run.id === model.latestBootstrapRunID);
  const latestBootstrapStatusGroup = nodeBootstrapRunStatusGroup(model.latestBootstrapStatus);
  const agent = diagnostics?.agent;
  const registrationTime = agent?.registered_at;
  const heartbeatTime = node.last_heartbeat_at || agent?.last_seen_at || node.agent_last_seen_at;
  const latestInventoryTime = agent?.last_inventory_sync_at || diagnostics?.latest_inventory?.created_at || inventory?.created_at;
  const showBootstrapSection = !actionMode && (
    model.currentStep === 'bootstrap'
    || model.bootstrapSelectionRequired
    || Boolean(recommendedBootstrapAction)
    || (!model.registered && Boolean(model.latestBootstrapRunID))
    || (model.currentStep === 'registration' && !model.registered)
  );
  const showInventorySection = !actionMode && !showBootstrapSection && (
    model.currentStep === 'inventory'
    || model.recommendedAction === 'sync_inventory'
    || model.inventoryJobState !== 'not_requested'
    || model.inventoryObserved
    || model.overallStatus === 'ready'
  );
  const showHeartbeatWaitingSection = !actionMode && !showBootstrapSection && !showInventorySection && model.currentStep === 'heartbeat' && model.registered && !model.heartbeatObserved;
  const showRegistrationWaitingSection = !actionMode && !showBootstrapSection && !showInventorySection && !showHeartbeatWaitingSection && model.currentStep === 'registration' && !model.registered;
  const errors: Array<{ key: string; error: unknown }> = [];
  if (diagnosticsError) errors.push({ key: 'diagnosticsUnavailable', error: diagnosticsError });
  if (enrollmentTokensError) errors.push({ key: 'enrollmentTokensUnavailable', error: enrollmentTokensError });
  if (bootstrapRunsError) errors.push({ key: 'bootstrapRunsUnavailable', error: bootstrapRunsError });
  if (accessMethodsError) errors.push({ key: 'accessMethodsUnavailable', error: accessMethodsError });
  if (inventoryError) errors.push({ key: 'inventoryUnavailable', error: inventoryError });

  const refresh = async () => {
    setRefreshStatus('');
    setRefreshError('');
    try {
      await onRefresh();
      setRefreshStatus(t('nodes.onboarding.statusRefreshed'));
    } catch (error) {
      setRefreshError(formatError(error));
    }
  };

  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-stack">
            <h3 className="card-title">{t('nodes.onboarding.agentOnboarding')}</h3>
            <Toolbar>
              <Badge>{t('nodes.onboarding.readOnlyStatus')}</Badge>
              <Badge>{t('nodes.onboarding.liveValidationNotProven')}</Badge>
              <OverallStatusBadge status={model.overallStatus} />
            </Toolbar>
            <div className="definition-grid">
              <span>{t('nodes.onboarding.statusTitle')}</span><strong>{t(statusLabelKey(model.overallStatus))}</strong>
              <span>{t('nodes.onboarding.currentStep')}</span><strong>{t(`nodes.onboarding.steps.${model.currentStep}.title`)}</strong>
              <span>{t('nodes.communicationState')}</span><strong>{model.communicationState}</strong>
              <span>{t('nodes.heartbeatState')}</span><strong>{model.heartbeatState}</strong>
              <span>{t('nodes.tokenRotationStatus')}</span><strong>{model.tokenRotationStatus}</strong>
            </div>
            {!canBootstrap ? <p className="muted">{t('nodes.onboarding.bootstrapPermissionHint')}</p> : null}
            {errors.map((item) => (
              <div key={item.key} role="alert" className="error-state-inline">
                {t(`nodes.${item.key}`)}: {formatError(item.error)}
              </div>
            ))}
            {refreshStatus ? <div role="status">{refreshStatus}</div> : null}
            {refreshError ? <div role="alert" className="error-state-inline">{refreshError}</div> : null}
            <Toolbar>
              <Button type="button" icon={<RefreshCw size={16} />} onClick={() => void refresh()}>
                {t('nodes.onboarding.refreshStatus')}
              </Button>
              {model.targetTab ? (
                <Button type="button" onClick={() => onOpenTab(model.targetTab as NodeOnboardingTargetTab)}>
                  {t(targetLabelKey(model.targetTab))}
                </Button>
              ) : null}
            </Toolbar>
          </div>
        </CardBody>
      </Card>
      {canBootstrap && actionMode ? (
        <Card>
          <CardBody>
            <div className="page-stack">
              <h3 className="card-title">
                {actionMode === 'issue' ? t('nodes.onboarding.issueEnrollmentToken') : t('nodes.onboarding.reissueEnrollmentToken')}
              </h3>
              <p className="muted">
                {actionMode === 'issue' ? t('nodes.onboarding.issueEnrollmentTokenHint') : t('nodes.onboarding.reissueEnrollmentTokenHint')}
              </p>
              <ul>
                <li>{t('nodes.onboarding.tokenShownOnlyOnce')}</li>
                <li>{t('nodes.onboarding.tokenIssueDoesNotProveConnectivity')}</li>
                <li>{t('nodes.onboarding.guidedBootstrapRemainsNext')}</li>
              </ul>
              <FormGrid>
                <FormField label={t('nodes.onboarding.enrollmentTokenTTL')}>
                  <TextField
                    type="number"
                    min={ENROLLMENT_TOKEN_TTL_MIN_HOURS}
                    max={ENROLLMENT_TOKEN_TTL_MAX_HOURS}
                    step={1}
                    value={ttlInput}
                    onChange={(event) => setTTLInput(event.target.value)}
                  />
                </FormField>
              </FormGrid>
              {ttlValidation.errorKey ? (
                <div role="alert" className="error-state-inline">
                  {t(`nodes.onboarding.tokenTTLErrors.${ttlValidation.errorKey}`, {
                    min: ENROLLMENT_TOKEN_TTL_MIN_HOURS,
                    max: ENROLLMENT_TOKEN_TTL_MAX_HOURS,
                  })}
                </div>
              ) : null}
              <Toolbar>
                <Button
                  type="button"
                  variant={actionMode === 'reissue' ? 'danger' : 'primary'}
                  icon={actionMode === 'reissue' ? <RotateCcw size={16} /> : <KeyRound size={16} />}
                  disabled={busy || Boolean(ttlValidation.errorKey)}
                  onClick={() => onRequestEnrollmentToken({ mode: actionMode, ttlHours: ttlValidation.ttlHours })}
                >
                  {actionMode === 'issue' ? t('nodes.onboarding.issueEnrollmentToken') : t('nodes.onboarding.reissueEnrollmentToken')}
                </Button>
              </Toolbar>
            </div>
          </CardBody>
        </Card>
      ) : showBootstrapSection ? (
        <Card>
          <CardBody>
            <div className="page-stack">
              <h3 className="card-title">{t('nodes.onboarding.chooseBootstrapMethod')}</h3>
              {latestBootstrapRun ? (
                <div className="definition-grid">
                  <span>{t('nodes.bootstrapRun')}</span><strong><code>{shortID(latestBootstrapRun.id)}</code></strong>
                  <span>{t('nodes.job')}</span><strong><code>{shortID(latestBootstrapRun.job_id || undefined)}</code></strong>
                  <span>{t('common.status')}</span><strong>{t(statusKey(latestBootstrapRun.status))}</strong>
                  <span>{t('nodes.bootstrapMode')}</span><strong>{text(latestBootstrapRun.bootstrap_mode)}</strong>
                  {latestRunTimestamp(latestBootstrapRun) ? (
                    <>
                      <span>{t('nodes.onboarding.evidenceTime')}</span>
                      <strong>{fmt.date(latestRunTimestamp(latestBootstrapRun))}</strong>
                    </>
                  ) : null}
                </div>
              ) : null}
              {latestBootstrapStatusGroup === 'running' ? <p className="muted">{t('nodes.onboarding.concurrentBootstrapActive')}</p> : null}
              {latestBootstrapStatusGroup === 'succeeded' ? <p className="muted">{t('nodes.onboarding.waitingForAgentRegistration')}</p> : null}
              {latestBootstrapStatusGroup === 'failed' ? <p className="muted">{t('nodes.onboarding.bootstrapFailureRetryHint')}</p> : null}
              {latestBootstrapStatusGroup === 'unknown' && latestBootstrapRun ? <p className="muted">{t('nodes.onboarding.bootstrapUnknown')}</p> : null}
              {latestBootstrapRun?.manual_bundle_available === true ? (
                <div className="inline-panel">
                  <strong>{t('nodes.onboarding.manualBundleAvailable')}</strong>
                  <p className="muted">{t('nodes.onboarding.manualBundleOpenBootstrapHint')}</p>
                </div>
              ) : null}
              <div className="page-stack">
                <strong>{t('nodes.onboarding.bootstrapPrerequisites')}</strong>
                {bootstrapModes.map((mode: NodeBootstrapModeAvailability) => (
                  <label key={mode.mode} className="inline-panel">
                    <div className="toolbar">
                      <input
                        type="radio"
                        name={`bootstrap-mode-${node.id}`}
                        value={mode.mode}
                        checked={effectiveBootstrapMode === mode.mode}
                        disabled={!mode.available || bootstrapBusy || latestBootstrapStatusGroup === 'running'}
                        onChange={() => setSelectedBootstrapMode(mode.mode)}
                      />
                      <strong>{t(bootstrapModeLabelKey(mode.mode))}</strong>
                      {mode.recommended ? <Badge>{t('nodes.onboarding.recommended')}</Badge> : null}
                      <Badge>{mode.available ? t('nodes.onboarding.available') : t('nodes.onboarding.unavailable')}</Badge>
                    </div>
                    <p className="muted">{t(bootstrapReasonKey(mode.reasonCode))}</p>
                    {mode.mode === 'ssh_bootstrap' ? (
                      <div className="definition-grid">
                        <span>{t('nodes.sshHost')}</span><strong>{text(mode.sshTarget?.host)}</strong>
                        <span>{t('nodes.sshPort')}</span><strong>{mode.sshTarget?.port || 'n/a'}</strong>
                        <span>{t('nodes.sshUser')}</span><strong>{text(mode.sshTarget?.user)}</strong>
                        <span>{t('nodes.onboarding.sshAccessMethodReady')}</span><strong>{mode.available ? t('common.yes') : t('common.no')}</strong>
                        <span>{t('nodes.onboarding.sshHostKeyConfirmed')}</span><strong>{mode.sshTarget?.hostKeyConfigured ? t('common.yes') : t('common.no')}</strong>
                        <span>{t('nodes.onboarding.sshCredentialConfigured')}</span><strong>{mode.sshTarget?.secretConfigured ? t('common.yes') : t('common.no')}</strong>
                      </div>
                    ) : (
                      <p className="muted">{t('nodes.onboarding.manualExecutionRequired')}</p>
                    )}
                    {mode.warnings.length ? (
                      <ul>
                        {mode.warnings.map((warning) => <li key={warning}>{t(bootstrapWarningKey(warning))}</li>)}
                      </ul>
                    ) : null}
                  </label>
                ))}
              </div>
              {trackedBootstrapJobIDs.length ? (
                <p className="muted">
                  {t('nodes.onboarding.bootstrapJobAccepted')}: {trackedBootstrapJobIDs.map((jobId) => shortID(jobId)).join(', ')}
                </p>
              ) : null}
              {trackedBootstrapRunIDs.length ? (
                <p className="muted">
                  {t('nodes.onboarding.bootstrapRunTracked')}: {trackedBootstrapRunIDs.map((runId) => shortID(runId)).join(', ')}
                </p>
              ) : null}
              <Toolbar>
                <Button
                  type="button"
                  variant={latestBootstrapStatusGroup === 'failed' ? 'danger' : 'primary'}
                  icon={<Play size={16} />}
                  disabled={!canBootstrap || bootstrapBusy || busy || !effectiveBootstrapMode || !selectedBootstrapAvailability?.available || latestBootstrapStatusGroup === 'running'}
                  onClick={() => effectiveBootstrapMode && onRequestBootstrap({ bootstrapMode: effectiveBootstrapMode })}
                >
                  {latestBootstrapStatusGroup === 'failed' ? t('nodes.onboarding.retryBootstrap') : t('nodes.onboarding.startBootstrap')}
                </Button>
                <Button type="button" onClick={() => onOpenTab('bootstrap')}>{t('nodes.onboarding.openBootstrap')}</Button>
                {trackedBootstrapJobIDs.length ? <Button type="button" onClick={() => onOpenTab('jobs')}>{t('nodes.onboarding.openJobs')}</Button> : null}
              </Toolbar>
              {!canBootstrap ? <p className="muted">{t('nodes.onboarding.bootstrapPermissionHint')}</p> : null}
              <p className="muted">{t('nodes.onboarding.guidedInventoryRequiresRegistrationHeartbeat')}</p>
            </div>
          </CardBody>
        </Card>
      ) : showInventorySection ? (
        <Card>
          <CardBody>
            <div className="page-stack">
              <h3 className="card-title">{t('nodes.onboarding.synchronizeInventory')}</h3>
              <p className="muted">{t('nodes.onboarding.inventoryAsyncJob')}</p>
              <div className="definition-grid">
                <span>{t('nodes.onboarding.agentRegistrationObserved')}</span><strong>{model.registered ? t('common.yes') : t('common.no')}</strong>
                <span>{t('nodes.onboarding.evidenceTime')}</span><strong>{fmt.date(registrationTime)}</strong>
                <span>{t('nodes.onboarding.firstHeartbeatObserved')}</span><strong>{model.heartbeatObserved ? t('common.yes') : t('common.no')}</strong>
                <span>{t('nodes.lastSeen')}</span><strong>{fmt.date(heartbeatTime)}</strong>
                <span>{t('nodes.heartbeatState')}</span><strong>{model.heartbeatState}</strong>
                <span>{t('nodes.communicationState')}</span><strong>{model.communicationState}</strong>
                <span>{t('nodes.lastInventory')}</span><strong>{fmt.date(latestInventoryTime)}</strong>
                <span>{t('nodes.onboarding.inventoryJobState')}</span><strong>{t(inventoryJobStateLabelKey(model.inventoryJobState))}</strong>
                {model.latestInventoryJobID ? (
                  <>
                    <span>{t('nodes.job')}</span><strong><code>{shortID(model.latestInventoryJobID)}</code></strong>
                  </>
                ) : null}
                {model.latestInventoryJobStatus ? (
                  <>
                    <span>{t('common.status')}</span><strong>{text(model.latestInventoryJobStatus)}</strong>
                  </>
                ) : null}
                {model.latestInventoryJobAt ? (
                  <>
                    <span>{t('nodes.onboarding.jobEvidenceTime')}</span><strong>{fmt.date(model.latestInventoryJobAt)}</strong>
                  </>
                ) : null}
              </div>
              {model.inventoryObserved ? <p className="muted">{t('nodes.onboarding.inventorySynchronized')}</p> : null}
              {model.inventorySyncInProgress ? <p className="muted">{model.inventoryJobState === 'running' ? t('nodes.onboarding.inventoryJobRunning') : t('nodes.onboarding.inventoryJobAccepted')}</p> : null}
              {model.inventorySyncStalled ? <div role="alert" className="error-state-inline">{t('nodes.onboarding.inventoryResultStalled')}</div> : null}
              {model.inventorySyncFailed ? <div role="alert" className="error-state-inline">{t('nodes.onboarding.inventorySynchronizationFailed')}</div> : null}
              {!model.heartbeatObserved ? <p className="muted">{t('nodes.onboarding.waitingForFirstHeartbeat')}</p> : null}
              {model.heartbeatObserved && model.heartbeatState !== 'online' ? <p className="muted">{t('nodes.onboarding.heartbeatMustBeOnline')}</p> : null}
              {trackedInventoryJobIDs.length ? (
                <p className="muted">
                  {t('nodes.onboarding.inventoryJobAccepted')}: {trackedInventoryJobIDs.map((jobId) => shortID(jobId)).join(', ')}
                </p>
              ) : null}
              {model.overallStatus === 'ready' ? (
                <div className="inline-panel">
                  <strong>{t('nodes.onboarding.onboardingReady')}</strong>
                  <p className="muted">{t('nodes.onboarding.controlPlaneMilestonesComplete')}</p>
                  <p className="muted">{t('nodes.onboarding.liveValidationStillRequired')}</p>
                </div>
              ) : null}
              <Toolbar>
                {model.recommendedAction === 'sync_inventory' && canSyncInventory ? (
                  <Button
                    type="button"
                    variant={model.inventorySyncFailed ? 'danger' : 'primary'}
                    icon={<DownloadCloud size={16} />}
                    disabled={inventorySyncBusy || busy || !model.inventorySyncEligible}
                    onClick={onRequestInventorySync}
                  >
                    {model.inventorySyncFailed ? t('nodes.onboarding.retryInventorySynchronization') : t('nodes.onboarding.synchronizeInventory')}
                  </Button>
                ) : null}
                <Button type="button" onClick={() => onOpenTab('inventory')}>{t('nodes.onboarding.openInventory')}</Button>
                {trackedInventoryJobIDs.length || model.latestInventoryJobID ? <Button type="button" onClick={() => onOpenTab('jobs')}>{t('nodes.onboarding.openJobs')}</Button> : null}
                <Button type="button" onClick={() => onOpenTab('diagnostics')}>{t('nodes.onboarding.openDiagnostics')}</Button>
              </Toolbar>
              {model.recommendedAction === 'sync_inventory' && !canSyncInventory ? <p className="muted">{t('nodes.onboarding.inventoryPermissionHint')}</p> : null}
            </div>
          </CardBody>
        </Card>
      ) : showHeartbeatWaitingSection ? (
        <Card>
          <CardBody>
            <div className="page-stack">
              <p className="muted">{t('nodes.onboarding.waitingForFirstHeartbeat')}</p>
              <Toolbar>
                <Button type="button" onClick={() => onOpenTab('runtime')}>{t('nodes.onboarding.openRuntime')}</Button>
                <Button type="button" onClick={() => onOpenTab('diagnostics')}>{t('nodes.onboarding.openDiagnostics')}</Button>
              </Toolbar>
            </div>
          </CardBody>
        </Card>
      ) : showRegistrationWaitingSection ? (
        <Card>
          <CardBody>
            <div className="page-stack">
              <p className="muted">{t('nodes.onboarding.waitingForAgentRegistration')}</p>
              <Toolbar>
                <Button type="button" onClick={() => onOpenTab('runtime')}>{t('nodes.onboarding.openRuntime')}</Button>
                <Button type="button" onClick={() => onOpenTab('diagnostics')}>{t('nodes.onboarding.openDiagnostics')}</Button>
              </Toolbar>
            </div>
          </CardBody>
        </Card>
      ) : null}
      <Card>
        <CardBody>
          <ol className="page-stack" aria-label={t('nodes.onboarding.agentOnboarding')}>
            {model.steps.map((step) => (
              <OnboardingStepItem key={step.key} step={step} onOpenTab={onOpenTab} />
            ))}
          </ol>
        </CardBody>
      </Card>
    </div>
  );
}
