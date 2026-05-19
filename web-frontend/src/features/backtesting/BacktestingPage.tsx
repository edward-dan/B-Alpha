import { FormEvent, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis
} from "recharts";
import { Play } from "lucide-react";
import { Button } from "../../components/ui/button";
import { Label } from "../../components/ui/label";
import { useI18n } from "../../i18n/useI18n";
import { formatCurrency, formatPercent, toNumber } from "../../lib/format";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../shared/ui/Card";
import { StatusBadge } from "../../shared/ui/StatusBadge";
import { PnLChartSkeleton } from "../../shared/ui/skeletons";
import { backtestsService, evolutionService, instancesService, ApiRequestError } from "../../shared/services";
import type { BacktestRun, GeneRecord, StrategyInstance } from "../../types/api";
import { catalogItemForInstance } from "../strategies/strategyCatalog";

type SourceMode = "champion" | "candidate" | "custom";

export function BacktestingPage() {
  const { t } = useI18n();
  const [searchParams] = useSearchParams();
  const [sourceMode, setSourceMode] = useState<SourceMode>(searchParams.get("genome") ? "candidate" : "champion");
  const [selectedInstanceId, setSelectedInstanceId] = useState<number | null>(null);
  const [selectedGeneId, setSelectedGeneId] = useState(searchParams.get("genome") ?? "");
  const [customJson, setCustomJson] = useState("");
  const [runId, setRunId] = useState<number | null>(null);

  const instancesQuery = useQuery({ queryKey: ["instances"], queryFn: instancesService.list, refetchInterval: 60_000 });
  const genomesQuery = useQuery({ queryKey: ["evolution-genomes"], queryFn: () => evolutionService.genomes(), refetchInterval: 30_000 });
  const instances = instancesQuery.data?.instances ?? [];
  const genomes = genomesQuery.data?.genomes ?? [];
  const championBySymbol = new Map(genomes.filter((gene) => gene.role === "champion").map((gene) => [gene.symbol, gene]));
  const eligibleInstances = instances.filter((instance) => championBySymbol.has(instance.symbol));
  const selectedInstance = eligibleInstances.find((item) => item.id === selectedInstanceId) ?? eligibleInstances[0] ?? null;
  const selectedChampion = selectedInstance ? championBySymbol.get(selectedInstance.symbol) ?? null : null;
  const candidates = genomes.filter((gene) => gene.role === "challenger" && (!selectedInstance || gene.symbol === selectedInstance.symbol));

  const runQuery = useQuery({
    queryKey: ["backtest-run", runId],
    queryFn: () => backtestsService.get(runId ?? 0),
    enabled: Boolean(runId),
    refetchInterval: (query) => {
      const status = (query.state.data as { backtest?: BacktestRun } | undefined)?.backtest?.status;
      return status === "running" ? 3_000 : false;
    }
  });

  const createRun = useMutation({
    mutationFn: () => {
      const catalog = catalogItemForInstance(selectedInstance);
      const parsedPack = customJson.trim() ? JSON.parse(customJson) : undefined;
      const geneID =
        sourceMode === "champion"
          ? selectedChampion?.id
          : sourceMode === "candidate" && selectedGeneId
            ? Number(selectedGeneId)
            : undefined;
      return backtestsService.create({
        strategy_id: catalog.strategyId,
        symbol: selectedInstance?.symbol ?? catalog.symbols[0],
        interval: selectedInstance?.interval ?? "1h",
        gene_id: geneID,
        param_pack: sourceMode === "custom" ? parsedPack : undefined,
        limit: 20000
      });
    },
    onSuccess: (data) => {
      setRunId(data.backtest.id);
    }
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    createRun.mutate();
  }

  const activeRun = runQuery.data?.backtest ?? createRun.data?.backtest ?? null;
  const result = activeRun?.result as Record<string, unknown> | undefined;
  const metrics = (result?.metrics ?? {}) as Record<string, unknown>;
  const error = createRun.error instanceof ApiRequestError ? createRun.error.message : createRun.error ? t("common.invalidJson") : null;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold tracking-wider text-slate-200">{t("backtesting.title")}</h1>
        <label className="flex items-center gap-2 text-xs text-slate-500">
          {t("backtesting.instanceSelector")}
          <select
            className="h-10 min-w-64 rounded-md border border-white/[0.06] bg-slate-950/70 px-3 text-sm text-slate-200 outline-none focus:border-accent"
            value={selectedInstance?.id ?? ""}
            onChange={(event) => setSelectedInstanceId(Number(event.target.value))}
          >
            {eligibleInstances.map((instance) => (
              <option key={instance.id} value={instance.id}>
                {instance.name} / {instance.symbol}
              </option>
            ))}
          </select>
        </label>
      </div>

      <Card className="bg-slate-900/30">
        <CardHeader>
          <div>
            <CardTitle>{t("backtesting.start")}</CardTitle>
            <CardDescription>{selectedInstance?.symbol ?? t("common.empty")}</CardDescription>
          </div>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4 lg:grid-cols-[1fr_auto]" onSubmit={submit}>
            <div className="space-y-3">
              <Label className="text-xs uppercase tracking-wider text-slate-400">{t("backtesting.source")}</Label>
              <div className="flex flex-wrap gap-2">
                {[
                  ["champion", t("backtesting.sourceChampion")],
                  ["candidate", t("backtesting.sourceCandidate")],
                  ["custom", t("backtesting.sourceCustom")]
                ].map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    className={`h-9 rounded-md border px-3 text-xs font-medium ${
                      sourceMode === value ? "border-accent bg-accent/10 text-accent" : "border-white/[0.06] text-slate-500"
                    }`}
                    onClick={() => setSourceMode(value as SourceMode)}
                  >
                    {label}
                  </button>
                ))}
              </div>
              {sourceMode === "candidate" && (
                <select
                  className="h-10 w-full rounded-md border border-white/[0.06] bg-slate-950/70 px-3 text-sm text-slate-200 outline-none focus:border-accent"
                  value={selectedGeneId}
                  onChange={(event) => setSelectedGeneId(event.target.value)}
                >
                  <option value="">{t("common.candidate")}</option>
                  {candidates.map((gene) => (
                    <option key={gene.id} value={gene.id}>
                      #{gene.id} / {Number(gene.score_total ?? 0).toFixed(4)}
                    </option>
                  ))}
                </select>
              )}
              {sourceMode === "custom" && (
                <textarea
                  className="min-h-32 w-full rounded-md border border-slate-700 bg-slate-900/80 p-3 font-mono text-xs text-slate-100 outline-none focus:border-accent"
                  value={customJson}
                  onChange={(event) => setCustomJson(event.target.value)}
                  placeholder='{"chromosome":{},"spawn_point":{}}'
                />
              )}
              {error && <p className="rounded-lg border border-red-400/20 bg-red-400/10 p-3 text-xs text-red-300">{error}</p>}
            </div>
            <Button
              className="self-end uppercase tracking-wider"
              type="submit"
              variant="primary"
              disabled={
                createRun.isPending ||
                !selectedInstance ||
                (sourceMode === "champion" && !selectedChampion) ||
                (sourceMode === "candidate" && !selectedGeneId)
              }
            >
              <Play className="h-4 w-4" />
              {createRun.isPending ? t("common.loading") : t("backtesting.start")}
            </Button>
          </form>
        </CardContent>
      </Card>

      <BacktestResult run={activeRun} metrics={metrics} result={result} loading={Boolean(runId && runQuery.isLoading)} />
    </div>
  );
}

function BacktestResult({
  run,
  metrics,
  result,
  loading
}: {
  run: BacktestRun | null;
  metrics: Record<string, unknown>;
  result?: Record<string, unknown>;
  loading: boolean;
}) {
  const { t } = useI18n();
  const chartData = useBacktestChartData(result, metrics);

  if (loading) return <PnLChartSkeleton />;
  if (!run) {
    return (
      <Card>
        <CardContent className="p-8 text-center text-sm text-slate-500">{t("common.empty")}</CardContent>
      </Card>
    );
  }

  return (
    <div className="grid gap-4">
      <div className="grid gap-3 md:grid-cols-4">
        <StatCard label={t("backtesting.totalReturn")} value={formatPercent(metric(metrics, ["roi", "ROI"]))} />
        <StatCard label={t("backtesting.alpha")} value={formatPercent(metric(metrics, ["alpha_vs_dca", "alpha"]))} />
        <StatCard label={t("common.maxDrawdown")} value={formatPercent(metric(metrics, ["max_drawdown", "MaxDrawdown"]))} danger />
        <StatCard label={t("backtesting.sharpe")} value={formatNumber(metric(metrics, ["sharpe", "Sharpe"]))} />
      </div>
      <Card className="bg-slate-900/30">
        <CardHeader>
          <div>
            <CardTitle>{t("backtesting.nav")}</CardTitle>
            <CardDescription>{t("backtesting.passiveLine")}</CardDescription>
          </div>
          <StatusBadge status={run.status} />
        </CardHeader>
        <CardContent>
          <BacktestChart data={chartData} />
        </CardContent>
      </Card>
      <div className="grid gap-3 md:grid-cols-4">
        <StatCard label={t("backtesting.window6m")} value={windowScore(result, "6m")} />
        <StatCard label={t("backtesting.window2y")} value={windowScore(result, "2y")} />
        <StatCard label={t("backtesting.window5y")} value={windowScore(result, "5y")} />
        <StatCard label={t("backtesting.windowAll")} value={windowScore(result, "all")} />
      </div>
      {run.error && <p className="rounded-lg border border-red-400/20 bg-red-400/10 p-3 text-xs text-red-300">{run.error}</p>}
    </div>
  );
}

function BacktestChart({ data }: { data: Array<{ time: string; nav: number; dca?: number }> }) {
  const { t } = useI18n();
  if (data.length === 0) {
    return <div className="flex h-72 items-center justify-center rounded-xl bg-slate-950/30 text-sm text-slate-500">{t("common.empty")}</div>;
  }
  return (
    <div className="h-72">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id="backtestFill" x1="0" x2="0" y1="0" y2="1">
              <stop offset="5%" stopColor="#2dd4bf" stopOpacity={0.35} />
              <stop offset="95%" stopColor="#2dd4bf" stopOpacity={0.02} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="rgba(148,163,184,0.12)" vertical={false} />
          <XAxis dataKey="time" tickLine={false} axisLine={false} tick={{ fill: "#64748b", fontSize: 11 }} />
          <YAxis tickLine={false} axisLine={false} tick={{ fill: "#64748b", fontSize: 11 }} width={74} />
          <Tooltip
            contentStyle={{ background: "#020617", border: "1px solid rgba(255,255,255,0.08)", borderRadius: 8 }}
            formatter={(value) => [formatCurrency(Number(value)), "USDT"]}
          />
          <Area type="monotone" dataKey="nav" stroke="#2dd4bf" strokeWidth={2} fill="url(#backtestFill)" />
          <Line type="monotone" dataKey="dca" stroke="#64748b" strokeDasharray="5 5" dot={false} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

function StatCard({ label, value, danger }: { label: string; value: string; danger?: boolean }) {
  return (
    <Card className="bg-slate-900/30">
      <CardContent className="p-4">
        <p className="text-xs text-slate-500">{label}</p>
        <p className={`qs-number mt-2 truncate text-xl font-semibold ${danger ? "text-red-300" : "text-slate-200"}`}>{value}</p>
      </CardContent>
    </Card>
  );
}

function useBacktestChartData(result: Record<string, unknown> | undefined, metrics: Record<string, unknown>) {
  return useMemo(() => {
    const nav = readCurve(result, ["nav_curve", "unit_nav_curve", "equity_curve"]);
    const dca = readCurve(result, ["ghost_dca_curve", "dca_curve"]);
    if (nav.length > 0) {
      return nav.map((point, index) => ({
        time: point.time ?? String(index + 1),
        nav: point.value,
        dca: dca[index]?.value
      }));
    }
    const finalEquity = metric(metrics, ["final_equity", "FinalEquity"]);
    return finalEquity > 0 ? [{ time: "1", nav: finalEquity, dca: finalEquity }] : [];
  }, [metrics, result]);
}

function readCurve(result: Record<string, unknown> | undefined, keys: string[]) {
  for (const key of keys) {
    const raw = result?.[key];
    if (Array.isArray(raw)) {
      return raw
        .map((item, index) => {
          if (typeof item === "number") return { time: String(index + 1), value: item };
          if (item && typeof item === "object") {
            const row = item as Record<string, unknown>;
            const value = row.value ?? row.nav ?? row.equity;
            return {
              time: String(row.time ?? row.date ?? index + 1),
              value: typeof value === "number" || typeof value === "string" ? toNumber(value) : 0
            };
          }
          return { time: String(index + 1), value: 0 };
        })
        .filter((item) => item.value > 0);
    }
  }
  return [];
}

function metric(metrics: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = metrics[key];
    if (typeof value === "number" || typeof value === "string") return toNumber(value);
  }
  return 0;
}

function formatNumber(value: number) {
  return Number.isFinite(value) && value !== 0 ? value.toFixed(2) : "-";
}

function windowScore(result: Record<string, unknown> | undefined, key: string) {
  const windows = (result?.window_scores ?? result?.scores ?? {}) as Record<string, unknown>;
  const value = windows[key] ?? windows[`${key}_score`];
  return typeof value === "number" ? value.toFixed(4) : typeof value === "string" ? value : "-";
}
