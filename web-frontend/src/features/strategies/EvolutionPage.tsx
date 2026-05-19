import { FormEvent, useState, type ReactNode } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, FlaskConical, Play, Square } from "lucide-react";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import { Label } from "../../components/ui/label";
import { Progress } from "../../components/ui/progress";
import { TabsList, TabsTrigger } from "../../components/ui/tabs";
import { useI18n } from "../../i18n/useI18n";
import { formatDateTime, formatPercent, toNumber } from "../../lib/format";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../shared/ui/Card";
import { StatusBadge } from "../../shared/ui/StatusBadge";
import { evolutionService, instancesService, ApiRequestError } from "../../shared/services";
import type { EvolutionTask, GeneRecord, StrategyInstance } from "../../types/api";
import { catalogItemForInstance } from "./strategyCatalog";

type TabKey = "optimize" | "library";
type SpawnMode = "inherit" | "random_once" | "manual";

export function EvolutionPage() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<TabKey>("optimize");
  const [expanded, setExpanded] = useState(false);
  const [popSize, setPopSize] = useState("300");
  const [maxGenerations, setMaxGenerations] = useState("25");
  const [spawnMode, setSpawnMode] = useState<SpawnMode>("inherit");
  const [manualJson, setManualJson] = useState("");
  const [confirmCancel, setConfirmCancel] = useState(false);
  const [promoteGene, setPromoteGene] = useState<GeneRecord | null>(null);

  const instancesQuery = useQuery({
    queryKey: ["instances"],
    queryFn: instancesService.list,
    refetchInterval: 60_000
  });
  const evolutionInstances = (instancesQuery.data?.instances ?? []).filter((instance) => catalogItemForInstance(instance).features.evolution);
  const selectedId = Number(searchParams.get("instance"));
  const selectedInstance = evolutionInstances.find((item) => item.id === selectedId) ?? evolutionInstances[0] ?? null;
  const catalog = catalogItemForInstance(selectedInstance);

  const tasksQuery = useQuery({
    queryKey: ["evolution-tasks"],
    queryFn: evolutionService.tasks,
    refetchInterval: 5_000
  });
  const genomesQuery = useQuery({
    queryKey: ["evolution-genomes", selectedInstance?.id],
    queryFn: () => evolutionService.genomes(selectedInstance?.id),
    enabled: Boolean(selectedInstance?.id),
    refetchInterval: 30_000
  });

  const currentTask = tasksQuery.data?.current_task?.status === "running" ? tasksQuery.data.current_task : null;
  const allTasks = compactTasks(tasksQuery.data?.current_task, tasksQuery.data?.tasks, tasksQuery.data?.active_tasks, tasksQuery.data?.recent_tasks);
  const genomes = genomesQuery.data?.genomes ?? [];
  const champion = genomes.find((item) => item.role === "champion") ?? null;

  const createTask = useMutation({
    mutationFn: () => {
      const manual = manualJson.trim() ? JSON.parse(manualJson) : undefined;
      return evolutionService.createTask({
        strategy_id: catalog.strategyId,
        symbol: selectedInstance?.symbol ?? catalog.symbols[0],
        pop_size: Number(popSize),
        max_generations: Number(maxGenerations),
        spawn_mode: spawnMode,
        spawn_point: spawnMode === "manual" ? manual : undefined
      });
    },
    onSuccess: () => {
      setExpanded(false);
      queryClient.invalidateQueries({ queryKey: ["evolution-tasks"] });
      queryClient.invalidateQueries({ queryKey: ["evolution-genomes"] });
    }
  });

  const cancelTask = useMutation({
    mutationFn: (taskId: number) => evolutionService.cancelTask(taskId),
    onSuccess: () => {
      setConfirmCancel(false);
      queryClient.invalidateQueries({ queryKey: ["evolution-tasks"] });
    }
  });

  const promote = useMutation({
    mutationFn: (gene: GeneRecord) => evolutionService.promote(resolveTaskIdForGene(gene, allTasks), gene.id),
    onSuccess: () => {
      setPromoteGene(null);
      queryClient.invalidateQueries({ queryKey: ["evolution-genomes"] });
      queryClient.invalidateQueries({ queryKey: ["evolution-tasks"] });
    }
  });

  function submitTask(event: FormEvent) {
    event.preventDefault();
    createTask.mutate();
  }

  const createError = createTask.error instanceof ApiRequestError ? createTask.error.message : createTask.error ? t("common.invalidJson") : null;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold tracking-wider text-slate-200">{t("evolution.title")}</h1>
        <InstanceSelector
          instances={evolutionInstances}
          selected={selectedInstance}
          onSelect={(id) => navigate(`/evolution?instance=${id}`)}
        />
      </div>

      <TabsList>
        <TabsTrigger active={tab === "optimize"} onClick={() => setTab("optimize")}>
          {t("evolution.optimize")}
        </TabsTrigger>
        <TabsTrigger active={tab === "library"} onClick={() => setTab("library")}>
          {t("evolution.library")}
        </TabsTrigger>
      </TabsList>

      {tab === "optimize" ? (
        <div className="grid gap-4">
          <EvolutionPanel
            expanded={expanded}
            currentTask={currentTask}
            popSize={popSize}
            maxGenerations={maxGenerations}
            spawnMode={spawnMode}
            manualJson={manualJson}
            error={createError}
            submitting={createTask.isPending}
            canceling={cancelTask.isPending}
            onExpand={() => setExpanded(true)}
            onSubmit={submitTask}
            onPopSize={setPopSize}
            onMaxGenerations={setMaxGenerations}
            onSpawnMode={setSpawnMode}
            onManualJson={setManualJson}
            onCancel={() => setConfirmCancel(true)}
          />
          <TaskQueueView tasks={allTasks} />
          <ChampionCard champion={champion} selectedInstance={selectedInstance} />
        </div>
      ) : (
        <GenomeLibrary genomes={genomes} onPromote={setPromoteGene} />
      )}

      {confirmCancel && currentTask && (
        <ConfirmDialog
          title={t("evolution.confirmTerminate")}
          body={t("evolution.terminate")}
          confirmLabel={t("common.confirm")}
          loading={cancelTask.isPending}
          onCancel={() => setConfirmCancel(false)}
          onConfirm={() => cancelTask.mutate(currentTask.id)}
        />
      )}
      {promoteGene && (
        <ConfirmDialog
          title={t("evolution.confirmPromote")}
          body={t("evolution.promoteImpact")}
          confirmLabel={t("evolution.promote")}
          loading={promote.isPending}
          onCancel={() => setPromoteGene(null)}
          onConfirm={() => promote.mutate(promoteGene)}
        />
      )}
    </div>
  );
}

function InstanceSelector({
  instances,
  selected,
  onSelect
}: {
  instances: StrategyInstance[];
  selected: StrategyInstance | null;
  onSelect: (id: number) => void;
}) {
  const { t } = useI18n();
  return (
    <label className="flex items-center gap-2 text-xs text-slate-500">
      {t("evolution.instanceSelector")}
      <select
        className="h-10 min-w-56 rounded-md border border-white/[0.06] bg-slate-950/70 px-3 text-sm text-slate-200 outline-none focus:border-accent"
        value={selected?.id ?? ""}
        onChange={(event) => onSelect(Number(event.target.value))}
      >
        {instances.map((instance) => (
          <option key={instance.id} value={instance.id}>
            {instance.name} / {instance.symbol}
          </option>
        ))}
      </select>
    </label>
  );
}

function EvolutionPanel({
  expanded,
  currentTask,
  popSize,
  maxGenerations,
  spawnMode,
  manualJson,
  error,
  submitting,
  canceling,
  onExpand,
  onSubmit,
  onPopSize,
  onMaxGenerations,
  onSpawnMode,
  onManualJson,
  onCancel
}: {
  expanded: boolean;
  currentTask: EvolutionTask | null;
  popSize: string;
  maxGenerations: string;
  spawnMode: SpawnMode;
  manualJson: string;
  error: string | null;
  submitting: boolean;
  canceling: boolean;
  onExpand: () => void;
  onSubmit: (event: FormEvent) => void;
  onPopSize: (value: string) => void;
  onMaxGenerations: (value: string) => void;
  onSpawnMode: (value: SpawnMode) => void;
  onManualJson: (value: string) => void;
  onCancel: () => void;
}) {
  const { t } = useI18n();
  const max = Number(configValue(currentTask?.config, "max_generations") ?? 0);
  const generation = Number(currentTask?.current_generation ?? 0);

  return (
    <Card className="bg-slate-900/30">
      <CardHeader>
        <div>
          <CardTitle>{currentTask ? t("evolution.runningTask") : t("evolution.startNew")}</CardTitle>
          <CardDescription>{currentTask ? `${generation} / ${max || "-"}` : t("evolution.inheritBest")}</CardDescription>
        </div>
        <FlaskConical className="h-4 w-4 text-accent" />
      </CardHeader>
      <CardContent>
        {currentTask ? (
          <div className="space-y-5">
            <div>
              <div className="mb-2 flex items-center justify-between text-xs text-slate-500">
                <span>{t("evolution.currentGeneration")}</span>
                <span className="qs-number">{max > 0 ? Math.round((generation / max) * 100) : currentTask.progress}%</span>
              </div>
              <Progress value={max > 0 ? (generation / max) * 100 : currentTask.progress} />
            </div>
            <div className="grid gap-3 md:grid-cols-3">
              <Metric label={t("evolution.bestScore")} value={Number(currentTask.best_score ?? 0).toFixed(4)} />
              <Metric label={t("common.maxDrawdown")} value={formatPercent(0)} danger />
              <Metric label={t("common.status")} value={t("status.running")} />
            </div>
            <Button variant="danger" disabled={canceling} onClick={onCancel}>
              <Square className="h-4 w-4" />
              {t("evolution.terminate")}
            </Button>
          </div>
        ) : expanded ? (
          <form className="grid gap-4 md:grid-cols-2" onSubmit={onSubmit}>
            <Field label={t("evolution.population")}>
              <Input type="number" min="10" max="500" value={popSize} onChange={(event) => onPopSize(event.target.value)} />
            </Field>
            <Field label={t("evolution.maxGenerations")}>
              <Input type="number" min="5" max="50" value={maxGenerations} onChange={(event) => onMaxGenerations(event.target.value)} />
            </Field>
            <div className="space-y-2 md:col-span-2">
              <Label className="text-xs uppercase tracking-wider text-slate-400">{t("evolution.inheritMode")}</Label>
              <div className="flex flex-wrap gap-2">
                {[
                  ["inherit", t("evolution.inheritBest")],
                  ["random_once", t("evolution.randomExplore")],
                  ["manual", t("evolution.manualSpecify")]
                ].map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    className={`h-9 rounded-md border px-3 text-xs font-medium ${
                      spawnMode === value ? "border-accent bg-accent/10 text-accent" : "border-white/[0.06] text-slate-500"
                    }`}
                    onClick={() => onSpawnMode(value as SpawnMode)}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>
            {spawnMode === "manual" && (
              <div className="space-y-2 md:col-span-2">
                <Label className="text-xs uppercase tracking-wider text-slate-400">{t("evolution.manualJson")}</Label>
                <textarea
                  className="min-h-36 w-full rounded-md border border-slate-700 bg-slate-900/80 p-3 font-mono text-xs text-slate-100 outline-none focus:border-accent"
                  value={manualJson}
                  onChange={(event) => onManualJson(event.target.value)}
                  placeholder='{"symbol":"BTCUSDT","base_interval":"1h"}'
                />
              </div>
            )}
            {error && <p className="md:col-span-2 rounded-lg border border-red-400/20 bg-red-400/10 p-3 text-xs text-red-300">{error}</p>}
            <div className="md:col-span-2">
              <Button variant="primary" type="submit" disabled={submitting}>
                <Play className="h-4 w-4" />
                {submitting ? t("common.loading") : t("evolution.submitTask")}
              </Button>
            </div>
          </form>
        ) : (
          <Button variant="primary" onClick={onExpand}>
            <Play className="h-4 w-4" />
            {t("evolution.startNew")}
          </Button>
        )}
      </CardContent>
    </Card>
  );
}

function TaskQueueView({ tasks }: { tasks: EvolutionTask[] }) {
  const { t } = useI18n();
  return (
    <Card className="bg-slate-900/30">
      <CardHeader>
        <div>
          <CardTitle>{t("evolution.taskQueue")}</CardTitle>
          <CardDescription>{tasks.length ? String(tasks.length) : t("common.empty")}</CardDescription>
        </div>
      </CardHeader>
      <CardContent className="grid gap-3">
        {tasks.length === 0 ? (
          <p className="text-sm text-slate-500">{t("common.empty")}</p>
        ) : (
          tasks.map((task) => (
            <div key={task.id} className="grid gap-3 rounded-lg border border-white/[0.04] bg-slate-950/35 p-3 md:grid-cols-4 md:items-center">
              <StatusBadge status={task.status} />
              <Metric label={t("common.score")} value={Number(task.best_score ?? 0).toFixed(4)} compact />
              <Metric label={t("common.duration")} value={taskDuration(task)} compact />
              <Metric label={t("evolution.currentGeneration")} value={String(task.current_generation ?? 0)} compact />
            </div>
          ))
        )}
      </CardContent>
    </Card>
  );
}

function ChampionCard({ champion, selectedInstance }: { champion: GeneRecord | null; selectedInstance: StrategyInstance | null }) {
  const { t } = useI18n();
  return (
    <Card className="border-accent/40 bg-accent/[0.04]">
      <CardHeader>
        <div>
          <CardTitle>{t("evolution.championMetrics")}</CardTitle>
          <CardDescription>{t("common.currentBest")}</CardDescription>
        </div>
      </CardHeader>
      <CardContent>
        {champion ? (
          <div className="grid gap-3 md:grid-cols-4">
            <Metric label={t("common.score")} value={Number(champion.score_total ?? 0).toFixed(4)} />
            <Metric label={t("backtesting.window6m")} value={scoreWindow(champion, "6m")} />
            <Metric label={t("backtesting.window2y")} value={scoreWindow(champion, "2y")} />
            <Metric label={t("common.maxDrawdown")} value={formatPercent(toNumber(champion.max_drawdown))} danger />
            <Link
              className="inline-flex h-10 items-center justify-center rounded-md bg-accent px-4 text-sm font-semibold text-slate-950 md:col-span-4"
              to={`/?instance=${selectedInstance?.id ?? ""}`}
            >
              {t("evolution.applyInstance")}
            </Link>
          </div>
        ) : (
          <p className="text-sm text-slate-500">{t("common.empty")}</p>
        )}
      </CardContent>
    </Card>
  );
}

function GenomeLibrary({ genomes, onPromote }: { genomes: GeneRecord[]; onPromote: (gene: GeneRecord) => void }) {
  const { t } = useI18n();
  return (
    <div className="grid gap-3">
      {genomes.length === 0 ? (
        <Card>
          <CardContent className="p-8 text-center text-sm text-slate-500">{t("common.empty")}</CardContent>
        </Card>
      ) : (
        genomes
          .slice()
          .sort((a, b) => new Date(b.created_at ?? 0).getTime() - new Date(a.created_at ?? 0).getTime())
          .map((gene) => (
            <Card key={gene.id} className={gene.role === "champion" ? "border-accent/40 bg-accent/[0.04]" : "bg-slate-900/30"}>
              <CardContent className="grid gap-3 p-4 md:grid-cols-[0.9fr_1fr_1fr_auto] md:items-center">
                <div className="space-y-2">
                  <StatusBadge status={gene.role === "champion" ? "succeeded" : "queued"} label={roleLabel(gene.role, t)} />
                  <p className="qs-number text-xs text-slate-500">{formatDateTime(gene.created_at)}</p>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <Metric label={t("common.score")} value={Number(gene.score_total ?? 0).toFixed(4)} compact />
                  <Metric label={t("common.maxDrawdown")} value={formatPercent(toNumber(gene.max_drawdown))} danger compact />
                </div>
                <div className="grid grid-cols-4 gap-2">
                  <Metric label={t("backtesting.window6m")} value={scoreWindow(gene, "6m")} compact />
                  <Metric label={t("backtesting.window2y")} value={scoreWindow(gene, "2y")} compact />
                  <Metric label={t("backtesting.window5y")} value={scoreWindow(gene, "5y")} compact />
                  <Metric label={t("backtesting.windowAll")} value={scoreWindow(gene, "all")} compact />
                </div>
                <div className="flex flex-wrap gap-2 md:justify-end">
                  {gene.role === "challenger" && (
                    <Button size="sm" variant="primary" onClick={() => onPromote(gene)}>
                      <Check className="h-3.5 w-3.5" />
                      {t("evolution.promote")}
                    </Button>
                  )}
                  <Link
                    className="inline-flex h-8 items-center rounded-md border border-white/[0.06] bg-white/[0.04] px-3 text-xs text-slate-300"
                    to={`/backtesting?genome=${gene.id}`}
                  >
                    {t("evolution.fullBacktest")}
                  </Link>
                </div>
              </CardContent>
            </Card>
          ))
      )}
    </div>
  );
}

function ConfirmDialog({
  title,
  body,
  confirmLabel,
  loading,
  onCancel,
  onConfirm
}: {
  title: string;
  body: string;
  confirmLabel: string;
  loading: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useI18n();
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 p-4 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-xl border border-white/[0.06] bg-slate-950 p-5">
        <h2 className="text-base font-semibold text-slate-200">{title}</h2>
        <p className="mt-3 text-sm leading-6 text-slate-400">{body}</p>
        <div className="mt-5 flex justify-end gap-2">
          <Button variant="ghost" disabled={loading} onClick={onCancel}>
            {t("common.cancel")}
          </Button>
          <Button variant="primary" disabled={loading} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
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

function Metric({ label, value, danger, compact }: { label: string; value: string; danger?: boolean; compact?: boolean }) {
  return (
    <div className={compact ? "" : "rounded-lg border border-white/[0.04] bg-slate-950/35 p-3"}>
      <p className="text-[11px] text-slate-500">{label}</p>
      <p className={`qs-number mt-1 truncate text-sm ${danger ? "text-red-300" : "text-slate-200"}`}>{value}</p>
    </div>
  );
}

function compactTasks(...groups: Array<EvolutionTask | EvolutionTask[] | null | undefined>) {
  const byID = new Map<number, EvolutionTask>();
  for (const group of groups) {
    if (!group) continue;
    const tasks = Array.isArray(group) ? group : [group];
    for (const task of tasks) byID.set(task.id, task);
  }
  return Array.from(byID.values()).sort((a, b) => b.id - a.id);
}

function configValue(config: Record<string, unknown> | undefined, key: string) {
  if (!config || typeof config !== "object") return undefined;
  return config[key];
}

function taskDuration(task: EvolutionTask) {
  const start = task.started_at ? new Date(task.started_at).getTime() : new Date(task.created_at ?? 0).getTime();
  const end = task.finished_at ? new Date(task.finished_at).getTime() : Date.now();
  if (!Number.isFinite(start) || start <= 0) return "-";
  const seconds = Math.max(0, Math.round((end - start) / 1000));
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}

function scoreWindow(gene: GeneRecord, key: string) {
  const report = gene.score_report ?? {};
  const value = report[key] ?? report[`${key}_score`] ?? report[`score_${key}`];
  return typeof value === "number" ? value.toFixed(4) : typeof value === "string" ? value : "-";
}

function roleLabel(role: string, t: (key: string) => string) {
  if (role === "champion") return t("common.currentBest");
  if (role === "retired") return t("common.archive");
  return t("common.candidate");
}

function resolveTaskIdForGene(gene: GeneRecord, tasks: EvolutionTask[]) {
  const matching = tasks.find((task) => String(task.best_gene_id) === String(gene.id));
  return matching?.id ?? gene.id;
}
