import type { ReactNode } from "react";
import { Activity } from "lucide-react";
import { AppBackground } from "../../shared/ui/AppBackground";
import { useI18n } from "../../i18n/useI18n";

type AuthScaffoldProps = {
  title: string;
  subtitle?: string;
  children: ReactNode;
};

export function AuthScaffold({ title, subtitle, children }: AuthScaffoldProps) {
  const { t } = useI18n();
  return (
    <AppBackground>
      <main className="flex min-h-screen items-center justify-center px-4 py-10">
        <section className="w-full max-w-[400px] rounded-xl border border-white/10 bg-slate-900/60 p-6 shadow-[0_24px_90px_rgb(2_6_23/0.45)] backdrop-blur-xl">
          <div className="mb-8 text-center">
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-lg border border-[#ff8c6b]/25 bg-[#ff8c6b]/10 text-[#ff8c6b]">
              <Activity className="h-6 w-6" />
            </div>
            <h1 className="text-2xl font-semibold tracking-wider text-slate-200">{t("app.name")}</h1>
            <p className="mt-2 text-xs leading-5 tracking-wider text-slate-400">{subtitle ?? t("app.slogan")}</p>
          </div>
          <h2 className="mb-6 text-center text-sm font-semibold uppercase tracking-wider text-slate-300">{title}</h2>
          {children}
        </section>
      </main>
    </AppBackground>
  );
}
