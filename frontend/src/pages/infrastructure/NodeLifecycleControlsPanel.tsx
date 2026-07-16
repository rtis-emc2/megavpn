import { RefreshCw, RotateCcw, ShieldAlert, TriangleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { NodeDetail, NodeDiagnostics, NodeStaleRotationCandidate, NodeStaleRotationPreview } from '../../shared/api/types';
import { Badge, Button, Card, CardBody, DataTable, EmptyState, LoadingSkeleton, StatusBadge, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import {
  deriveNodeLifecycleStatusModel,
  deriveNodeRebootActionState,
  describeStaleRotationReason,
  formatAgeSeconds,
  nodeAgentIdentityExpectedConfirmation,
  normalizeLifecycleStatus,
  staleRotationPreviewErrorKey,
  type NodeLifecycleSeverity,
} from './nodeLifecycleControls';
import { deriveNodeEmergencyCleanupActionState } from './nodeEmergencyCleanup';

type NodeLifecycleControlsPanelProps = {
  node: NodeDetail;
  diagnostics?: NodeDiagnostics;
  staleRotationPreview?: NodeStaleRotationPreview;
  staleRotationPreviewError?: unknown;
  staleRotationPreviewLoading: boolean;
  staleRotationPreviewFetching: boolean;
  canReadNode: boolean;
  canBootstrapNode: boolean;
  lifecycleDataCurrent: boolean;
  revokePending: boolean;
  rebootPending: boolean;
  emergencyCleanupPending: boolean;
  onOpenRevokeDialog: () => void;
  onOpenRebootDialog: () => void;
  onOpenEmergencyCleanupDialog: () => void;
  onRefreshStaleRotationPreview: () => void;
};

function statusForSeverity(severity: NodeLifecycleSeverity): string {
  if (severity === 'healthy') return 'ready';
  if (severity === 'warning') return 'warning';
  if (severity === 'blocked') return 'danger';
  return 'unknown';
}

function lifecycleStatusValue(value: string) {
  return <StatusBadge status={value || 'unknown'} />;
}

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

export function NodeLifecycleControlsPanel({
  node,
  diagnostics,
  staleRotationPreview,
  staleRotationPreviewError,
  staleRotationPreviewLoading,
  staleRotationPreviewFetching,
  canReadNode,
  canBootstrapNode,
  lifecycleDataCurrent,
  revokePending,
  rebootPending,
  emergencyCleanupPending,
  onOpenRevokeDialog,
  onOpenRebootDialog,
  onOpenEmergencyCleanupDialog,
  onRefreshStaleRotationPreview,
}: NodeLifecycleControlsPanelProps) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const model = deriveNodeLifecycleStatusModel({ node, diagnostics, staleRotationPreview });
  const candidates = staleRotationPreview?.candidates || [];
  const agentIdentityStatus = normalizeLifecycleStatus(diagnostics?.agent?.status || node.agent_status || '');
  const identityAlreadyRevoked = agentIdentityStatus === 'revoked';
  const identityMissing = ['missing', 'none', 'not_found', 'deleted'].includes(agentIdentityStatus);
  const identityUnknown = !agentIdentityStatus || agentIdentityStatus === 'unknown';
  const revokeAllowed = canBootstrapNode && lifecycleDataCurrent && !identityAlreadyRevoked && !identityMissing;
  const rebootState = deriveNodeRebootActionState({ node, diagnostics, canBootstrapNode, lifecycleDataCurrent });
  const emergencyCleanupState = deriveNodeEmergencyCleanupActionState({ node, diagnostics, canBootstrapNode, lifecycleDataCurrent });
  const communicationState = normalizeLifecycleStatus(diagnostics?.communication_state || node.agent_channel_status || '');

  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-stack">
            <Toolbar>
              <StatusBadge status={statusForSeverity(model.overallSeverity)} />
              <Badge>{t('common.readOnly')}</Badge>
            </Toolbar>
            <div>
              <h3 className="card-title">{t('nodes.lifecycleControls.statusTitle')}</h3>
              <p className="muted">{t(model.overallStatusKey)}</p>
            </div>
            <div className="definition-grid">
              {model.items.map((item) => (
                <div key={item.key} style={{ display: 'contents' }}>
                  <span>{t(item.labelKey)}</span>
                  <strong>{lifecycleStatusValue(item.value)}</strong>
                </div>
              ))}
              <span>{t('nodes.lifecycleControls.staleRotationDetected')}</span>
              <strong>{model.staleRotation.detected ? t('common.yes') : t('common.no')}</strong>
              <span>{t('nodes.lifecycleControls.backendSafeCandidates')}</span>
              <strong>{model.staleRotation.backendSafeCandidateCount}</strong>
              <span>{t('nodes.lifecycleControls.evaluatedAt')}</span>
              <strong>{fmt.date(model.staleRotation.evaluatedAt)}</strong>
            </div>
            <div className="inline-panel" aria-label={t('nodes.lifecycleControls.deferredActionsLabel')}>
              <strong>{t('nodes.lifecycleControls.deferredActionsTitle')}</strong>
              <span className="muted">{t('nodes.lifecycleControls.deferredActionsBody')}</span>
            </div>
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardBody>
          <div className="page-stack">
            <Toolbar>
              <StatusBadge status={emergencyCleanupState.available ? 'danger' : 'blocked'} />
              <Badge>{t('nodes.lifecycleControls.emergencyCleanup.maintenanceBadge')}</Badge>
            </Toolbar>
            <div>
              <h3 className="card-title">{t('nodes.lifecycleControls.emergencyCleanup.actionTitle')}</h3>
              <p className="muted">{t('nodes.lifecycleControls.emergencyCleanup.actionDescription')}</p>
            </div>
            <div className="definition-grid">
              <span>{t('nodes.lifecycleControls.emergencyCleanup.maintenanceState')}</span>
              <strong>{lifecycleStatusValue(node.status || 'unknown')}</strong>
              <span>{t('nodes.lifecycleControls.emergencyCleanup.agentStatus')}</span>
              <strong>{lifecycleStatusValue(agentIdentityStatus || 'unknown')}</strong>
              <span>{t('nodes.lifecycleControls.emergencyCleanup.communicationStatus')}</span>
              <strong>{lifecycleStatusValue(communicationState || 'unknown')}</strong>
            </div>
            {emergencyCleanupState.blockedKey ? (
              <div role="alert" className="error-state-inline">{t(emergencyCleanupState.blockedKey, { permission: 'node.bootstrap' })}</div>
            ) : null}
            <p className="muted">{t('nodes.lifecycleControls.emergencyCleanup.maintenanceGuidance')}</p>
            <Toolbar>
              {emergencyCleanupState.available ? (
                <Button
                  variant="danger"
                  icon={<TriangleAlert size={16} />}
                  disabled={emergencyCleanupPending}
                  onClick={onOpenEmergencyCleanupDialog}
                >
                  {emergencyCleanupPending
                    ? t('nodes.lifecycleControls.emergencyCleanup.pending')
                    : t('nodes.lifecycleControls.emergencyCleanup.openAction')}
                </Button>
              ) : null}
            </Toolbar>
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardBody>
          <div className="page-stack">
            <Toolbar>
              <StatusBadge status={rebootState.available ? 'warning' : 'blocked'} />
              {rebootState.unknownState ? <Badge>{t('nodes.lifecycleControls.nodeReboot.unknownStateBadge')}</Badge> : null}
            </Toolbar>
            <div>
              <h3 className="card-title">{t('nodes.lifecycleControls.nodeReboot.actionTitle')}</h3>
              <p className="muted">{t('nodes.lifecycleControls.nodeReboot.actionDescription')}</p>
            </div>
            <div className="definition-grid">
              <span>{t('nodes.lifecycleControls.nodeReboot.communicationStatus')}</span>
              <strong>{lifecycleStatusValue(communicationState || 'unknown')}</strong>
              <span>{t('nodes.lifecycleControls.nodeReboot.agentStatus')}</span>
              <strong>{lifecycleStatusValue(agentIdentityStatus || 'unknown')}</strong>
            </div>
            {rebootState.blockedKey ? (
              <div role="alert" className="error-state-inline">{t(rebootState.blockedKey, { permission: 'node.bootstrap' })}</div>
            ) : null}
            {rebootState.available && rebootState.unknownState ? (
              <div role="alert" className="error-state-inline">{t('nodes.lifecycleControls.nodeReboot.unknownStateWarning')}</div>
            ) : null}
            <Toolbar>
              {rebootState.available ? (
                <Button
                  variant="danger"
                  icon={<RotateCcw size={16} />}
                  disabled={rebootPending}
                  onClick={onOpenRebootDialog}
                >
                  {rebootPending ? t('nodes.lifecycleControls.nodeReboot.pending') : t('nodes.lifecycleControls.nodeReboot.buttonLabel')}
                </Button>
              ) : null}
            </Toolbar>
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardBody>
          <div className="page-stack">
            <Toolbar>
              <StatusBadge status={agentIdentityStatus || 'unknown'} />
              {identityUnknown ? <Badge>{t('nodes.lifecycleControls.agentIdentityRevoke.statusUnknown')}</Badge> : null}
              {identityAlreadyRevoked ? <Badge>{t('nodes.lifecycleControls.agentIdentityRevoke.alreadyRevokedBadge')}</Badge> : null}
            </Toolbar>
            <div>
              <h3 className="card-title">{t('nodes.lifecycleControls.agentIdentityRevoke.actionTitle')}</h3>
              <p className="muted">{t('nodes.lifecycleControls.agentIdentityRevoke.actionDescription')}</p>
            </div>
            <div className="definition-grid">
              <span>{t('nodes.lifecycleControls.agentIdentityRevoke.identityStatus')}</span>
              <strong>{lifecycleStatusValue(agentIdentityStatus || 'unknown')}</strong>
              <span>{t('nodes.lifecycleControls.agentIdentityRevoke.confirmationTarget')}</span>
              <strong>{nodeAgentIdentityExpectedConfirmation(node)}</strong>
            </div>
            {!canBootstrapNode ? (
              <div role="alert" className="error-state-inline">{t('common.permissionRequired', { permission: 'node.bootstrap' })}</div>
            ) : null}
            {canBootstrapNode && !lifecycleDataCurrent ? (
              <div role="alert" className="error-state-inline">{t('nodes.lifecycleControls.agentIdentityRevoke.lifecycleDataStale')}</div>
            ) : null}
            {canBootstrapNode && identityAlreadyRevoked ? (
              <div role="status" className="inline-panel">{t('nodes.lifecycleControls.agentIdentityRevoke.identityAlreadyRevokedState')}</div>
            ) : null}
            {canBootstrapNode && identityMissing ? (
              <div role="status" className="inline-panel">{t('nodes.lifecycleControls.agentIdentityRevoke.identityMissingState')}</div>
            ) : null}
            {canBootstrapNode && identityUnknown ? (
              <div role="alert" className="error-state-inline">{t('nodes.lifecycleControls.agentIdentityRevoke.identityUnknownWarning')}</div>
            ) : null}
            <Toolbar>
              {revokeAllowed ? (
                <Button
                  variant="danger"
                  icon={<ShieldAlert size={16} />}
                  disabled={revokePending}
                  onClick={onOpenRevokeDialog}
                >
                  {revokePending ? t('nodes.lifecycleControls.agentIdentityRevoke.pending') : t('nodes.lifecycleControls.agentIdentityRevoke.buttonLabel')}
                </Button>
              ) : null}
            </Toolbar>
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardBody>
          <div className="page-stack">
            <Toolbar>
              <StatusBadge status={statusForSeverity(model.staleRotation.severity)} />
              <Button
                icon={<RefreshCw size={16} />}
                disabled={!canReadNode || staleRotationPreviewLoading || staleRotationPreviewFetching}
                onClick={onRefreshStaleRotationPreview}
              >
                {staleRotationPreviewFetching ? t('nodes.lifecycleControls.refreshingPreview') : t('nodes.lifecycleControls.refreshPreview')}
              </Button>
            </Toolbar>
            <div>
              <h3 className="card-title">{t('nodes.lifecycleControls.staleRotationTitle')}</h3>
              <p className="muted">{t('nodes.lifecycleControls.staleRotationBody')}</p>
            </div>
            {!canReadNode ? (
              <div role="alert" className="error-state-inline">{t('common.permissionRequired', { permission: 'node.read' })}</div>
            ) : null}
            {canReadNode && staleRotationPreviewLoading ? <LoadingSkeleton label={t('nodes.lifecycleControls.loadingPreview')} /> : null}
            {canReadNode && staleRotationPreviewError ? (
              <div role="alert" className="error-state-inline">{t(staleRotationPreviewErrorKey(staleRotationPreviewError))}</div>
            ) : null}
            {canReadNode && staleRotationPreview && !candidates.length ? (
              <EmptyState title={t('nodes.lifecycleControls.noCandidatesTitle')} body={t('nodes.lifecycleControls.noCandidatesBody')} />
            ) : null}
            {canReadNode && staleRotationPreview && candidates.length ? (
              <>
                <div className="definition-grid" aria-live="polite">
                  <span>{t('nodes.lifecycleControls.previewNode')}</span><strong>{text(staleRotationPreview.node_id)}</strong>
                  <span>{t('nodes.lifecycleControls.tokenRotationStatus')}</span><strong>{lifecycleStatusValue(staleRotationPreview.token_rotation_status)}</strong>
                  <span>{t('nodes.lifecycleControls.candidateCount')}</span><strong>{candidates.length}</strong>
                  <span>{t('nodes.lifecycleControls.unknownReasons')}</span><strong>{model.staleRotation.unknownReasonCount}</strong>
                </div>
                <DataTable<NodeStaleRotationCandidate>
                  rows={candidates}
                  title={t('nodes.lifecycleControls.candidatesTitle')}
                  columns={[
                    { key: 'job', header: t('nodes.lifecycleControls.job'), render: (row) => <code>{shortID(row.job_id)}</code> },
                    { key: 'status', header: t('common.status'), render: (row) => lifecycleStatusValue(row.status) },
                    { key: 'reason', header: t('nodes.lifecycleControls.reason'), render: (row) => <CandidateReason candidate={row} /> },
                    { key: 'safe', header: t('nodes.lifecycleControls.safeToClear'), render: (row) => row.safe_to_clear ? t('common.yes') : t('common.no') },
                    { key: 'age', header: t('nodes.lifecycleControls.age'), render: (row) => formatAgeSeconds(row.age_seconds) },
                    { key: 'created', header: t('common.created'), render: (row) => fmt.date(row.created_at) },
                    { key: 'started', header: t('nodes.lifecycleControls.startedAt'), render: (row) => fmt.date(row.started_at) },
                    { key: 'claim', header: t('nodes.lifecycleControls.lastClaimAt'), render: (row) => fmt.date(row.last_claim_at) },
                    { key: 'result', header: t('nodes.lifecycleControls.lastResultAt'), render: (row) => fmt.date(row.last_result_at) },
                    { key: 'seen', header: t('nodes.lifecycleControls.lastSeenAt'), render: (row) => fmt.date(row.last_seen_at) },
                    { key: 'poll', header: t('nodes.lifecycleControls.lastPollAt'), render: (row) => fmt.date(row.last_poll_at) },
                  ]}
                />
              </>
            ) : null}
          </div>
        </CardBody>
      </Card>
    </div>
  );
}
