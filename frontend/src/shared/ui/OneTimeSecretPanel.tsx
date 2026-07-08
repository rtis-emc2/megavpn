import { Copy, Eye, EyeOff } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from './Button';

export type OneTimeSecretPanelProps = {
  label: string;
  value: string;
  expiresAt?: string;
  onClose: () => void;
};

export function CopySecretButton({ value }: { value: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await navigator.clipboard?.writeText(value);
    setCopied(true);
  };
  return (
    <Button icon={<Copy size={16} />} onClick={() => void copy()}>
      {copied ? t('clients.delivery.copied') : t('common.copy')}
    </Button>
  );
}

export function OneTimeSecretPanel({ label, value, expiresAt, onClose }: OneTimeSecretPanelProps) {
  const { t } = useTranslation();
  const [revealed, setRevealed] = useState(false);
  return (
    <div className="inline-panel" role="status">
      <div className="page-stack">
        <strong>{label}</strong>
        <p className="muted">{t('clients.delivery.oneTimeWarning')}</p>
        {expiresAt ? <p className="muted">{t('clients.delivery.expiresAt', { value: expiresAt })}</p> : null}
        <code className="secret-value">{revealed ? value : '********************************'}</code>
        <div className="toolbar">
          <Button icon={revealed ? <EyeOff size={16} /> : <Eye size={16} />} onClick={() => setRevealed(!revealed)}>
            {revealed ? t('common.hide') : t('common.reveal')}
          </Button>
          <CopySecretButton value={value} />
          <Button onClick={onClose}>{t('common.close')}</Button>
        </div>
      </div>
    </div>
  );
}
