import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BarChart3, FlaskConical, Plus, Trash2 } from "lucide-react";
import { Button } from "../../components/ui/button";
import { useI18n } from "../../i18n/useI18n";
import { formatCurrency, formatRelativeTime } from "../../lib/format";
import { Card, CardContent } from "../../shared/ui/Card";
import { StatusBadge } from "../../shared/ui/StatusBadge";
import { TableSkeleton } from "../../shared/ui/skeletons";
import { instancesService } from "../../shared/services";
import type { StrategyInstance } from "../../types/api";
import { catalogItemForInstance } from "./strategyCatalog";

export function InstanceListPage() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [confirmingId, setConfirmingId] = useState<number | null>(null);

  const instancesQuery = useQuery({
    queryKey: ["instances"],
    queryFn: instancesService.list,
    refetchInterval: 60_000
  });
  const instances = instancesQuery.data?.instances ?? [];

  const remove = useMutation({
    mutationFn: instancesService.remove,
    onSuccess: () => {
      setConfirmingId(null);
      queryClient.invalidateQueries({ queryKey: ["instances"] });
    }
  });

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold tracking-wider text-slate-200">{t("instances.title")}</h1>
        <Link
          to="/instances/new"
          className="inline-flex h-10 items-center gap-2 rounded-md bg-accent px-4 text-sm font-semibold uppercase tracking-wider text-slate-950 hover:bg-accent/90"
        >
          <Plus className="h-4 w-4" />
          {t("instances.createNew")}
        </Link>
      </div>
      {instancesQuery.isLoading ? (
        <TableSkeleton rows={4} />
      ) : instances.length === 0 ? (
        <Card>
          <CardContent className="p-8 text-center text-sm text-slate-500">{t("dashboard.noInstancesHint")}</CardContent>
        </Card>
      ) : (
        <div className="grid gap-3">
          {instances.map((instance) => (
            <InstanceRow
              key={instance.id}
              instance={instance}
              confirming={confirmingId === instance.id}
              deleting={remove.isPending}
              onAskDelete={() => setConfirmingId(instance.id)}
              onCancelDelete={() => setConfirmingId(null)}
              onDelete={() => remove.mutate(instance.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function InstanceRow({
  instance,
  confirming,
  deleting,
  onAskDelete,
  onCancelDelete,
  onDelete
}: {
  instance: StrategyInstance;
  confirming: boolean;
  deleting: boolean;
  onAskDelete: () => void;
  onCancelDelete: () => void;
  onDelete: () => void;
}) {
  const { t } = useI18n();
  const catalog = catalogItemForInstance(instance);

  return (
    <Card className="bg-slate-900/30">
      <CardContent className="grid gap-4 p-4 lg:grid-cols-[minmax(0,1.4fr)_0.8fr_0.8fr_auto] lg:items-center">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="truncate text-sm font-semibold text-slate-200">{instance.name}</h2>
            <StatusBadge status={normalizeInstanceStatus(instance.status)} />
          </div>
          <p className="mt-1 text-xs text-slate-500">
            {catalog.name} / {instance.symbol}
          </p>
        </div>
        <div>
          <p className="text-xs text-slate-500">{t("common.totalAssets")}</p>
          <p className="qs-number mt-1 text-sm text-slate-200">{formatCurrency(instance.portfolio?.total_equity)}</p>
        </div>
        <div>
          <p className="text-xs text-slate-500">{t("common.createdAt")}</p>
          <p className="mt-1 text-sm text-slate-300">{formatRelativeTime(instance.created_at)}</p>
        </div>
        <div className="flex flex-wrap gap-2 lg:justify-end">
          <Link
            to={`/?instance=${instance.id}`}
            className="inline-flex h-9 items-center gap-2 rounded-md border border-white/[0.06] bg-white/[0.04] px-3 text-xs font-medium text-slate-200 hover:bg-white/[0.07]"
          >
            <BarChart3 className="h-3.5 w-3.5" />
            {t("instances.goDashboard")}
          </Link>
          <Link
            to={`/evolution?instance=${instance.id}`}
            className="inline-flex h-9 items-center gap-2 rounded-md border border-white/[0.06] bg-white/[0.04] px-3 text-xs font-medium text-slate-200 hover:bg-white/[0.07]"
          >
            <FlaskConical className="h-3.5 w-3.5" />
            {t("instances.goEvolution")}
          </Link>
          {confirming ? (
            <>
              <Button size="sm" variant="danger" disabled={deleting} onClick={onDelete}>
                {t("instances.confirmDelete")}
              </Button>
              <Button size="sm" variant="ghost" disabled={deleting} onClick={onCancelDelete}>
                {t("common.cancel")}
              </Button>
            </>
          ) : (
            <Button
              title={t("common.delete")}
              aria-label={t("common.delete")}
              size="icon"
              variant="danger"
              icon={<Trash2 className="h-4 w-4" />}
              onClick={onAskDelete}
            />
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function normalizeInstanceStatus(status?: string) {
  if (status === "RUNNING") return "running";
  if (status === "ERROR") return "error";
  return "stopped";
}
