import { Link, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Activity, ArrowUpRight, CircleDollarSign, Clock, Plus, ShieldCheck, WalletCards } from "lucide-react";
import { api } from "../lib/api";
import { buildNavSeries, emptyDashboard } from "../lib/mockData";
import { actionLabel, businessStrategyName, engineLabel, instanceStatusLabel } from "../lib/terminology";
import { formatAsset, formatCurrency, formatDateTime, toNumber } from "../lib/format";
import { PageHeader } from "../components/layout/PageHeader";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Badge } from "../components/ui/badge";
import { EmptyState } from "../components/ui/empty-state";
import { NavChart } from "../components/charts/NavChart";
import { AllocationChart } from "../components/charts/AllocationChart";
import { useAuthStore } from "../stores/authStore";

export function DashboardPage() {
  const token = useAuthStore((state) => state.token);
  const [searchParams] = useSearchParams();
  const selectedId = Number(searchParams.get("instance"));

  const dashboardQuery = useQuery({
    queryKey: ["dashboard"],
    queryFn: () => api.getDashboard(token ?? ""),
    enabled: Boolean(token)
  });

  const dashboard = dashboardQuery.data ?? emptyDashboard;
  const selectedInstance =
    dashboard.instances.find((item) => item.id === selectedId) ?? dashboard.instances[0] ?? null;
  const navSeries = buildNavSeries(dashboard.instances);

  return (
    <div>
      <PageHeader
        title="Dashboard 总览"
        description="聚合实例运行、总资产曲线、资产语义账本与最近策略决策结果。"
        action={
          <Link
            to="/instances/new"
            className="inline-flex h-10 items-center gap-2 rounded-md border border-accent/20 bg-accent/15 px-4 text-sm font-medium text-accent transition duration-150 hover:bg-accent/20"
          >
            <Plus className="h-4 w-4" />
            创建实例
          </Link>
        }
      />

      <div className="qs-bento-grid">
        <MetricCard
          title="总资产"
          value={formatCurrency(dashboard.overview.total_equity)}
          icon={<CircleDollarSign className="h-4 w-4" />}
          tone="accent"
        />
        <MetricCard
          title="可用资金"
          value={formatCurrency(dashboard.overview.usdt_balance)}
          icon={<WalletCards className="h-4 w-4" />}
          tone="info"
        />
        <MetricCard
          title="活跃实例"
          value={`${dashboard.overview.running_count} / ${dashboard.overview.instances_total}`}
          icon={<Activity className="h-4 w-4" />}
          tone="success"
        />
        <MetricCard
          title="待确认指令"
          value={String(dashboard.overview.pending_commands)}
          icon={<Clock className="h-4 w-4" />}
          tone="warm"
        />

        <Card className="md:col-span-2 xl:col-span-3">
          <CardHeader>
            <div>
              <CardTitle>总资产曲线</CardTitle>
              <CardDescription>按当前实例资产生成的净值趋势，用于观察资金曲线连续性。</CardDescription>
            </div>
            <Badge tone="accent">NAV</Badge>
          </CardHeader>
          <CardContent>
            <NavChart data={navSeries} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div>
              <CardTitle>策略旅程</CardTitle>
              <CardDescription>当前选中实例的资产结构与最近处理时间。</CardDescription>
            </div>
          </CardHeader>
          <CardContent>
            {selectedInstance ? (
              <div className="space-y-4">
                <div className="rounded-lg border border-white/[0.05] bg-white/[0.03] p-3">
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-text-main">{selectedInstance.name}</p>
                      <p className="mt-1 text-xs text-text-muted">
                        {selectedInstance.symbol} · {selectedInstance.interval}
                      </p>
                    </div>
                    <Badge tone={selectedInstance.status === "RUNNING" ? "success" : selectedInstance.status === "ERROR" ? "danger" : "neutral"}>
                      {instanceStatusLabel(selectedInstance.status)}
                    </Badge>
                  </div>
                </div>
                <AllocationChart portfolio={selectedInstance.portfolio} />
                <div className="grid grid-cols-2 gap-3 text-xs">
                  <AssetLine label="长期持仓" value={formatAsset(selectedInstance.portfolio?.dead_btc)} />
                  <AssetLine label="活跃仓位" value={formatAsset(selectedInstance.portfolio?.float_btc)} />
                  <AssetLine label="封存资产" value={formatAsset(selectedInstance.portfolio?.cold_sealed_btc)} />
                  <AssetLine
                    label="最近决策"
                    value={formatDateTime(selectedInstance.portfolio?.last_processed_bar_time)}
                  />
                </div>
              </div>
            ) : (
              <EmptyState title="还没有实例" description="创建第一个实例后，这里会展示资产曲线和策略旅程。" />
            )}
          </CardContent>
        </Card>

        <Card className="md:col-span-2 xl:col-span-2">
          <CardHeader>
            <div>
              <CardTitle>实例卡片</CardTitle>
              <CardDescription>URL 参数 instance 会自动选中对应实例。</CardDescription>
            </div>
          </CardHeader>
          <CardContent className="grid gap-3 md:grid-cols-2">
            {dashboard.instances.length === 0 ? (
              <EmptyState
                className="md:col-span-2"
                title="暂无实例"
                description="实例创建后会在这里展示运行状态、资金规模和最新处理时间。"
              />
            ) : (
              dashboard.instances.map((instance) => (
                <Link
                  key={instance.id}
                  to={`/?instance=${instance.id}`}
                  className={`rounded-lg border p-3 transition duration-150 ${
                    instance.id === selectedInstance?.id
                      ? "border-accent/20 bg-accent/[0.06]"
                      : "border-white/[0.05] bg-white/[0.025] hover:bg-white/[0.04]"
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <p className="truncate text-sm font-medium text-text-main">{instance.name}</p>
                    <ArrowUpRight className="h-4 w-4 text-text-weak" />
                  </div>
                  <p className="mt-1 text-xs text-text-muted">{businessStrategyName(instance.template?.name)}</p>
                  <div className="mt-4 flex items-end justify-between gap-3">
                    <div>
                      <p className="qs-number text-lg text-text-main">{formatCurrency(instance.portfolio?.total_equity)}</p>
                      <p className="mt-1 text-[11px] text-text-weak">{instance.symbol}</p>
                    </div>
                    <Badge tone={instance.status === "RUNNING" ? "success" : instance.status === "ERROR" ? "danger" : "neutral"}>
                      {instanceStatusLabel(instance.status)}
                    </Badge>
                  </div>
                </Link>
              ))
            )}
          </CardContent>
        </Card>

        <Card className="md:col-span-2 xl:col-span-2">
          <CardHeader>
            <div>
              <CardTitle>最近策略决策</CardTitle>
              <CardDescription>展示最近成交与策略决策来源，避免暴露内部原因码。</CardDescription>
            </div>
            <ShieldCheck className="h-4 w-4 text-accent" />
          </CardHeader>
          <CardContent>
            {dashboard.recent_trades.length === 0 ? (
              <EmptyState title="暂无成交记录" description="Agent 上报成交后会在这里显示最近决策结果。" />
            ) : (
              <div className="space-y-2">
                {dashboard.recent_trades.map((trade) => (
                  <div key={trade.id} className="flex items-center justify-between gap-3 rounded-lg bg-white/[0.025] p-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm text-text-main">
                        {actionLabel(trade.action)} · {engineLabel(trade.engine)}
                      </p>
                      <p className="mt-1 text-xs text-text-muted">
                        {trade.symbol} · {formatDateTime(trade.executed_at)}
                      </p>
                    </div>
                    <div className="text-right">
                      <p className="qs-number text-sm text-text-main">{formatAsset(trade.filled_qty, trade.symbol.replace("USDT", ""))}</p>
                      <p className="qs-number mt-1 text-xs text-text-muted">{formatCurrency(toNumber(trade.filled_price))}</p>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function MetricCard({
  title,
  value,
  icon,
  tone
}: {
  title: string;
  value: string;
  icon: JSX.Element;
  tone: "accent" | "info" | "success" | "warm";
}) {
  const toneClass = {
    accent: "border-accent/20 bg-accent/10 text-accent",
    info: "border-info/20 bg-info/10 text-info",
    success: "border-success/20 bg-success/10 text-success",
    warm: "border-warm/20 bg-warm/10 text-warm"
  }[tone];

  return (
    <Card>
      <CardContent className="pt-4">
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs text-text-muted">{title}</p>
          <div className={`flex h-8 w-8 items-center justify-center rounded-md border ${toneClass}`}>{icon}</div>
        </div>
        <p className="qs-number mt-4 truncate text-2xl font-semibold text-text-main">{value}</p>
      </CardContent>
    </Card>
  );
}

function AssetLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md bg-white/[0.025] p-3">
      <p className="text-text-weak">{label}</p>
      <p className="qs-number mt-1 truncate text-text-main">{value}</p>
    </div>
  );
}
