import { useQuery } from "@tanstack/react-query";
import { Boxes, Cpu, FileText } from "lucide-react";
import { api } from "../lib/api";
import { businessStrategyDescription, businessStrategyName } from "../lib/terminology";
import { PageHeader } from "../components/layout/PageHeader";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Badge } from "../components/ui/badge";
import { EmptyState } from "../components/ui/empty-state";
import { useAuthStore } from "../stores/authStore";

export function TemplatesPage() {
  const token = useAuthStore((state) => state.token);
  const query = useQuery({
    queryKey: ["strategies"],
    queryFn: () => api.getStrategies(token ?? ""),
    enabled: Boolean(token)
  });
  const templates = query.data?.strategies ?? [];

  return (
    <div>
      <PageHeader
        title="策略模板目录"
        description="模板只展示面向用户的策略能力、资产类型和版本信息，内部参数结构由后端统一管理。"
      />
      {templates.length === 0 ? (
        <EmptyState title="暂无策略模板" description="后端加载模板元数据后，这里会展示可创建的策略模板。" />
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {templates.map((template, index) => {
            const manifest = template.manifest ?? {};
            const title = businessStrategyName(String(manifest.name ?? template.name));
            const description = businessStrategyDescription(String(manifest.description ?? ""));
            const tone = index % 3 === 0 ? "accent" : index % 3 === 1 ? "info" : "warm";
            return (
              <Card key={template.id} className="overflow-hidden">
                <div className={`h-1 ${tone === "accent" ? "bg-accent" : tone === "info" ? "bg-info" : "bg-warm"}`} />
                <CardHeader>
                  <div className="flex min-w-0 items-start gap-3">
                    <div
                      className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border ${
                        tone === "accent"
                          ? "border-accent/20 bg-accent/10 text-accent"
                          : tone === "info"
                            ? "border-info/20 bg-info/10 text-info"
                            : "border-warm/20 bg-warm/10 text-warm"
                      }`}
                    >
                      <Cpu className="h-5 w-5" />
                    </div>
                    <div className="min-w-0">
                      <CardTitle className="truncate">{title}</CardTitle>
                      <CardDescription>版本 {template.version}</CardDescription>
                    </div>
                  </div>
                  <Badge tone={template.is_spot ? "success" : "neutral"}>{template.is_spot ? "现货" : "资产组合"}</Badge>
                </CardHeader>
                <CardContent>
                  <p className="min-h-16 text-sm leading-6 text-text-muted">{description}</p>
                  <div className="mt-5 grid grid-cols-2 gap-3">
                    <TemplateFact icon={<Boxes className="h-4 w-4" />} label="资金模式" value="复利滚动" />
                    <TemplateFact icon={<FileText className="h-4 w-4" />} label="配置来源" value="当前最优参数" />
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}

function TemplateFact({ icon, label, value }: { icon: JSX.Element; label: string; value: string }) {
  return (
    <div className="rounded-md border border-white/[0.04] bg-white/[0.025] p-3">
      <div className="flex items-center gap-2 text-text-weak">
        {icon}
        <span className="text-[11px]">{label}</span>
      </div>
      <p className="mt-2 text-sm font-medium text-text-main">{value}</p>
    </div>
  );
}
