import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";
import { formatAsset, toNumber } from "../../lib/format";
import type { PortfolioState } from "../../types/api";

const colors = ["#2dd4bf", "#0ea5e9", "#ff8c6b"];

export function AllocationChart({ portfolio }: { portfolio?: PortfolioState | null }) {
  const data = [
    { name: "长期持仓", value: toNumber(portfolio?.dead_btc) },
    { name: "活跃仓位", value: toNumber(portfolio?.float_btc) },
    { name: "封存资产", value: toNumber(portfolio?.cold_sealed_btc) }
  ].filter((item) => item.value > 0);

  if (data.length === 0) {
    return <div className="flex h-44 items-center justify-center text-xs text-text-weak">暂无仓位数据</div>;
  }

  return (
    <div className="h-44 w-full">
      <ResponsiveContainer>
        <PieChart>
          <Pie data={data} dataKey="value" innerRadius={48} outerRadius={70} paddingAngle={3}>
            {data.map((item, index) => (
              <Cell key={item.name} fill={colors[index % colors.length]} />
            ))}
          </Pie>
          <Tooltip
            contentStyle={{
              background: "rgba(15, 23, 42, 0.92)",
              border: "1px solid rgba(255,255,255,0.06)",
              borderRadius: 8,
              color: "#e2e8f0"
            }}
            formatter={(value) => [formatAsset(Number(value)), "数量"]}
          />
        </PieChart>
      </ResponsiveContainer>
    </div>
  );
}
