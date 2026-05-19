import { useMemo, useState } from "react";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis
} from "recharts";
import { Play, Plus, Settings, Square, X } from "lucide-react";
import { Button } from "../../components/ui/button";
import { useI18n } from "../../i18n/useI18n";
import { formatAsset, formatCurrency, formatDateTime, toNumber } from "../../lib/format";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../shared/ui/Card";
import { StatusBadge } from "../../shared/ui/StatusBadge";
import { PnLChartSkeleton } from "../../shared/ui/skeletons";
import { dashboardService, instancesService } from "../../shared/services";
import type { EquitySnapshot, StrategyInstance } from "../../types/api";
import { catalogItemForInstance } from "../strategies/strategyCatalog";

type RangeKey = "7" | "30" | "90";

export function DashboardPage() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const [range, setRange] = useState<RangeKey>("30");
  const [sheetOpen, setSheetOpen] = useState(false);

  const instancesQuery = useQuery({
    queryKey: ["instances"],
    queryFn: instancesService.list,
    refetchInterval: 60_000
  });
  const instances = instancesQuery.data?.instances ?? [];
  const selectedId = Number(searchParams.get("instance"));
  const selectedInstance = instances.find((item) => item.id === selectedId) ?? instances[0] ?? null;

  const equityQuery = useQuery({
    queryKey: ["equity-snapshots", selectedInstance?.id, range],
    queryFn: () => dashboardService.equitySnapshots(selectedInstance?.id ?? 0, Number(range)),
    enabled: Boolean(selectedInstance?.id),
    refetchInterval: 60_000
  });

  const transition = useMutation({
    mutationFn: ({ id, status }: { id: number; status: "running" | "stopped" }) =>
      instancesService.updateStatus(id, status),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["instances"] });
      queryClient.invalidateQueries({ queryKey: ["equity-snapshots"] });
    }
  });

  const notice = typeof location.state === "object" && location.state && "notice" in location.state ? String(location.state.notice) : "";

  function selectInstance(instance: StrategyInstance) {
    navigate(`/?instance=${instance.id}`);
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-wider text-slate-200">{t("dashboard.title")}</h1>
          {notice && <p className="mt-1 text-xs text-accent">{notice}</p>}
        </div>
        <Button
          variant="secondary"
          title={t("dashboard.openConfig")}
          aria-label={t("dashboard.openConfig")}
          icon={<Settings className="h-4 w-4" />}
          onClick={() => setSheetOpen(true)}
        />
      </div>

      <div className="qs-bento-grid lg:grid-cols-[minmax(260px,1fr)_3fr] xl:grid-cols-[minmax(280px,1fr)_3fr]">
        <Card className="flex min-h-[calc(100vh-9rem)] flex-col bg-slate-900/30">
          <CardHeader>
            <div>
              <CardTitle>{t("dashboard.instanceList")}</CardTitle>
              <CardDescription>{selectedInstance ? selectedInstance.symbol : t("common.empty")}</CardDescription>
            </div>
          </CardHeader>
          <CardContent className="flex min-h-0 flex-1 flex-col gap-3">
            <div className="grid flex-1 content-start gap-3 overflow-y-auto pr-1">
              {instances.length === 0 ? (
                <div className="rounded-xl border border-dashed border-white/[0.08] p-4 text-sm text-slate-400">
                  <p className="font-medium text-slate-300">{t("dashboard.noInstances")}</p>
                  <p className="mt-2 text-xs leading-5">{t("dashboard.noInstancesHint")}</p>
                </div>
              ) : (
                instances.map((instance) => (
                  <button
                    key={instance.id}
                    type="button"
                    className={`rounded-xl border bg-slate-950/35 p-3 text-left transition ${
                      instance.id === selectedInstance?.id
                        ? "border-l-4 border-l-accent border-white/[0.06]"
                        : "border-white/[0.04] hover:border-white/[0.1]"
                    }`}
                    onClick={() => selectInstance(instance)}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-semibold text-slate-200">{instance.name}</p>
                        <p className="mt-1 text-xs text-slate-500">{instance.symbol}</p>
                      </div>
                      <StatusBadge status={normalizeInstanceStatus(instance.status)} />
                    </div>
                    <div className="mt-3 flex items-center justify-between gap-2">
                      <span className="text-xs text-slate-500">{catalogItemForInstance(instance).name}</span>
                      <Button
                        size="sm"
                        variant={instance.status === "RUNNING" ? "secondary" : "primary"}
                        disabled={transition.isPending}
                        onClick={(event) => {
                          event.stopPropagation();
                          transition.mutate({
                            id: instance.id,
                            status: instance.status === "RUNNING" ? "stopped" : "running"
                          });
                        }}
                      >
                        {instance.status === "RUNNING" ? <Square className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
                        {instance.status === "RUNNING" ? t("common.pause") : t("common.start")}
                      </Button>
                    </div>
                  </button>
                ))
              )}
            </div>
            <Link
              to="/instances/new"
              className="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-accent bg-accent px-4 text-sm font-medium uppercase tracking-wider text-slate-950 transition hover:bg-accent/90"
            >
              <Plus className="h-4 w-4" />
              {t("dashboard.newInstance")}
            </Link>
          </CardContent>
        </Card>

        <div className="grid gap-4">
          <StrategyOverviewCard instance={selectedInstance} />
          <Card>
            <CardHeader className="items-center">
              <div>
                <CardTitle>{t("dashboard.pnl")}</CardTitle>
                <CardDescription>{selectedInstance?.symbol ?? t("common.empty")}</CardDescription>
              </div>
              <div className="flex rounded-lg border border-white/[0.06] bg-slate-950/40 p-1">
                {(["7", "30", "90"] as RangeKey[]).map((item) => (
                  <button
                    key={item}
                    type="button"
                    className={`h-8 rounded-md px-3 text-xs font-medium transition ${
                      range === item ? "bg-accent/10 text-accent" : "text-slate-500 hover:text-slate-300"
                    }`}
                    onClick={() => setRange(item)}
                  >
                    {t(`dashboard.range${item}`)}
                  </button>
                ))}
              </div>
            </CardHeader>
            <CardContent>
              {equityQuery.isLoading ? (
                <PnLChartSkeleton />
              ) : (
                <PnLChart data={equityQuery.data?.snapshots ?? []} />
              )}
            </CardContent>
          </Card>
          <StrategyJourneyCard instance={selectedInstance} />
        </div>
      </div>
      <ConfigFormSheet instance={selectedInstance} open={sheetOpen} onClose={() => setSheetOpen(false)} />
    </div>
  );
}

function StrategyOverviewCard({ instance }: { instance: StrategyInstance | null }) {
  const { t } = useI18n();
  const portfolio = instance?.portfolio;
  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>{t("dashboard.overview")}</CardTitle>
          <CardDescription>{instance ? catalogItemForInstance(instance).name : t("common.empty")}</CardDescription>
        </div>
        {instance && <StatusBadge status={normalizeInstanceStatus(instance.status)} />}
      </CardHeader>
      <CardContent>
        <div className="grid gap-3 md:grid-cols-5">
          <AssetMetric className="md:col-span-2" label={t("common.totalAssets")} value={formatCurrency(portfolio?.total_equity)} large />
          <AssetMetric label={t("dashboard.longHolding")} value={formatAsset(portfolio?.dead_btc)} />
          <AssetMetric label={t("dashboard.activePosition")} value={formatAsset(portfolio?.float_btc)} />
          <AssetMetric label={t("dashboard.availableFunds")} value={formatCurrency(portfolio?.usdt_balance)} />
          <AssetMetric label={t("dashboard.sealedAssets")} value={formatAsset(portfolio?.cold_sealed_btc)} />
        </div>
        <div className="mt-3 rounded-lg border border-white/[0.04] bg-slate-950/30 p-3">
          <p className="text-xs text-slate-500">{t("dashboard.lastDecision")}</p>
          <p className="qs-number mt-1 text-sm text-slate-200">{formatDateTime(portfolio?.last_processed_bar_time)}</p>
        </div>
      </CardContent>
    </Card>
  );
}

function AssetMetric({
  label,
  value,
  large,
  className
}: {
  label: string;
  value: string;
  large?: boolean;
  className?: string;
}) {
  return (
    <div className={`rounded-lg border border-white/[0.04] bg-slate-950/30 p-3 ${className ?? ""}`}>
      <p className="text-xs text-slate-500">{label}</p>
      <p className={`qs-number mt-2 truncate font-semibold text-slate-200 ${large ? "text-2xl" : "text-sm"}`}>{value}</p>
    </div>
  );
}

function PnLChart({ data }: { data: EquitySnapshot[] }) {
  const { t } = useI18n();
  const chartData = useMemo(
    () =>
      data.map((item) => ({
        time: formatChartTime(item.time),
        total: toNumber(item.total_equity)
      })),
    [data]
  );

  if (chartData.length === 0) {
    return <div className="flex h-72 items-center justify-center rounded-xl bg-slate-950/30 text-sm text-slate-500">{t("common.empty")}</div>;
  }

  return (
    <div className="h-72">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={chartData} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id="equityFill" x1="0" x2="0" y1="0" y2="1">
              <stop offset="5%" stopColor="#2dd4bf" stopOpacity={0.35} />
              <stop offset="95%" stopColor="#2dd4bf" stopOpacity={0.02} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="rgba(148,163,184,0.12)" vertical={false} />
          <XAxis dataKey="time" tickLine={false} axisLine={false} tick={{ fill: "#64748b", fontSize: 11 }} />
          <YAxis tickLine={false} axisLine={false} tick={{ fill: "#64748b", fontSize: 11 }} width={74} />
          <Tooltip
            contentStyle={{ background: "#020617", border: "1px solid rgba(255,255,255,0.08)", borderRadius: 8 }}
            labelStyle={{ color: "#cbd5e1" }}
            formatter={(value) => [formatCurrency(Number(value)), "USDT"]}
          />
          <Area type="monotone" dataKey="total" stroke="#2dd4bf" strokeWidth={2} fill="url(#equityFill)" />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

function StrategyJourneyCard({ instance }: { instance: StrategyInstance | null }) {
  const { t } = useI18n();
  const state = instance?.runtime_state?.state ?? {};
  const decisionCount = valueFromState(state, ["decision_count", "decisions", "runs"]);
  const monthTrades = valueFromState(state, ["month_trades", "monthly_trades"]);

  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle>{t("dashboard.journey")}</CardTitle>
          <CardDescription>{instance?.name ?? t("common.empty")}</CardDescription>
        </div>
      </CardHeader>
      <CardContent className="grid gap-3 md:grid-cols-3">
        <AssetMetric label={t("dashboard.firstRun")} value={formatDateTime(instance?.created_at)} />
        <AssetMetric label={t("dashboard.decisionCount")} value={decisionCount} />
        <AssetMetric label={t("dashboard.monthTrades")} value={monthTrades} />
      </CardContent>
    </Card>
  );
}

function ConfigFormSheet({ instance, open, onClose }: { instance: StrategyInstance | null; open: boolean; onClose: () => void }) {
  const { t } = useI18n();
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-slate-950/60 backdrop-blur-sm">
      <aside className="h-full w-full max-w-md border-l border-white/[0.06] bg-slate-950/90 p-5 shadow-2xl">
        <div className="mb-5 flex items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold tracking-wider text-slate-200">{t("dashboard.configTitle")}</h2>
            <p className="mt-1 text-xs leading-5 text-slate-500">{t("dashboard.configHint")}</p>
          </div>
          <Button variant="ghost" size="icon" title={t("common.close")} aria-label={t("common.close")} icon={<X className="h-4 w-4" />} onClick={onClose} />
        </div>
        {instance ? (
          <div className="grid gap-3">
            <AssetMetric label={t("instanceCreate.instanceName")} value={instance.name} />
            <AssetMetric label={t("common.symbol")} value={`${instance.symbol} / ${instance.interval}`} />
            <AssetMetric label={t("instances.strategyType")} value={catalogItemForInstance(instance).name} />
            <Link className="mt-2 inline-flex h-10 items-center justify-center rounded-md bg-accent px-4 text-sm font-medium text-slate-950" to="/settings">
              {t("common.settings")}
            </Link>
          </div>
        ) : (
          <p className="text-sm text-slate-500">{t("dashboard.noInstancesHint")}</p>
        )}
      </aside>
    </div>
  );
}

function normalizeInstanceStatus(status?: string) {
  if (status === "RUNNING") return "running";
  if (status === "ERROR") return "error";
  return "stopped";
}

function formatChartTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit" });
}

function valueFromState(state: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = state[key];
    if (typeof value === "number" || typeof value === "string") return String(value);
  }
  return "0";
}
