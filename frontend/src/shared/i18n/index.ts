import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import en from './resources/en.json';
import ru from './resources/ru.json';

export const supportedLocales = ['ru', 'en'] as const;
export type SupportedLocale = (typeof supportedLocales)[number];

const storageKey = 'megavpn.locale';

function normalizeLocale(value: string | null | undefined): SupportedLocale | null {
  const normalized = String(value || '').toLowerCase().slice(0, 2);
  return supportedLocales.includes(normalized as SupportedLocale) ? (normalized as SupportedLocale) : null;
}

export function detectLocale(): SupportedLocale {
  const stored = normalizeLocale(window.localStorage.getItem(storageKey));
  if (stored) return stored;
  const browser = normalizeLocale(window.navigator.language);
  return browser || 'ru';
}

export async function setLocale(locale: SupportedLocale) {
  window.localStorage.setItem(storageKey, locale);
  await i18n.changeLanguage(locale);
  document.documentElement.lang = locale;
}

i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    ru: { translation: ru },
  },
  lng: detectLocale(),
  fallbackLng: 'ru',
  interpolation: {
    escapeValue: false,
  },
});

document.documentElement.lang = i18n.language;

export default i18n;
