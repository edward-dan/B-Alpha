import { FormEvent, type ReactNode, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, FlaskConical, Play } from "lucide-react";
import { api, ApiRequestError } from "../lib/api";
import { formatDateTime, formatPercent, toNumber } from "../lib/format";
import { businessStrategyName, geneRoleLabel, strategyLabelFromID } from "../lib/terminology";
import { PageHeader } from "../components/layout/PageHeader";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Badge } from "../components/ui/badge";
import { Progress } from "../components/ui/progress";
import { TabsList, TabsTrigger } from "../components/ui/tabs";
import { EmptyState } from "../components/ui/empty-state";
import { useAuthStore } from "../stores/authStore";

type TabKey = "optimize" | "library";

export function EvolutionPage() {
  const token = useAuthStore((state) => state.token);
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<TabKey>("optimize");
  const [strategyId, setStrategyId] = useState("example_sigmoid_dca_v1");
  const [symbol, setSymbol] = useState("BTCUSDT");
  const [popSize, setPopSize] = useState("24");
  const [maxGenerations, setMaxGenerations] = useState("12");

  const statusQuery = useQuery({
    queryKey: ["evolution-tasks"],
    queryFn: () => api.getEvolutionTasks(token ?? ""),
    enabled: Boolean(token),
    refetchInterval: 30_000
  });

  const genomeQuery = useQuery({
    queryKey: ["challenger-genomes"],
    queryFn: () => api.getChallengerGenomes(token ?? ""),
    enabled: Boolean(token)
  });
  const templatesQuery = useQuery({
    queryKey: ["strategies"],
    queryFn: () => api.getStrategies(token ?? ""),
    enabled: Boolean(token)
  });

  const currentTask = (statusQuery.data?.current_task ?? null) as any;
  const challengers = useMemo(() => {
    const fromStatus = Array.isArray(statusQuery.data?.challengers) ? statusQuery.data?.challengers : [];
    return genomeQuery.data?.challengers ?? fromStatus ?? [];
  }, [genomeQuery.data?.challengers, statusQuery.data]);

  const createTask = useMutation({
    mutationFn: () =>
      api.createEvolutionTask(token ?? "", {
        strategy_id: strategyId,
        symbol,
        pop_size: Number(popSize),
        max_generations: Number(maxGenerations),
        spawn_mode: "inherit",
        test_mode: false
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evolution-tasks"] });
      queryClient.invalidateQueries({ queryKey: ["challenger-genomes"] });
    }
  });

  const promote = useMutation({
    mutationFn: ({ taskId, geneId }: { taskId: number; geneId?: number }) =>
      api.promoteEvolutionTask(token ?? "", taskId, geneId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evolution-tasks"] });
      queryClient.invalidateQueries({ queryKey: ["challenger-genomes"] });
    }
  });

  function submitTask(event: FormEvent) {
    event.preventDefault();
    createTask.mutate();
  }

  const createError = createTask.error instanceof ApiRequestError ? createTask.error.message : null;

  return (
    <div>
      <PageHeader
        title="进化实验室"
        description="在 Lab 或开发环境中触发参数优化，候选参数必须人工审批后才会成为当前最优参数。"
      />
      <TabsList className="mb-5">
        <TabsTrigger active={tab === "optimize"} onClick={() => setTab("optimize")}>
          任务监控
        </TabsTrigger>
        <TabsTrigger active={tab === "library"} onClick={() => setTab("library")}>
          基因库
        </TabsTrigger>
      </TabsList>

      {tab === "optimize" ? (
        <div className="grid gap-4 xl:grid-cols-[0.8fr_1.2fr]">
          <Card>
            <CardHeader>
              <div>
                <CardTitle>触发参数优化</CardTitle>
                <CardDescription>使用当前最优参数作为起点，生成新的候选参数。</CardDescription>
              </div>
              <FlaskConical className="h-4 w-4 text-accent" />
            </CardHeader>
            <CardContent>
              <form className="space-y-4" onSubmit={submitTask}>
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
                <Field label="交易对">
                  <Input value={symbol} onChange={(event) => setSymbol(event.target.value.toUpperCase())} />
                </Field>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="候选规模">
                    <Input type="number" min="8" value={popSize} onChange={(event) => setPopSize(event.target.value)} />
                  </Field>
                  <Field label="优化轮数">
                    <Input
                      type="number"
                      min="1"
                      value={maxGenerations}
                      onChange={(event) => setMaxGenerations(event.target.value)}
                    />
                  </Field>
                </div>
                {createError && <div className="rounded-md border border-danger/20 bg-danger/10 p-3 text-xs text-danger">{createError}</div>}
                <Button variant="primary" type="submit" disabled={createTask.isPending}>
                  <Play className="h-4 w-4" />
                  {createTask.isPending ? "提交中" : "开始优化"}
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <div>
                <CardTitle>进度监控</CardTitle>
                <CardDescription>每 30 秒刷新任务状态。</CardDescription>
              </div>
              {currentTask && <Badge tone="info">{taskStatusLabel(currentTask.status)}</Badge>}
            </CardHeader>
            <CardContent>
              {currentTask ? (
                <div className="space-y-5">
                  <div>
                    <div className="mb-2 flex items-center justify-between text-xs text-text-muted">
                      <span>优化进度</span>
                      <span className="qs-number">{currentTask.progress ?? 0}%</span>
                    </div>
                    <Progress value={Number(currentTask.progress ?? 0)} />
                  </div>
                  <div className="grid gap-3 md:grid-cols-3">
                    <Metric label="当前轮次" value={String(currentTask.current_generation ?? 0)} />
                    <Metric label="最佳评分" value={Number(currentTask.best_score ?? 0).toFixed(4)} />
                    <Metric label="开始时间" value={formatDateTime(currentTask.started_at)} />
                  </div>
                  {currentTask.best_gene_id && (
                    <Button
                      variant="primary"
                      disabled={promote.isPending}
                      onClick={() =>
                        promote.mutate({
                          taskId: Number(currentTask.id),
                          geneId: Number(currentTask.best_gene_id)
                        })
                      }
                    >
                      <Check className="h-4 w-4" />
                      审批为当前最优参数
                    </Button>
                  )}
                </div>
              ) : (
                <EmptyState title="暂无运行任务" description="提交参数优化任务后，这里会显示进度和候选参数审批入口。" />
              )}
            </CardContent>
          </Card>
        </div>
      ) : (
        <Card>
          <CardHeader>
            <div>
              <CardTitle>参数记录</CardTitle>
              <CardDescription>只展示候选参数与关键评分，详细参数包由后端保管。</CardDescription>
            </div>
          </CardHeader>
          <CardContent>
            {challengers.length === 0 ? (
              <EmptyState title="暂无候选参数" description="参数优化完成后，候选记录会出现在这里。" />
            ) : (
              <div className="grid gap-3">
                {challengers.map((gene: any) => (
                  <div key={gene.id} className="grid gap-3 rounded-lg border border-white/[0.05] bg-white/[0.025] p-3 md:grid-cols-5 md:items-center">
                    <div className="md:col-span-2">
                      <p className="text-sm font-medium text-text-main">{gene.symbol}</p>
                      <p className="mt-1 text-xs text-text-muted">{strategyLabelFromID(gene.strategy_id)}</p>
                    </div>
                    <Badge tone={gene.role === "challenger" ? "accent" : "neutral"}>{geneRoleLabel(gene.role)}</Badge>
                    <Metric label="评分" value={Number(gene.score_total ?? 0).toFixed(4)} compact />
                    <Metric label="最大回撤" value={formatPercent(toNumber(gene.max_drawdown))} compact />
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      )}
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

function taskStatusLabel(status?: string) {
  switch (status) {
    case "running":
      return "运行中";
    case "succeeded":
      return "已完成";
    case "failed":
      return "异常";
    case "queued":
      return "排队中";
    case "canceled":
      return "已取消";
    default:
      return "未知";
  }
}
