import { Link } from "react-router-dom";
import { Cpu, Sparkles } from "lucide-react";
import { useI18n } from "../../i18n/useI18n";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../shared/ui/Card";
import { strategyCatalog } from "./strategyCatalog";

export function TemplatesPage() {
  const { t } = useI18n();

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-xl font-semibold tracking-wider text-slate-200">{t("templates.title")}</h1>
        <p className="mt-1 text-sm text-slate-500">{t("templates.description")}</p>
      </div>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {strategyCatalog.map((template) => (
          <Card key={template.id} className="overflow-hidden bg-slate-900/30">
            <div className="h-1.5" style={{ backgroundColor: template.color }} />
            <CardHeader>
              <div className="flex min-w-0 items-start gap-3">
                <div
                  className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border bg-slate-950/40"
                  style={{ borderColor: `${template.color}40`, color: template.color }}
                >
                  <Cpu className="h-5 w-5" />
                </div>
                <div className="min-w-0">
                  <CardTitle className="truncate">{template.name}</CardTitle>
                  <CardDescription>{template.symbols.join(" / ")}</CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-5">
              <p className="line-clamp-2 min-h-10 text-sm leading-5 text-slate-400">{template.description}</p>
              <div className="grid gap-2 text-xs text-slate-500">
                <div className="flex items-center justify-between gap-3 rounded-lg bg-slate-950/35 p-3">
                  <span>{t("common.exchange")}</span>
                  <span className="text-slate-300">{template.exchanges.join(" / ")}</span>
                </div>
                <div className="flex items-center justify-between gap-3 rounded-lg bg-slate-950/35 p-3">
                  <span>{t("common.symbol")}</span>
                  <span className="text-slate-300">{template.symbols.join(" / ")}</span>
                </div>
              </div>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <span
                  className="inline-flex h-7 items-center gap-2 rounded-full border px-2.5 text-xs font-medium"
                  style={{ borderColor: `${template.color}40`, backgroundColor: `${template.color}1A`, color: template.color }}
                >
                  <Sparkles className="h-3.5 w-3.5" />
                  {template.features.evolution ? t("templates.evolutionReady") : t("templates.evolutionOff")}
                </span>
                <Link
                  to={`/instances/new?template=${template.id}`}
                  className="inline-flex h-9 items-center rounded-md bg-accent px-3 text-xs font-semibold uppercase tracking-wider text-slate-950 hover:bg-accent/90"
                >
                  {t("templates.createInstance")}
                </Link>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
