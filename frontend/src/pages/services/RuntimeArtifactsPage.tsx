import { Eye, RefreshCw, Upload } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { APIError } from '../../shared/api/client';
import type { RuntimeArtifact } from '../../shared/api/types';
import { useImportRuntimeArtifact, useRuntimeArtifacts } from '../../shared/query/hooks';
import { Badge, Button, Card, CardBody, DataTable, Drawer, FormField, FormGrid, Select, StatusBadge, TextField, Toolbar } from '../../shared/ui';
import { shortID, text, useLocaleFormat } from '../../shared/utils/format';
import { PageScaffold, QueryBoundary } from '../common';
import { ServicesTabs } from './ServicesTabs';

function formatAPIError(error: unknown): string {
  if (!(error instanceof APIError)) return error instanceof Error ? error.message : 'Request failed';
  const prefix = error.status === 403
    ? 'Permission denied'
    : error.status === 409
      ? 'Conflict'
      : error.status === 422 || error.status === 400
        ? 'Validation failed'
        : `HTTP ${error.status}`;
  return `${prefix}: ${error.message}`;
}

function safeJSON(value: unknown): string {
  try {
    return JSON.stringify(value ?? {}, null, 2);
  } catch {
    return '{}';
  }
}

export function RuntimeArtifactsPage() {
  const { t } = useTranslation();
  const fmt = useLocaleFormat();
  const artifacts = useRuntimeArtifacts();
  const importArtifact = useImportRuntimeArtifact();
  const [selected, setSelected] = useState<RuntimeArtifact | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [sourceURL, setSourceURL] = useState('');
  const [expectedSHA256, setExpectedSHA256] = useState('');
  const [name, setName] = useState('');
  const [kind, setKind] = useState('runtime_binary');
  const [serviceCode, setServiceCode] = useState('');
  const [version, setVersion] = useState('');
  const [architecture, setArchitecture] = useState('');
  const [replaceFile, setReplaceFile] = useState(false);
  const [notice, setNotice] = useState('');

  const submitImport = async () => {
    setNotice('');
    try {
      const artifact = await importArtifact.mutateAsync({
        source_url: sourceURL,
        expected_sha256: expectedSHA256,
        name,
        kind,
        service_code: serviceCode,
        version,
        architecture,
        replace_file: replaceFile,
      });
      setSelected(artifact);
      setNotice(t('runtimeArtifacts.importAccepted'));
    } catch (error) {
      setNotice(formatAPIError(error));
    }
  };

  return (
    <PageScaffold title={t('instances.runtimeArtifactsTitle')} subtitle={t('runtimeArtifacts.subtitle')} actions={<Button icon={<Upload size={16} />} onClick={() => setImportOpen(true)}>{t('runtimeArtifacts.import')}</Button>}>
      <ServicesTabs />
      <Toolbar>
        <Badge>{t('runtimeArtifacts.binaryRepository')}</Badge>
        <Badge>{t('runtimeArtifacts.deleteUnsupported')}</Badge>
        <Button icon={<RefreshCw size={16} />} onClick={() => void artifacts.refetch()}>{t('common.refresh')}</Button>
      </Toolbar>
      <QueryBoundary isLoading={artifacts.isLoading} isError={artifacts.isError} error={artifacts.error} refetch={() => void artifacts.refetch()}>
        <DataTable
          rows={artifacts.data || []}
          columns={[
            { key: 'name', header: t('common.name'), render: (row) => <strong>{text(row.name || row.id)}</strong> },
            { key: 'status', header: t('common.status'), render: (row) => <StatusBadge status={String(row.status || 'unknown')} /> },
            { key: 'service', header: t('instances.service'), render: (row) => <code>{text(row.service_code)}</code> },
            { key: 'version', header: t('servicePacks.version'), render: (row) => text(row.version) },
            { key: 'hash', header: t('instances.configHash'), render: (row) => <code>{shortID(row.sha256)}</code> },
            { key: 'size', header: t('artifacts.size'), render: (row) => fmt.bytes(row.size_bytes) },
            { key: 'created', header: t('common.created'), render: (row) => fmt.date(row.created_at) },
            { key: 'actions', header: t('common.actions'), render: (row) => <Button icon={<Eye size={16} />} onClick={() => setSelected(row)}>{t('common.open')}</Button> },
          ]}
        />
      </QueryBoundary>

      <Drawer title={t('runtimeArtifacts.import')} open={importOpen} onClose={() => setImportOpen(false)}>
        <div className="page-stack">
          <ServicesTabs />
          <Badge>{t('runtimeArtifacts.urlImportOnly')}</Badge>
          {notice ? <div role={notice.includes(':') ? 'alert' : 'status'}>{notice}</div> : null}
          <FormGrid>
            <FormField label={t('runtimeArtifacts.sourceURL')} full>
              <TextField value={sourceURL} onChange={(event) => setSourceURL(event.target.value)} />
            </FormField>
            <FormField label={t('runtimeArtifacts.expectedSHA256')} full>
              <TextField value={expectedSHA256} onChange={(event) => setExpectedSHA256(event.target.value)} />
            </FormField>
            <FormField label={t('common.name')}>
              <TextField value={name} onChange={(event) => setName(event.target.value)} />
            </FormField>
            <FormField label={t('common.type')}>
              <Select value={kind} onChange={(event) => setKind(event.target.value)}>
                <option value="runtime_binary">runtime_binary</option>
                <option value="archive">archive</option>
                <option value="installer">installer</option>
              </Select>
            </FormField>
            <FormField label={t('instances.service')}>
              <TextField value={serviceCode} onChange={(event) => setServiceCode(event.target.value)} />
            </FormField>
            <FormField label={t('servicePacks.version')}>
              <TextField value={version} onChange={(event) => setVersion(event.target.value)} />
            </FormField>
            <FormField label={t('runtimeArtifacts.architecture')}>
              <TextField value={architecture} onChange={(event) => setArchitecture(event.target.value)} />
            </FormField>
          </FormGrid>
          <label className="toolbar">
            <input type="checkbox" checked={replaceFile} onChange={(event) => setReplaceFile(event.target.checked)} />
            <span>{t('runtimeArtifacts.replaceFile')}</span>
          </label>
          <Toolbar>
            <Button variant="primary" disabled={importArtifact.isPending || !sourceURL || !expectedSHA256} onClick={() => void submitImport()}>{t('runtimeArtifacts.import')}</Button>
            <Button onClick={() => setImportOpen(false)}>{t('common.close')}</Button>
          </Toolbar>
        </div>
      </Drawer>

      <Drawer title={selected?.name || selected?.id || t('instances.runtimeArtifactsTitle')} open={Boolean(selected)} onClose={() => setSelected(null)}>
        {selected ? (
          <div className="page-stack">
            <ServicesTabs />
            <Toolbar>
              <Badge>{selected.kind || 'artifact'}</Badge>
              <StatusBadge status={selected.status} />
            </Toolbar>
            <Card>
              <CardBody>
                <div className="definition-grid">
                  <span>{t('common.id')}</span><strong>{selected.id}</strong>
                  <span>{t('instances.service')}</span><strong>{selected.service_code || 'n/a'}</strong>
                  <span>{t('servicePacks.version')}</span><strong>{selected.version || 'n/a'}</strong>
                  <span>{t('runtimeArtifacts.architecture')}</span><strong>{selected.architecture || 'n/a'}</strong>
                  <span>{t('instances.configHash')}</span><strong>{selected.sha256 || 'n/a'}</strong>
                  <span>{t('artifacts.size')}</span><strong>{fmt.bytes(selected.size_bytes)}</strong>
                  <span>{t('common.created')}</span><strong>{fmt.date(selected.created_at)}</strong>
                  <span>{t('common.updated')}</span><strong>{fmt.date(selected.updated_at)}</strong>
                </div>
              </CardBody>
            </Card>
            <Card>
              <CardBody>
                <pre className="code-block">{safeJSON({ metadata: selected.metadata || {}, storage_path: selected.storage_path || '' })}</pre>
              </CardBody>
            </Card>
            <Toolbar>
              <Button variant="danger" disabled>{t('common.delete')} - {t('runtimeArtifacts.deleteUnsupported')}</Button>
            </Toolbar>
          </div>
        ) : null}
      </Drawer>
    </PageScaffold>
  );
}
