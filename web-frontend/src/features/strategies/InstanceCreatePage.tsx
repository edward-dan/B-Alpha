import { FormEvent, useMemo, useState, type ReactNode } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, CheckCircle2, ChevronRight } from "lucide-react";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import { Label } from "../../components/ui/label";
import { useI18n } from "../../i18n/useI18n";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../shared/ui/Card";
import { instancesService, ApiRequestError } from "../../shared/services";
import { findCatalogItem, strategyCatalog, type StrategyCatalogItem } from "./strategyCatalog";

export function InstanceCreatePage() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const initialTemplate = Number(searchParams.get("template")) || strategyCatalog[0].id;
  const [step, setStep] = useState<1 | 2>(initialTemplate ? 2 : 1);
  const [templateId, setTemplateId] = useState(initialTemplate);
  const [name, setName] = useState("");
  const [initialCapital, setInitialCapital] = useState("10000");
  const [monthlyInjection, setMonthlyInjection] = useState("");
  const [sealedAmount, setSealedAmount] = useState("");
  const [maxDrawdown, setMaxDrawdown] = useState("35");

  const selectedTemplate = useMemo(() => findCatalogItem(templateId), [templateId]);
  const mutation = useMutation({
    mutationFn: () =>
      instancesService.create({
        template_id: selectedTemplate.id,
        name: name.trim() || selectedTemplate.name,
        symbol: selectedTemplate.symbols[0],
        interval: "1h",
        config: {
          monthly_inject_usdt: Number(monthlyInjection || 0),
          max_available_drawdown_pct: Number(maxDrawdown) / 100,
          template_ui_id: selectedTemplate.id
        },
        initial_portfolio: {
          usdt_balance: Number(initialCapital || 0),
          dead_btc: 0,
          float_btc: 0,
          cold_sealed_btc: Number(sealedAmount || 0),
          total_equity: Number(initialCapital || 0)
        },
        initial_cost_price: 0
      }),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["instances"] });
      navigate(`/?instance=${data.instance.id}`, {
        replace: true,
        state: { notice: t("instanceCreate.createdNotice") }
      });
    }
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    mutation.mutate();
  }

  const error = mutation.error instanceof ApiRequestError ? mutation.error.message : null;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-wider text-slate-200">{t("instanceCreate.title")}</h1>
          <p className="mt-1 text-sm text-slate-500">
            {step === 1 ? t("instanceCreate.stepTemplate") : t("instanceCreate.stepConfig")}
          </p>
        </div>
        <Button variant="ghost" icon={<ArrowLeft className="h-4 w-4" />} onClick={() => navigate("/instances")}>
          {t("common.back")}
        </Button>
      </div>

      {step === 1 ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {strategyCatalog.map((template) => (
            <TemplateSelectCard
              key={template.id}
              template={template}
              selected={template.id === templateId}
              onSelect={() => setTemplateId(template.id)}
            />
          ))}
          <div className="md:col-span-2 xl:col-span-3">
            <Button variant="primary" disabled={!templateId} onClick={() => setStep(2)}>
              {t("common.next")}
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      ) : (
        <form className="grid gap-4 xl:grid-cols-[1fr_0.8fr]" onSubmit={submit}>
          <Card className="bg-slate-900/30">
            <CardHeader>
              <div>
                <CardTitle>{selectedTemplate.name}</CardTitle>
                <CardDescription>{selectedTemplate.description}</CardDescription>
              </div>
            </CardHeader>
            <CardContent className="grid gap-4 md:grid-cols-2">
              <Field label={t("instanceCreate.instanceName")}>
                <Input value={name} onChange={(event) => setName(event.target.value)} placeholder={t("instanceCreate.namePlaceholder")} />
              </Field>
              <Field label={t("common.symbol")}>
                <Input value={selectedTemplate.symbols[0]} disabled />
              </Field>
              <Field label={`${t("instanceCreate.initialCapital")} (USDT)`}>
                <Input type="number" min="0" step="0.01" value={initialCapital} onChange={(event) => setInitialCapital(event.target.value)} required />
              </Field>
              <Field label={`${t("instanceCreate.monthlyInjection")} (USDT)`}>
                <Input type="number" min="0" step="0.01" value={monthlyInjection} onChange={(event) => setMonthlyInjection(event.target.value)} />
              </Field>
              <Field label={t("instanceCreate.sealedAmount")}>
                <Input type="number" min="0" step="0.000001" value={sealedAmount} onChange={(event) => setSealedAmount(event.target.value)} />
              </Field>
              <div className="space-y-2">
                <Label className="text-xs uppercase tracking-wider text-slate-400">{t("instanceCreate.maxDrawdown")}</Label>
                <div className="rounded-lg border border-white/[0.06] bg-slate-950/35 p-3">
                  <input
                    className="w-full accent-[#2dd4bf]"
                    type="range"
                    min="5"
                    max="80"
                    step="1"
                    value={maxDrawdown}
                    onChange={(event) => setMaxDrawdown(event.target.value)}
                  />
                  <div className="mt-2 flex items-center justify-between text-xs text-slate-500">
                    <span>{t("instanceCreate.riskPreference")}</span>
                    <span className="qs-number text-slate-200">{maxDrawdown}%</span>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
          <Card className="bg-slate-900/30">
            <CardHeader>
              <div>
                <CardTitle>{t("instanceCreate.stepConfig")}</CardTitle>
                <CardDescription>{selectedTemplate.exchanges.join(" / ")}</CardDescription>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <SummaryLine label={t("templates.createInstance")} value={selectedTemplate.name} />
              <SummaryLine label={t("common.symbol")} value={selectedTemplate.symbols[0]} />
              <SummaryLine label={t("instanceCreate.initialCapital")} value={`${Number(initialCapital || 0).toFixed(2)} USDT`} />
              <SummaryLine label={t("instanceCreate.maxDrawdown")} value={`${maxDrawdown}%`} />
              {error && <p className="rounded-lg border border-red-400/20 bg-red-400/10 p-3 text-xs text-red-300">{error}</p>}
              <Button className="w-full uppercase tracking-wider" type="submit" variant="primary" disabled={mutation.isPending}>
                <CheckCircle2 className="h-4 w-4" />
                {mutation.isPending ? t("common.loading") : t("common.create")}
              </Button>
            </CardContent>
          </Card>
        </form>
      )}
    </div>
  );
}

function TemplateSelectCard({
  template,
  selected,
  onSelect
}: {
  template: StrategyCatalogItem;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      className={`rounded-xl border bg-slate-900/30 p-4 text-left transition ${
        selected ? "border-accent shadow-[0_0_24px_rgb(45_212_191/0.14)]" : "border-white/[0.04] hover:border-white/[0.12]"
      }`}
      onClick={onSelect}
    >
      <div className="h-1.5 w-16 rounded-full" style={{ backgroundColor: template.color }} />
      <h2 className="mt-4 text-sm font-semibold text-slate-200">{template.name}</h2>
      <p className="mt-2 line-clamp-2 text-sm leading-5 text-slate-500">{template.description}</p>
      <p className="mt-4 text-xs text-slate-400">{template.symbols.join(" / ")}</p>
    </button>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <Label className="text-xs uppercase tracking-wider text-slate-400">{label}</Label>
      {children}
    </div>
  );
}

function SummaryLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg bg-slate-950/35 p-3">
      <span className="text-xs text-slate-500">{label}</span>
      <span className="truncate text-sm text-slate-200">{value}</span>
    </div>
  );
}
