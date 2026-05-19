import { useQuery } from "@tanstack/react-query";
import { KeyRound, RadioTower, Shield } from "lucide-react";
import { api } from "../lib/api";
import { PageHeader } from "../components/layout/PageHeader";
import { Badge } from "../components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { useAuthStore } from "../stores/authStore";

export function AgentsPage() {
  const token = useAuthStore((state) => state.token);
  const query = useQuery({
    queryKey: ["agent-status"],
    queryFn: () => api.getAgentStatus(token ?? ""),
    enabled: Boolean(token),
    refetchInterval: 30_000
  });
  const status = query.data;

  return (
    <div>
      <PageHeader
        title="Agent 管理"
        description="Agent 是本地执行端，只负责接收交易指令、调用交易所并上报事实结果。交易所密钥只保存在本地配置文件。"
      />
      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <div>
              <CardTitle>连接状态</CardTitle>
              <CardDescription>状态由 SaaS WebSocket Hub 返回，每 30 秒刷新。</CardDescription>
            </div>
            <Badge tone={status?.connected ? "success" : "warning"}>{status?.connected ? "在线" : "离线"}</Badge>
          </CardHeader>
          <CardContent className="grid gap-3 md:grid-cols-3">
            <InfoBlock icon={<RadioTower className="h-4 w-4" />} label="Agent 标识" value={status?.agent_id ?? "user 当前用户"} />
            <InfoBlock icon={<Shield className="h-4 w-4" />} label="执行边界" value="本地执行" />
            <InfoBlock icon={<KeyRound className="h-4 w-4" />} label="密钥位置" value="config.agent.yaml" />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <div>
              <CardTitle>配置入口</CardTitle>
              <CardDescription>前端只提示配置位置，不接收、不保存、不上传交易所密钥。</CardDescription>
            </div>
          </CardHeader>
          <CardContent>
            <div className="rounded-lg border border-warm/20 bg-warm/10 p-4 text-sm leading-6 text-warm">
              请在本地 Agent 侧配置交易所凭证，SaaS 和浏览器页面不应出现任何 API Key 或 Secret。
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function InfoBlock({ icon, label, value }: { icon: JSX.Element; label: string; value: string }) {
  return (
    <div className="rounded-lg border border-white/[0.04] bg-white/[0.025] p-4">
      <div className="flex items-center gap-2 text-text-weak">
        {icon}
        <span className="text-xs">{label}</span>
      </div>
      <p className="mt-3 truncate text-sm font-medium text-text-main">{value}</p>
    </div>
  );
}
