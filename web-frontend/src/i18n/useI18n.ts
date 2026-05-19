import { createContext, createElement, useContext, type ReactNode } from "react";
import en from "./locales/en.json";
import zh from "./locales/zh.json";
import { useLocaleStore } from "../stores/localeStore";
import type { Locale } from "../stores/localeStore";

const dictionaries = {
  zh,
  en
} as const;

type Dict = Record<string, string>;
type Values = Record<string, string | number | null | undefined>;

type I18nContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: string, values?: Values) => string;
};

const I18nContext = createContext<I18nContextValue | null>(null);

function interpolate(template: string, values?: Values) {
  if (!values) return template;
  return template.replace(/\{(\w+)\}/g, (_, key: string) => {
    const value = values[key];
    return value == null ? "" : String(value);
  });
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const locale = useLocaleStore((state) => state.locale);
  const setLocale = useLocaleStore((state) => state.setLocale);
  const dictionary = (dictionaries[locale] ?? dictionaries.zh) as Dict;
  const fallback = dictionaries.zh as Dict;

  function t(key: string, values?: Values) {
    return interpolate(dictionary[key] ?? fallback[key] ?? key, values);
  }

  return createElement(I18nContext.Provider, { value: { locale, setLocale, t } }, children);
}

export function useI18n() {
  const value = useContext(I18nContext);
  if (value) return value;

  const locale = useLocaleStore.getState().locale;
  const setLocale = useLocaleStore.getState().setLocale;
  const dictionary = (dictionaries[locale] ?? dictionaries.zh) as Dict;
  const fallback = dictionaries.zh as Dict;
  return {
    locale,
    setLocale,
    t: (key: string, values?: Values) => interpolate(dictionary[key] ?? fallback[key] ?? key, values)
  };
}
