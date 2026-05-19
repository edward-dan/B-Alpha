import { FormEvent, type ReactNode, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { BarChart3, Play } from "lucide-react";
import { api, ApiRequestError } from "../lib/api";
import { businessStrategyName } from "../lib/terminology";
import { formatDateTime } from "../lib/format";
import { PageHeader } from "../components/layout/PageHeader";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Badge } from "../components/ui/badge";
import { EmptyState } from "../components/ui/empty-state";
import { useAuthStore } from "../stores/authStore";
import type { BacktestRun } from "../types/api";

export function BacktestingPage() {
  const token = useAuthStore((state) => state.token);
  const [strategyId, setStrategyId] = useState("example_sigmoid_dca_v1");
  const [symbol, setSymbol] = useState("BTCUSDT");
  const [interval, setInterval] = useState("1h");
  const [geneId, setGeneId] = useState("");
  const [limit, setLimit] = useState("5000");
  const [paramPack, setParamPack] = useState("");
  const [lastRun, setLastRun] = useState<BacktestRun | null>(null);

  const templatesQuery = useQuery({
    queryKey: ["strategies"],
    queryFn: () => api.getStrategies(token ?? ""),
    enabled: Boolean(token)
  });

  const mutation = useMutation({
    mutationFn: () => {
      const parsedPack = paramPack.trim() ? JSON.parse(paramPack) : undefined;
      return api.createBacktest(token ?? "", {
        strategy_id: strategyId,
        symbol,
        interval,
        gene_id: geneId ? Number(geneId) : undefined,
        param_pack: parsedPack,
        limit: Number(limit)
      });
    },
    onSuccess: (data) => setLastRun(data.backtest)
  });

  function onSubmit(event: FormEvent) {
    event.preventDefault();
    mutation.mutate();
  }

  const error = mutation.error instanceof ApiRequestError ? mutation.error.message : mutation.error ? "参数包 JSON 格式不正确" : null;
  const result = lastRun?.result as any;
  const metrics = result?.metrics ?? {};
  const coverage = result?.data_coverage ?? {};

  return (
    <div>
      <PageHeader
        title="回测触发与结果展示"
        description="回测只在 Lab 或开发环境开放，并通过与实盘一致的策略决策入口执行。"
      />
      <div className="grid gap-4 xl:grid-cols-[0.75fr_1.25fr]">
        <Card>
          <CardHeader>
            <div>
              <CardTitle>创建回测</CardTitle>
              <CardDescription>可使用候选参数编号，或粘贴完整参数包。</CardDescription>
            </div>
            <BarChart3 className="h-4 w-4 text-info" />
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={onSubmit}>
              <Field label="策略">
                <select
                  className="h-10 w-full rounded-md border border-white/[0.06] bg-slate-950/60 px-3 text-sm text-text-main outline-none focus:border-accent/40"
                  value={strategyId}
                  onChange={(event) => setStrategyId(event.target.value)}
                >
                  <option value="example_sigmoid_dca_v1">动态平衡现货策略</option>
                  {(templatesQuery.data?.strategies ?? []).map((template) => {
                    const optionValue = String(template.manifest?.id ?? template.name);
                    return (
                      <option key={template.id} value={optionValue}>
                        {businessStrategyName(String(template.manifest?.name ?? template.name))}
                      </option>
                    );
                  })}
                </select>
              </Field>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="交易对">
                  <Input value={symbol} onChange={(event) => setSymbol(event.target.value.toUpperCase())} />
                </Field>
                <Field label="周期">
                  <Input value={interval} onChange={(event) => setInterval(event.target.value)} />
                </Field>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="候选参数编号">
                  <Input type="number" min="0" value={geneId} onChange={(event) => setGeneId(event.target.value)} />
                </Field>
                <Field label="最大 K 线数量">
                  <Input type="number" min="100" value={limit} onChange={(event) => setLimit(event.target.value)} />
                </Field>
              </div>
              <div className="space-y-2">
                <Label>参数包 JSON</Label>
                <textarea
                  className="min-h-32 w-full rounded-md border border-white/[0.06] bg-slate-950/60 p-3 font-mono text-xs text-text-main outline-none transition duration-150 placeholder:text-text-weak focus:border-accent/40"
                  value={paramPack}
                  onChange={(event) => setParamPack(event.target.value)}
                  placeholder='{"chromosome":{},"spawn_point":{}}'
                />
              </div>
              {error && <div className="rounded-md border border-danger/20 bg-danger/10 p-3 text-xs text-danger">{error}</div>}
              <Button variant="primary" type="submit" disabled={mutation.isPending || (!geneId && !paramPack.trim())}>
                <Play className="h-4 w-4" />
                {mutation.isPending ? "运行中" : "开始回测"}
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div>
              <CardTitle>回测结果</CardTitle>
              <CardDescription>展示净值、回撤、数据覆盖范围和运行状态。</CardDescription>
            </div>
            {lastRun && <Badge tone={lastRun.status === "succeeded" ? "success" : lastRun.status === "failed" ? "danger" : "info"}>{runStatusLabel(lastRun.status)}</Badge>}
          </CardHeader>
          <CardContent>
            {!lastRun ? (
              <EmptyState title="暂无回测结果" description="提交回测后，结果会显示在这里。" />
            ) : (
              <div className="space-y-4">
                <div className="grid gap-3 md:grid-cols-4">
                  <Metric label="净值" value={metricValue(metrics.final_equity ?? metrics.FinalEquity)} />
                  <Metric label="收益率" value={metricValue(metrics.roi ?? metrics.ROI)} />
                  <Metric label="最大回撤" value={metricValue(metrics.max_drawdown ?? metrics.MaxDrawdown)} />
                  <Metric label="K 线数量" value={String(result?.bars_count ?? 0)} />
                </div>
                <div className="rounded-lg border border-white/[0.05] bg-white/[0.025] p-4">
                  <p className="text-sm font-medium text-text-main">数据覆盖</p>
                  <div className="mt-3 grid gap-3 md:grid-cols-3">
                    <Metric label="开始" value={formatDateTime(Number(coverage.start_open_time_ms))} compact />
                    <Metric label="结束" value={formatDateTime(Number(coverage.end_open_time_ms))} compact />
                    <Metric label="评估起点" value={formatDateTime(Number(coverage.eval_start_ms))} compact />
                  </div>
                </div>
                {lastRun.error && <div className="rounded-md border border-danger/20 bg-danger/10 p-3 text-xs text-danger">{lastRun.error}</div>}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      {children}
    </div>
  );
}

function Metric({ label, value, compact }: { label: string; value: string; compact?: boolean }) {
  return (
    <div className={compact ? "" : "rounded-md bg-white/[0.025] p-3"}>
      <p className="text-[11px] text-text-weak">{label}</p>
      <p className="qs-number mt-1 truncate text-sm text-text-main">{value}</p>
    </div>
  );
}

function metricValue(value: unknown) {
  if (typeof value === "number") return Number.isFinite(value) ? value.toFixed(4) : "暂无";
  if (typeof value === "string") return value || "暂无";
  return "暂无";
}

function runStatusLabel(status: string) {
  switch (status) {
    case "succeeded":
      return "已完成";
    case "failed":
      return "异常";
    default:
      return "运行中";
  }
}
