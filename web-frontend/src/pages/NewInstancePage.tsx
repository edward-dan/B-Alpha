import { FormEvent, type ReactNode, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, CheckCircle2 } from "lucide-react";
import { api, ApiRequestError } from "../lib/api";
import { businessStrategyName } from "../lib/terminology";
import { PageHeader } from "../components/layout/PageHeader";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { useAuthStore } from "../stores/authStore";

export function NewInstancePage() {
  const token = useAuthStore((state) => state.token);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [templateId, setTemplateId] = useState("");
  const [name, setName] = useState("");
  const [symbol, setSymbol] = useState("BTCUSDT");
  const [interval, setInterval] = useState("1h");
  const [agentId, setAgentId] = useState("");
  const [initialUSDT, setInitialUSDT] = useState("10000");
  const [longHolding, setLongHolding] = useState("0");
  const [activeHolding, setActiveHolding] = useState("0");
  const [sealedHolding, setSealedHolding] = useState("0");
  const [costPrice, setCostPrice] = useState("65000");

  const templatesQuery = useQuery({
    queryKey: ["strategies"],
    queryFn: () => api.getStrategies(token ?? ""),
    enabled: Boolean(token)
  });
  const templates = templatesQuery.data?.strategies ?? [];
  const selectedTemplate = useMemo(
    () => templates.find((template) => String(template.id) === templateId),
    [templateId, templates]
  );

  const mutation = useMutation({
    mutationFn: () =>
      api.createInstance(token ?? "", {
        template_id: Number(templateId),
        name,
        symbol,
        interval,
        config: {
          agent_id: agentId.trim() || undefined,
          interval,
          quote_asset: "USDT",
          market_lookback: 240,
          command_ttl_seconds: 120
        },
        initial_portfolio: {
          usdt_balance: Number(initialUSDT),
          dead_btc: Number(longHolding),
          float_btc: Number(activeHolding),
          cold_sealed_btc: Number(sealedHolding),
          total_equity:
            Number(initialUSDT) +
            (Number(longHolding) + Number(activeHolding) + Number(sealedHolding)) * Number(costPrice)
        },
        initial_cost_price: Number(costPrice)
      }),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["instances"] });
      queryClient.invalidateQueries({ queryKey: ["dashboard"] });
      navigate(`/?instance=${data.instance.id}`);
    }
  });

  function onSubmit(event: FormEvent) {
    event.preventDefault();
    mutation.mutate();
  }

  const error = mutation.error instanceof ApiRequestError ? mutation.error.message : null;

  return (
    <div>
      <PageHeader
        title="创建实例"
        description="选择策略模板并填写初始资金配额。API Key 不会出现在这里，只能保存在本地 Agent 配置文件。"
        action={
          <Button variant="ghost" icon={<ArrowLeft className="h-4 w-4" />} onClick={() => navigate("/instances")}>
            返回
          </Button>
        }
      />
      <form className="grid gap-4 xl:grid-cols-[1.3fr_0.7fr]" onSubmit={onSubmit}>
        <Card>
          <CardHeader>
            <div>
              <CardTitle>实例配置</CardTitle>
              <CardDescription>创建后默认处于已暂停状态，可在实例列表中启动。</CardDescription>
            </div>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            <Field label="策略模板">
              <select
                className="h-10 w-full rounded-md border border-white/[0.06] bg-slate-950/60 px-3 text-sm text-text-main outline-none focus:border-accent/40"
                value={templateId}
                onChange={(event) => setTemplateId(event.target.value)}
                required
              >
                <option value="">选择模板</option>
                {templates.map((template) => (
                  <option key={template.id} value={template.id}>
                    {businessStrategyName(template.name)} v{template.version}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="实例名称">
              <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="BTC 长期配置" />
            </Field>
            <Field label="交易对">
              <Input value={symbol} onChange={(event) => setSymbol(event.target.value.toUpperCase())} required />
            </Field>
            <Field label="策略周期">
              <Input value={interval} onChange={(event) => setInterval(event.target.value)} required />
            </Field>
            <Field label="Agent 标识">
              <Input value={agentId} onChange={(event) => setAgentId(event.target.value)} placeholder="默认使用当前用户" />
            </Field>
            <Field label="参考成本价">
              <Input type="number" min="0" step="0.01" value={costPrice} onChange={(event) => setCostPrice(event.target.value)} />
            </Field>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div>
              <CardTitle>资金配额</CardTitle>
              <CardDescription>初始资产用于建立 SaaS 侧语义账本。</CardDescription>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <Field label="可用资金 USDT">
              <Input type="number" min="0" step="0.01" value={initialUSDT} onChange={(event) => setInitialUSDT(event.target.value)} />
            </Field>
            <Field label="长期持仓 BTC">
              <Input type="number" min="0" step="0.00000001" value={longHolding} onChange={(event) => setLongHolding(event.target.value)} />
            </Field>
            <Field label="活跃仓位 BTC">
              <Input type="number" min="0" step="0.00000001" value={activeHolding} onChange={(event) => setActiveHolding(event.target.value)} />
            </Field>
            <Field label="封存资产 BTC">
              <Input type="number" min="0" step="0.00000001" value={sealedHolding} onChange={(event) => setSealedHolding(event.target.value)} />
            </Field>
            {selectedTemplate && (
              <div className="rounded-md border border-accent/10 bg-accent/[0.04] p-3 text-xs leading-5 text-accent">
                已选择 {businessStrategyName(selectedTemplate.name)}，创建后可在 Dashboard 观察资产旅程。
              </div>
            )}
            {error && <div className="rounded-md border border-danger/20 bg-danger/10 p-3 text-xs text-danger">{error}</div>}
            <Button className="w-full" variant="primary" type="submit" disabled={mutation.isPending || !templateId}>
              <CheckCircle2 className="h-4 w-4" />
              {mutation.isPending ? "创建中" : "创建实例"}
            </Button>
          </CardContent>
        </Card>
      </form>
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
