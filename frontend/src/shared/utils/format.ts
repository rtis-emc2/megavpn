import { useTranslation } from 'react-i18next';

export function text(value: unknown, fallback = 'n/a'): string {
  if (value == null || value === '') return fallback;
  return String(value);
}

export function shortID(value: unknown): string {
  const raw = text(value, '');
  return raw.length > 12 ? `${raw.slice(0, 8)}...` : raw || 'n/a';
}

export function useLocaleFormat() {
  const { i18n } = useTranslation();
  const locale = i18n.language || 'ru';
  const number = new Intl.NumberFormat(locale);
  const bytes = new Intl.NumberFormat(locale, { notation: 'compact', maximumFractionDigits: 1 });
  const date = new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' });

  return {
    number: (value: unknown) => {
      const n = Number(value);
      return Number.isFinite(n) ? number.format(n) : text(value);
    },
    bytes: (value: unknown) => {
      const n = Number(value);
      return Number.isFinite(n) ? bytes.format(n) : text(value);
    },
    date: (value: unknown) => {
      const raw = text(value, '');
      if (!raw) return 'n/a';
      const parsed = new Date(raw);
      return Number.isNaN(parsed.getTime()) ? raw : date.format(parsed);
    },
  };
}
