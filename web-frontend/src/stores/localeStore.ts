import { create } from "zustand";
import { persist } from "zustand/middleware";

export type Locale = "zh" | "en";

type LocaleStore = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
};

function normalizeLocale(value: unknown): Locale {
  if (value === "en") return "en";
  return "zh";
}

export const useLocaleStore = create<LocaleStore>()(
  persist(
    (set) => ({
      locale: "zh",
      setLocale: (locale) => set({ locale: normalizeLocale(locale) })
    }),
    {
      name: "b-alpha-locale",
      merge: (persisted, current) => {
        const state = persisted as Partial<LocaleStore> | undefined;
        return { ...current, locale: normalizeLocale(state?.locale) };
      }
    }
  )
);
