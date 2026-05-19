import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Play, Plus, Square, Trash2 } from "lucide-react";
import { api } from "../lib/api";
import { businessStrategyName, instanceStatusLabel } from "../lib/terminology";
import { formatCurrency, formatDateTime } from "../lib/format";
import { PageHeader } from "../components/layout/PageHeader";
import { Button } from "../components/ui/button";
import { Badge } from "../components/ui/badge";
import { Card, CardContent } from "../components/ui/card";
import { EmptyState } from "../components/ui/empty-state";
import { useAuthStore } from "../stores/authStore";

export function InstancesPage() {
  const token = useAuthStore((state) => state.token);
  const queryClient = useQueryClient();
  const instancesQuery = useQuery({
    queryKey: ["instances"],
    queryFn: () => api.getInstances(token ?? ""),
    enabled: Boolean(token)
  });
  const instances = instancesQuery.data?.instances ?? [];

  const transition = useMutation({
    mutationFn: ({ id, action }: { id: number; action: "start" | "stop" | "delete" }) => {
      if (action === "start") return api.startInstance(token ?? "", id);
      if (action === "stop") return api.stopInstance(token ?? "", id);
      return api.deleteInstance(token ?? "", id);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["instances"] });
      queryClient.invalidateQueries({ queryKey: ["dashboard"] });
    }
  });

  return (
    <div>
      <PageHeader
        title="实例列表"
        description="管理策略实例的运行、暂停与删除。删除只会通过后端实例接口处理，不在前端直接改写数据。"
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
      {instances.length === 0 ? (
        <EmptyState title="暂无实例" description="创建实例后，可在这里查看全部状态并执行运行管理。" />
      ) : (
        <div className="grid gap-4">
          {instances.map((instance) => (
            <Card key={instance.id}>
              <CardContent className="flex flex-col gap-4 pt-4 lg:flex-row lg:items-center lg:justify-between">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="truncate text-base font-semibold text-text-main">{instance.name}</h2>
                    <Badge tone={instance.status === "RUNNING" ? "success" : instance.status === "ERROR" ? "danger" : "neutral"}>
                      {instanceStatusLabel(instance.status)}
                    </Badge>
                  </div>
                  <p className="mt-2 text-sm text-text-muted">
                    {businessStrategyName(instance.template?.name)} · {instance.symbol} · {instance.interval}
                  </p>
                </div>
                <div className="grid gap-3 sm:grid-cols-3 lg:min-w-[520px]">
                  <InstanceFact label="总资产" value={formatCurrency(instance.portfolio?.total_equity)} />
                  <InstanceFact label="最近决策" value={formatDateTime(instance.portfolio?.last_processed_bar_time)} />
                  <InstanceFact label="更新于" value={formatDateTime(instance.updated_at)} />
                </div>
                <div className="flex gap-2">
                  {instance.status === "RUNNING" ? (
                    <Button
                      title="暂停"
                      aria-label="暂停"
                      variant="secondary"
                      size="icon"
                      icon={<Square className="h-4 w-4" />}
                      disabled={transition.isPending}
                      onClick={() => transition.mutate({ id: instance.id, action: "stop" })}
                    />
                  ) : (
                    <Button
                      title="运行"
                      aria-label="运行"
                      variant="primary"
                      size="icon"
                      icon={<Play className="h-4 w-4" />}
                      disabled={transition.isPending}
                      onClick={() => transition.mutate({ id: instance.id, action: "start" })}
                    />
                  )}
                  <Button
                    title="删除"
                    aria-label="删除"
                    variant="danger"
                    size="icon"
                    icon={<Trash2 className="h-4 w-4" />}
                    disabled={transition.isPending}
                    onClick={() => transition.mutate({ id: instance.id, action: "delete" })}
                  />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

function InstanceFact({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md bg-white/[0.025] p-3">
      <p className="text-[11px] text-text-weak">{label}</p>
      <p className="qs-number mt-1 truncate text-sm text-text-main">{value}</p>
    </div>
  );
}
