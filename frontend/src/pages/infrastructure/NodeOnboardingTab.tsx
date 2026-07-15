import { KeyRound, RefreshCw, RotateCcw } from 'lucide-react';
import { Fragment, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type {
  EnrollmentToken,
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
  type NodeOnboardingStep,
  type NodeOnboardingTargetTab,
} from './nodeOnboarding';

type NodeOnboardingTabProps = {
  node: NodeDetail;
  diagnostics?: NodeDiagnostics;
  diagnosticsError?: unknown;
  enrollmentTokens?: EnrollmentToken[];
  enrollmentTokensError?: unknown;
  bootstrapRuns?: NodeBootstrapRun[];
  bootstrapRunsError?: unknown;
  inventory?: NodeInventorySnapshot;
  inventoryError?: unknown;
  canBootstrap: boolean;
  busy: boolean;
  onOpenTab: (tab: NodeOnboardingTargetTab) => void;
  onRefresh: () => Promise<void>;
  onRequestEnrollmentToken: (input: { mode: 'issue' | 'reissue'; ttlHours: number }) => void;
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
  inventory,
  inventoryError,
  canBootstrap,
  busy,
  onOpenTab,
  onRefresh,
  onRequestEnrollmentToken,
  formatError,
}: NodeOnboardingTabProps) {
  const { t } = useTranslation();
  const [refreshStatus, setRefreshStatus] = useState('');
  const [refreshError, setRefreshError] = useState('');
  const [ttlInput, setTTLInput] = useState(String(ENROLLMENT_TOKEN_TTL_DEFAULT_HOURS));
  const model = useMemo(() => deriveNodeOnboardingModel({
    node,
    diagnostics,
    enrollmentTokens,
    bootstrapRuns,
    inventory,
  }), [node, diagnostics, enrollmentTokens, bootstrapRuns, inventory]);
  const ttlValidation = validateEnrollmentTokenTTL(ttlInput);
  const actionMode = model.recommendedAction === 'issue_enrollment_token'
    ? 'issue'
    : model.recommendedAction === 'reissue_enrollment_token'
      ? 'reissue'
      : null;
  const errors: Array<{ key: string; error: unknown }> = [];
  if (diagnosticsError) errors.push({ key: 'diagnosticsUnavailable', error: diagnosticsError });
  if (enrollmentTokensError) errors.push({ key: 'enrollmentTokensUnavailable', error: enrollmentTokensError });
  if (bootstrapRunsError) errors.push({ key: 'bootstrapRunsUnavailable', error: bootstrapRunsError });
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
      ) : model.currentStep === 'registration' && !model.registered ? (
        <Card>
          <CardBody>
            <p className="muted">{t('nodes.onboarding.waitingForAgentRegistration')}</p>
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
