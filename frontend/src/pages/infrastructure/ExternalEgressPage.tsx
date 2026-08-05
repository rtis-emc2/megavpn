import { Activity, Cable, Pencil, Play, Plus, ShieldCheck, Trash2, Unplug } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type {
  ExternalEgressDeployment,
  ExternalEgressImportPreview,
  ExternalEgressProfile,
  ExternalEgressProfileInput,
  ExternalEgressProtocol,
} from '../../shared/api/types';
import { useAuth } from '../../shared/auth/AuthProvider';
import { hasPermissions } from '../../shared/permissions/permissions';
import {
  useApplyExternalEgressDeployment,
  useCleanupExternalEgressDeployment,
  useCreateExternalEgressDeployment,
  useCreateExternalEgressProfile,
  useDeleteExternalEgressDeployment,
  useDeleteExternalEgressProfile,
  useExternalEgressCatalog,
  useExternalEgressProfiles,
  useInstances,
  useNodes,
  usePreviewExternalEgressImport,
  useProbeExternalEgressDeployment,
  useUpdateExternalEgressProfile,
} from '../../shared/query/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  ConfirmDialog,
  DataTable,
  ErrorState,
  FormField,
  FormGrid,
  Modal,
  Select,
  StatusBadge,
  Textarea,
  TextField,
  Toolbar,
} from '../../shared/ui';
import { text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';

type ProfileDraft = {
  displayName: string;
  description: string;
  protocol: string;
  status: string;
  config: string;
  username: string;
  password: string;
  uuid: string;
  privateKey: string;
  presharedKey: string;
  caCertificate: string;
  certificate: string;
  server: string;
  remoteID: string;
  authMethod: 'psk' | 'certificate';
  ikeProposal: string;
  espProposal: string;
};

type DeploymentAction =
  | { kind: 'apply' | 'probe' | 'cleanup'; profile: ExternalEgressProfile; deployment: ExternalEgressDeployment }
  | { kind: 'remove'; profile: ExternalEgressProfile; deployment: ExternalEgressDeployment }
  | { kind: 'delete-profile'; profile: ExternalEgressProfile };

const defaultDraft: ProfileDraft = {
  displayName: '',
  description: '',
  protocol: 'openvpn',
  status: 'active',
  config: '',
  username: '',
  password: '',
  uuid: '',
  privateKey: '',
  presharedKey: '',
  caCertificate: '',
  certificate: '',
  server: '',
  remoteID: '',
  authMethod: 'psk',
  ikeProposal: 'aes256-sha256-modp2048,aes256-sha1-modp2048',
  espProposal: 'aes256-sha256,aes256-sha1,aes128-sha1',
};

function profileConfig(profile?: ExternalEgressProfile | null): Record<string, unknown> {
  return profile?.config_json && typeof profile.config_json === 'object' ? profile.config_json : {};
}

function draftFromProfile(profile?: ExternalEgressProfile | null): ProfileDraft {
  if (!profile) return { ...defaultDraft };
  const config = profileConfig(profile);
  const purposes = new Set(profile.secret_purposes || []);
  return {
    ...defaultDraft,
    displayName: profile.display_name || '',
    description: profile.description || '',
    protocol: profile.protocol || 'openvpn',
    status: profile.status || 'active',
    server: profile.endpoint_host || '',
    remoteID: String(config.remote_id || ''),
    authMethod: String(config.auth_method || (purposes.has('certificate') ? 'certificate' : 'psk')) === 'certificate' ? 'certificate' : 'psk',
    ikeProposal: String(config.ike_proposal || defaultDraft.ikeProposal),
    espProposal: String(config.esp_proposal || defaultDraft.espProposal),
  };
}

function resolveImportFormat(protocol: string, content: string): string {
  const trimmed = content.trim();
  if (protocol === 'openvpn') return 'ovpn';
  if (protocol === 'wireguard') return 'conf';
  if (protocol === 'l2tp_ipsec') return 'json';
  if (trimmed.startsWith('{')) return 'json';
  return 'url';
}

function l2tpContent(draft: ProfileDraft): string {
  return JSON.stringify({
    server: draft.server.trim(),
    remote_id: draft.remoteID.trim(),
    auth_method: draft.authMethod,
    ike_proposal: draft.ikeProposal.trim(),
    esp_proposal: draft.espProposal.trim(),
  });
}

function previewContent(draft: ProfileDraft): string {
  return draft.protocol === 'l2tp_ipsec' ? l2tpContent(draft) : draft.config.trim();
}

function endpointLabel(profile: ExternalEgressProfile): string {
  const endpoint = profile.endpoint_host || '';
  return profile.endpoint_port ? `${endpoint}:${profile.endpoint_port}` : endpoint || 'n/a';
}

function protocolLabel(catalog: ExternalEgressProtocol[], code: string): string {
  return catalog.find((item) => item.code === code)?.label || code;
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function ExternalEgressPage() {
  const { t } = useTranslation();
  const auth = useAuth();
  const fmt = useLocaleFormat();
  const catalog = useExternalEgressCatalog();
  const profiles = useExternalEgressProfiles();
  const nodes = useNodes();
  const instances = useInstances();
  const [selectedID, setSelectedID] = useState('');
  const [editorProfile, setEditorProfile] = useState<ExternalEgressProfile | null | undefined>(undefined);
  const [deployProfile, setDeployProfile] = useState<ExternalEgressProfile | null>(null);
  const [confirm, setConfirm] = useState<DeploymentAction | null>(null);
  const [notice, setNotice] = useState('');

  const canWrite = hasPermissions(auth.permissions, auth.roles, ['node.write', 'access_group.policy.write']);
  const availableProtocols = (catalog.data || []).filter((protocol) => protocol.runtime_support === 'ready');
  const profileRows = profiles.data || [];
  const selected = profileRows.find((profile) => profile.id === selectedID) || null;
  const activeNodes = (nodes.data || []).filter((node) => node.status !== 'retired');
  const reservedL2TPNodeIDs = new Set(
    (instances.data || [])
      .filter((instance) => instance.service_code === 'xl2tpd' && instance.status !== 'deleted' && instance.enabled !== false)
      .map((instance) => instance.node_id)
      .filter((nodeID): nodeID is string => Boolean(nodeID)),
  );

  return (
    <PageScaffold
      title={t('externalEgress.title')}
      subtitle={t('externalEgress.subtitle')}
      actions={(
        <Button
          variant="primary"
          icon={<Plus size={16} />}
          disabled={!canWrite || catalog.isLoading || !availableProtocols.length}
          onClick={() => setEditorProfile(null)}
        >
          {t('externalEgress.newProfile')}
        </Button>
      )}
    >
      {notice ? <div className="notice" role="status">{notice}</div> : null}
      {!canWrite ? <div className="notice">{t('externalEgress.readOnly')}</div> : null}

      <Card>
        <CardBody>
          <div className="external-egress-flow" aria-label={t('externalEgress.flowTitle')}>
            <div><Badge>1</Badge><strong>{t('externalEgress.flow.group')}</strong></div>
            <span aria-hidden="true">→</span>
            <div><Badge>2</Badge><strong>{t('externalEgress.flow.profile')}</strong></div>
            <span aria-hidden="true">→</span>
            <div><Badge>3</Badge><strong>{t('externalEgress.flow.runtime')}</strong></div>
            <span aria-hidden="true">→</span>
            <div><Badge>4</Badge><strong>{t('externalEgress.flow.provider')}</strong></div>
          </div>
        </CardBody>
      </Card>

      <QueryBoundary
        isLoading={catalog.isLoading || profiles.isLoading || nodes.isLoading || instances.isLoading}
        isError={catalog.isError || profiles.isError || nodes.isError || instances.isError}
        error={catalog.error || profiles.error || nodes.error || instances.error}
        refetch={() => {
          void catalog.refetch();
          void profiles.refetch();
          void nodes.refetch();
          void instances.refetch();
        }}
      >
        <DataTable
          title={t('externalEgress.profiles')}
          rows={profileRows}
          columns={[
            {
              key: 'profile',
              header: t('externalEgress.profile'),
              render: (profile) => (
                <div>
                  <strong>{text(profile.display_name)}</strong>
                  <div className="muted">{text(profile.description)}</div>
                </div>
              ),
            },
            { key: 'protocol', header: t('externalEgress.protocol'), render: (profile) => protocolLabel(catalog.data || [], profile.protocol) },
            { key: 'status', header: t('common.status'), render: (profile) => <StatusBadge status={profile.status} /> },
            { key: 'endpoint', header: t('externalEgress.endpoint'), render: (profile) => <code>{endpointLabel(profile)}</code> },
            { key: 'deployments', header: t('externalEgress.deployments'), render: (profile) => profile.deployments?.filter((item) => item.status !== 'deleted').length || 0 },
            { key: 'updated', header: t('common.updated'), render: (profile) => fmt.date(profile.updated_at) },
            {
              key: 'actions',
              header: t('common.actions'),
              render: (profile) => (
                <Toolbar>
                  <Button icon={<Cable size={16} />} onClick={() => setSelectedID(profile.id)}>{t('common.open')}</Button>
                  <Button icon={<Pencil size={16} />} disabled={!canWrite} onClick={() => setEditorProfile(profile)}>{t('common.edit')}</Button>
                  <Button icon={<Plus size={16} />} disabled={!canWrite || profile.status !== 'active'} onClick={() => setDeployProfile(profile)}>
                    {t('externalEgress.deploy')}
                  </Button>
                </Toolbar>
              ),
            },
          ]}
        />

        {selected ? (
          <ProfileDetail
            profile={selected}
            canWrite={canWrite}
            onDeploy={() => setDeployProfile(selected)}
            onAction={setConfirm}
          />
        ) : null}
      </QueryBoundary>

      <ProfileEditor
        key={editorProfile === undefined ? 'closed' : editorProfile?.id || 'new'}
        profile={editorProfile}
        catalog={availableProtocols}
        onClose={() => setEditorProfile(undefined)}
        onSaved={(profile) => {
          setSelectedID(profile.id);
          setNotice(t('externalEgress.saved'));
          setEditorProfile(undefined);
        }}
      />
      <DeployDialog
        profile={deployProfile}
        profiles={profileRows}
        nodes={activeNodes}
        reservedL2TPNodeIDs={reservedL2TPNodeIDs}
        onClose={() => setDeployProfile(null)}
        onQueued={(profile) => {
          setSelectedID(profile.id);
          setNotice(t('externalEgress.applyQueued'));
          setDeployProfile(null);
        }}
      />
      <ActionDialog
        action={confirm}
        onClose={() => setConfirm(null)}
        onDone={(message) => {
          setNotice(message);
          setConfirm(null);
        }}
      />
    </PageScaffold>
  );
}

function ProfileDetail({ profile, canWrite, onDeploy, onAction }: {
  profile: ExternalEgressProfile;
  canWrite: boolean;
  onDeploy: () => void;
  onAction: (action: DeploymentAction) => void;
}) {
  const { t } = useTranslation();
  const deployments = (profile.deployments || []).filter((item) => item.status !== 'deleted');
  return (
    <div className="page-stack">
      <Card>
        <CardBody>
          <div className="page-header">
            <div>
              <h2 className="card-title">{profile.display_name}</h2>
              <p>{profile.description || t('common.none')}</p>
            </div>
            <Toolbar>
              {(profile.secret_purposes || []).map((purpose) => <Badge key={purpose}>{purpose}</Badge>)}
              <Button variant="primary" icon={<Plus size={16} />} disabled={!canWrite || profile.status !== 'active'} onClick={onDeploy}>
                {t('externalEgress.deploy')}
              </Button>
              <Button variant="danger" icon={<Trash2 size={16} />} disabled={!canWrite} onClick={() => onAction({ kind: 'delete-profile', profile })}>
                {t('common.delete')}
              </Button>
            </Toolbar>
          </div>
        </CardBody>
      </Card>
      <DataTable
        title={t('externalEgress.deployments')}
        rows={deployments}
        columns={[
          {
            key: 'node',
            header: t('externalEgress.node'),
            render: (deployment) => (
              <div>
                <strong>{text(deployment.node_name || deployment.node_id)}</strong>
                <div className="muted"><code>{text(deployment.interface_name)}</code></div>
              </div>
            ),
          },
          { key: 'status', header: t('common.status'), render: (deployment) => <StatusBadge status={deployment.status} /> },
          { key: 'route', header: t('externalEgress.routing'), render: (deployment) => <code>{text(deployment.routing_table)}</code> },
          { key: 'health', header: t('externalEgress.health'), render: (deployment) => deployment.last_error ? <span className="error-state-inline">{deployment.last_error}</span> : <StatusBadge status={deployment.status === 'active' ? 'healthy' : deployment.status} /> },
          {
            key: 'actions',
            header: t('common.actions'),
            render: (deployment) => (
              <Toolbar>
                <Button icon={<Activity size={16} />} disabled={!canWrite} onClick={() => onAction({ kind: 'probe', profile, deployment })}>{t('externalEgress.probe')}</Button>
                <Button variant="primary" icon={<Play size={16} />} disabled={!canWrite} onClick={() => onAction({ kind: 'apply', profile, deployment })}>{t('common.apply')}</Button>
                <Button icon={<Unplug size={16} />} disabled={!canWrite} onClick={() => onAction({ kind: 'cleanup', profile, deployment })}>{t('externalEgress.cleanup')}</Button>
                <Button variant="danger" icon={<Trash2 size={16} />} disabled={!canWrite || deployment.status === 'active'} onClick={() => onAction({ kind: 'remove', profile, deployment })}>{t('externalEgress.remove')}</Button>
              </Toolbar>
            ),
          },
        ]}
      />
    </div>
  );
}

function ProfileEditor({ profile, catalog, onClose, onSaved }: {
  profile: ExternalEgressProfile | null | undefined;
  catalog: ExternalEgressProtocol[];
  onClose: () => void;
  onSaved: (profile: ExternalEgressProfile) => void;
}) {
  const { t } = useTranslation();
  const open = profile !== undefined;
  const editing = Boolean(profile);
  const create = useCreateExternalEgressProfile();
  const update = useUpdateExternalEgressProfile();
  const preview = usePreviewExternalEgressImport();
  const [draft, setDraft] = useState<ProfileDraft>(() => {
    const initial = draftFromProfile(profile);
    if (!profile && catalog.length && !catalog.some((item) => item.code === initial.protocol)) {
      return { ...initial, protocol: catalog[0].code };
    }
    return initial;
  });
  const [validated, setValidated] = useState<{ fingerprint: string; result: ExternalEgressImportPreview } | null>(null);

  if (!open) return null;
  const set = <K extends keyof ProfileDraft>(key: K, value: ProfileDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
    setValidated(null);
  };
  const content = previewContent(draft);
  const fingerprint = `${draft.protocol}\n${content}`;
  const currentPreview = validated?.fingerprint === fingerprint ? validated.result : null;
  const busy = create.isPending || update.isPending || preview.isPending;
  const error = create.error || update.error || preview.error;

  const validate = async () => {
    const result = await preview.mutateAsync({
      protocol: draft.protocol,
      format: resolveImportFormat(draft.protocol, content),
      content,
    });
    setValidated({ fingerprint, result });
  };

  const save = async () => {
    const requiresPreview = !editing || Boolean(content);
    if (requiresPreview && !currentPreview) return;
    const secrets: Record<string, string> = {};
    if (content) secrets.config = content;
    const values: Record<string, string> = {
      username: draft.username,
      password: draft.password,
      uuid: draft.uuid,
      private_key: draft.privateKey,
      preshared_key: draft.presharedKey,
      ca_certificate: draft.caCertificate,
      certificate: draft.certificate,
    };
    Object.entries(values).forEach(([key, value]) => {
      if (value.trim()) secrets[key] = value.trim();
    });
    const existing = profileConfig(profile);
    const input: ExternalEgressProfileInput = {
      display_name: draft.displayName.trim(),
      description: draft.description.trim(),
      protocol: draft.protocol,
      status: draft.status,
      endpoint_host: currentPreview?.endpoint_host || profile?.endpoint_host || '',
      endpoint_port: currentPreview?.endpoint_port || profile?.endpoint_port || 0,
      transport: currentPreview?.transport || profile?.transport || '',
      import_format: currentPreview?.import_format || profile?.import_format || resolveImportFormat(draft.protocol, content),
      config_json: draft.protocol === 'l2tp_ipsec'
        ? {
            auth_method: draft.authMethod,
            remote_id: draft.remoteID.trim(),
            ike_proposal: draft.ikeProposal.trim(),
            esp_proposal: draft.espProposal.trim(),
          }
        : existing,
      secrets,
    };
    const result = editing && profile
      ? await update.mutateAsync({ profileId: profile.id, input })
      : await create.mutateAsync(input);
    onSaved(result);
  };

  return (
    <Modal size="wide" title={editing ? t('externalEgress.editProfile') : t('externalEgress.newProfile')} open={open} onClose={onClose}>
      <div className="external-egress-editor">
        <section className="form-section" aria-labelledby="external-egress-profile-details">
          <h3 id="external-egress-profile-details" className="form-section-title">{t('externalEgress.profileDetails')}</h3>
          <FormGrid>
            <FormField label={t('common.name')}>
              <TextField required value={draft.displayName} onChange={(event) => set('displayName', event.currentTarget.value)} />
            </FormField>
            <FormField label={t('externalEgress.protocol')}>
              {editing ? (
                <TextField disabled value={protocolLabel(catalog, draft.protocol)} />
              ) : (
                <Select value={draft.protocol} onChange={(event) => set('protocol', event.currentTarget.value)}>
                  {catalog.map((item) => <option key={item.code} value={item.code}>{item.label}</option>)}
                </Select>
              )}
            </FormField>
            <FormField label={t('common.status')}>
              <Select value={draft.status} onChange={(event) => set('status', event.currentTarget.value)}>
                <option value="active">{t('common.enabled')}</option>
                <option value="draft">{t('externalEgress.draft')}</option>
                <option value="disabled">{t('common.disabled')}</option>
              </Select>
            </FormField>
            <FormField full label={t('common.description')}>
              <Textarea rows={2} value={draft.description} onChange={(event) => set('description', event.currentTarget.value)} />
            </FormField>
          </FormGrid>
        </section>

        {draft.protocol === 'l2tp_ipsec' ? (
          <L2TPFields draft={draft} set={set} editing={editing} />
        ) : (
          <ImportedProfileFields draft={draft} set={set} editing={editing} />
        )}

        {currentPreview ? (
          <section className="validation-summary" aria-live="polite">
            <Toolbar><ShieldCheck size={18} /><strong>{t('externalEgress.validated')}</strong></Toolbar>
            <div><code>{currentPreview.endpoint_host}:{currentPreview.endpoint_port || ''}</code> · {currentPreview.transport}</div>
            {currentPreview.required_secrets?.length ? <div>{t('externalEgress.requiredSecrets')}: {currentPreview.required_secrets.join(', ')}</div> : null}
            {currentPreview.warnings?.map((warning) => <div className="muted" key={warning}>{warning}</div>)}
          </section>
        ) : null}
        {error ? <ErrorState body={errorText(error)} /> : null}
        <div className="modal-actions">
          <Button onClick={onClose}>{t('common.cancel')}</Button>
          <Button disabled={busy || !content} onClick={() => void validate()}>{preview.isPending ? t('common.loading') : t('externalEgress.validate')}</Button>
          <Button
            variant="primary"
            disabled={busy || !draft.displayName.trim() || ((!editing || Boolean(content)) && !currentPreview)}
            onClick={() => void save()}
          >
            {create.isPending || update.isPending ? t('common.loading') : t('common.save')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function ImportedProfileFields({ draft, set, editing }: {
  draft: ProfileDraft;
  set: <K extends keyof ProfileDraft>(key: K, value: ProfileDraft[K]) => void;
  editing: boolean;
}) {
  const { t } = useTranslation();
  return (
    <section className="form-section" aria-labelledby="external-egress-provider-config">
      <h3 id="external-egress-provider-config" className="form-section-title">{t('externalEgress.connectionDetails')}</h3>
      <FormField label={t('externalEgress.providerConfig')} full>
        <Textarea
          rows={8}
          value={draft.config}
          placeholder={editing ? t('externalEgress.keepConfig') : t('externalEgress.configPlaceholder')}
          onChange={(event) => set('config', event.currentTarget.value)}
        />
      </FormField>
      <FormGrid>
        {draft.protocol === 'openvpn' ? (
          <>
            <FormField label={t('externalEgress.username')}><TextField autoComplete="off" value={draft.username} onChange={(event) => set('username', event.currentTarget.value)} /></FormField>
            <FormField label={t('externalEgress.password')}><TextField type="password" autoComplete="new-password" value={draft.password} onChange={(event) => set('password', event.currentTarget.value)} /></FormField>
          </>
        ) : null}
        {draft.protocol === 'vless' ? <FormField label={t('externalEgress.uuid')}><TextField value={draft.uuid} onChange={(event) => set('uuid', event.currentTarget.value)} /></FormField> : null}
        {draft.protocol === 'shadowsocks' ? <FormField label={t('externalEgress.password')}><TextField type="password" value={draft.password} onChange={(event) => set('password', event.currentTarget.value)} /></FormField> : null}
        {draft.protocol === 'wireguard' ? (
          <>
            <FormField label={t('externalEgress.privateKey')}><Textarea rows={3} value={draft.privateKey} onChange={(event) => set('privateKey', event.currentTarget.value)} /></FormField>
            <FormField label={t('externalEgress.presharedKey')}><Textarea rows={3} value={draft.presharedKey} onChange={(event) => set('presharedKey', event.currentTarget.value)} /></FormField>
          </>
        ) : null}
      </FormGrid>
    </section>
  );
}

function L2TPFields({ draft, set, editing }: {
  draft: ProfileDraft;
  set: <K extends keyof ProfileDraft>(key: K, value: ProfileDraft[K]) => void;
  editing: boolean;
}) {
  const { t } = useTranslation();
  return (
    <section className="form-section" aria-labelledby="external-egress-l2tp-connection">
      <h3 id="external-egress-l2tp-connection" className="form-section-title">{t('externalEgress.l2tpConnection')}</h3>
      <FormGrid>
            <FormField label={t('externalEgress.server')}>
              <TextField required value={draft.server} placeholder="vpn.provider.example" onChange={(event) => set('server', event.currentTarget.value)} />
            </FormField>
            <FormField label={t('externalEgress.remoteID')}>
              <TextField value={draft.remoteID} onChange={(event) => set('remoteID', event.currentTarget.value)} />
            </FormField>
            <FormField label={t('externalEgress.username')}>
              <TextField required={!editing} autoComplete="off" value={draft.username} onChange={(event) => set('username', event.currentTarget.value)} />
            </FormField>
            <FormField label={t('externalEgress.password')}>
              <TextField required={!editing} type="password" autoComplete="new-password" value={draft.password} onChange={(event) => set('password', event.currentTarget.value)} />
            </FormField>
            <FormField label={t('externalEgress.ipsecAuth')}>
              <Select value={draft.authMethod} onChange={(event) => set('authMethod', event.currentTarget.value === 'certificate' ? 'certificate' : 'psk')}>
                <option value="psk">{t('externalEgress.pskAuth')}</option>
                <option value="certificate">{t('externalEgress.certificateAuth')}</option>
              </Select>
            </FormField>
            {draft.authMethod === 'psk' ? (
              <FormField label={t('externalEgress.presharedKey')}>
                <TextField required={!editing} type="password" value={draft.presharedKey} onChange={(event) => set('presharedKey', event.currentTarget.value)} />
              </FormField>
            ) : (
              <>
                <FormField label={t('externalEgress.caCertificate')}><Textarea required={!editing} rows={4} value={draft.caCertificate} onChange={(event) => set('caCertificate', event.currentTarget.value)} /></FormField>
                <FormField label={t('externalEgress.certificate')}><Textarea required={!editing} rows={4} value={draft.certificate} onChange={(event) => set('certificate', event.currentTarget.value)} /></FormField>
                <FormField full label={t('externalEgress.privateKey')}><Textarea required={!editing} rows={4} value={draft.privateKey} onChange={(event) => set('privateKey', event.currentTarget.value)} /></FormField>
              </>
            )}
            <FormField label={t('externalEgress.ikeProposal')}><TextField value={draft.ikeProposal} onChange={(event) => set('ikeProposal', event.currentTarget.value)} /></FormField>
            <FormField label={t('externalEgress.espProposal')}><TextField value={draft.espProposal} onChange={(event) => set('espProposal', event.currentTarget.value)} /></FormField>
      </FormGrid>
    </section>
  );
}

function DeployDialog({ profile, profiles, nodes, reservedL2TPNodeIDs, onClose, onQueued }: {
  profile: ExternalEgressProfile | null;
  profiles: ExternalEgressProfile[];
  nodes: { id: string; name?: string; role?: string }[];
  reservedL2TPNodeIDs: Set<string>;
  onClose: () => void;
  onQueued: (profile: ExternalEgressProfile) => void;
}) {
  const { t } = useTranslation();
  const create = useCreateExternalEgressDeployment();
  const apply = useApplyExternalEgressDeployment();
  const [nodeID, setNodeID] = useState('');
  const [metric, setMetric] = useState(100);
  if (!profile) return null;

  const occupied = new Set(
    profiles.flatMap((item) => (item.deployments || [])
      .filter((deployment) => deployment.status !== 'deleted' && (item.id === profile.id || profile.protocol === 'l2tp_ipsec' && item.protocol === 'l2tp_ipsec'))
      .map((deployment) => deployment.node_id)),
  );
  if (profile.protocol === 'l2tp_ipsec') {
    reservedL2TPNodeIDs.forEach((nodeID) => occupied.add(nodeID));
  }
  const available = nodes.filter((node) => !occupied.has(node.id));
  const run = async () => {
    const deployment = await create.mutateAsync({
      profileId: profile.id,
      input: { node_id: nodeID, desired_status: 'active', routing_table: 'auto', route_metric: metric, config_json: {} },
    });
    await apply.mutateAsync({ deploymentId: deployment.id, profileId: profile.id });
    onQueued(profile);
  };
  const error = create.error || apply.error;
  return (
    <Modal title={`${t('externalEgress.deploy')}: ${profile.display_name}`} open={Boolean(profile)} onClose={onClose}>
      <div className="page-stack">
        <p>{t('externalEgress.deployHint')}</p>
        <FormGrid>
          <FormField label={t('externalEgress.node')}>
            <Select value={nodeID} onChange={(event) => setNodeID(event.currentTarget.value)}>
              <option value="">{t('common.select')}</option>
              {available.map((node) => <option key={node.id} value={node.id}>{node.name || node.id} · {node.role || 'node'}</option>)}
            </Select>
          </FormField>
          <FormField label={t('externalEgress.routeMetric')}>
            <TextField type="number" min={1} max={32767} value={metric} onChange={(event) => setMetric(Number(event.currentTarget.value || 100))} />
          </FormField>
        </FormGrid>
        {!available.length ? <div className="notice">{t('externalEgress.noAvailableNodes')}</div> : null}
        {error ? <ErrorState body={errorText(error)} /> : null}
        <div className="modal-actions">
          <Button onClick={onClose}>{t('common.cancel')}</Button>
          <Button variant="primary" disabled={!nodeID || create.isPending || apply.isPending} onClick={() => void run()}>
            {create.isPending || apply.isPending ? t('common.loading') : t('externalEgress.deployAndApply')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function ActionDialog({ action, onClose, onDone }: {
  action: DeploymentAction | null;
  onClose: () => void;
  onDone: (message: string) => void;
}) {
  const { t } = useTranslation();
  const apply = useApplyExternalEgressDeployment();
  const probe = useProbeExternalEgressDeployment();
  const cleanup = useCleanupExternalEgressDeployment();
  const remove = useDeleteExternalEgressDeployment();
  const deleteProfile = useDeleteExternalEgressProfile();
  if (!action) return null;

  const busy = apply.isPending || probe.isPending || cleanup.isPending || remove.isPending || deleteProfile.isPending;
  const error = apply.error || probe.error || cleanup.error || remove.error || deleteProfile.error;
  const run = async () => {
    if (action.kind === 'delete-profile') {
      await deleteProfile.mutateAsync(action.profile.id);
      onDone(t('externalEgress.profileDeleted'));
      return;
    }
    if (action.kind === 'remove') {
      await remove.mutateAsync(action.deployment.id);
      onDone(t('externalEgress.deploymentRemoved'));
      return;
    }
    const input = { deploymentId: action.deployment.id, profileId: action.profile.id };
    if (action.kind === 'apply') await apply.mutateAsync(input);
    if (action.kind === 'probe') await probe.mutateAsync(input);
    if (action.kind === 'cleanup') await cleanup.mutateAsync(input);
    onDone(t('externalEgress.jobQueued'));
  };
  const destructive = action.kind === 'cleanup' || action.kind === 'remove' || action.kind === 'delete-profile';
  return (
    <ConfirmDialog title={t(`externalEgress.action.${action.kind}`)} open={Boolean(action)} onClose={onClose}>
      <div className="page-stack">
        <p>{t(`externalEgress.confirm.${action.kind}`)}</p>
        {error ? <ErrorState body={errorText(error)} /> : null}
        <div className="modal-actions">
          <Button onClick={onClose}>{t('common.cancel')}</Button>
          <Button variant={destructive ? 'danger' : 'primary'} disabled={busy} onClick={() => void run()}>
            {busy ? t('common.loading') : t('clients.core.confirm')}
          </Button>
        </div>
      </div>
    </ConfirmDialog>
  );
}
