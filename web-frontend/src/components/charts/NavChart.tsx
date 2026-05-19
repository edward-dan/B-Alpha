import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis
} from "recharts";
import { formatCurrency } from "../../lib/format";

type NavPoint = {
  label: string;
  total: number;
  active: number;
};

export function NavChart({ data }: { data: NavPoint[] }) {
  return (
    <div className="h-72 w-full">
      <ResponsiveContainer>
        <AreaChart data={data} margin={{ left: 0, right: 8, top: 10, bottom: 0 }}>
          <defs>
            <linearGradient id="totalFill" x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor="#2dd4bf" stopOpacity={0.28} />
              <stop offset="100%" stopColor="#2dd4bf" stopOpacity={0} />
            </linearGradient>
            <linearGradient id="activeFill" x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor="#0ea5e9" stopOpacity={0.2} />
              <stop offset="100%" stopColor="#0ea5e9" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="#1e293b" strokeDasharray="3 3" vertical={false} />
          <XAxis dataKey="label" tickLine={false} axisLine={false} tick={{ fill: "#64748b", fontSize: 11 }} />
          <YAxis
            width={70}
            tickLine={false}
            axisLine={false}
            tick={{ fill: "#64748b", fontSize: 11 }}
            tickFormatter={(value) => `${Number(value / 1000).toFixed(0)}k`}
          />
          <Tooltip
            cursor={{ stroke: "#2dd4bf", strokeOpacity: 0.25 }}
            contentStyle={{
              background: "rgba(15, 23, 42, 0.92)",
              border: "1px solid rgba(255,255,255,0.06)",
              borderRadius: 8,
              color: "#e2e8f0"
            }}
            formatter={(value, name) => [
              formatCurrency(Number(value)),
              name === "total" ? "总资产" : "活跃仓位"
            ]}
            labelStyle={{ color: "#94a3b8" }}
          />
          <Area
            type="monotone"
            dataKey="total"
            stroke="#2dd4bf"
            strokeWidth={2}
            fill="url(#totalFill)"
            name="total"
          />
          <Area
            type="monotone"
            dataKey="active"
            stroke="#0ea5e9"
            strokeWidth={1.5}
            fill="url(#activeFill)"
            name="active"
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}
