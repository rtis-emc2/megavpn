import { RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { NodeDetail, NodeDiagnostics, NodeStaleRotationCandidate, NodeStaleRotationPreview } from '../../shared/api/types';
import { Badge, Button, Card, CardBody, DataTable, EmptyState, LoadingSkeleton, StatusBadge, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import {
  deriveNodeLifecycleStatusModel,
  describeStaleRotationReason,
  formatAgeSeconds,
  staleRotationPreviewErrorKey,
  type NodeLifecycleSeverity,
} from './nodeLifecycleControls';

type NodeLifecycleControlsPanelProps = {
  node: NodeDetail;
  diagnostics?: NodeDiagnostics;
  staleRotationPreview?: NodeStaleRotationPreview;
  staleRotationPreviewError?: unknown;
  staleRotationPreviewLoading: boolean;
  staleRotationPreviewFetching: boolean;
  canReadNode: boolean;
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
  onRefreshStaleRotationPreview,
}: NodeLifecycleControlsPanelProps) {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const model = deriveNodeLifecycleStatusModel({ node, diagnostics, staleRotationPreview });
  const candidates = staleRotationPreview?.candidates || [];

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
