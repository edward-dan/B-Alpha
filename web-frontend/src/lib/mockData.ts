import type { DashboardResponse, StrategyInstance } from "../types/api";

export const emptyDashboard: DashboardResponse = {
  overview: {
    user_id: 0,
    instances_total: 0,
    running_count: 0,
    stopped_count: 0,
    error_count: 0,
    total_equity: 0,
    usdt_balance: 0,
    dead_btc: 0,
    float_btc: 0,
    cold_sealed_btc: 0,
    pending_commands: 0
  },
  instances: [],
  recent_trades: []
};

export function buildNavSeries(instances: StrategyInstance[]) {
  const seed = instances.reduce((sum, item) => sum + Number(item.id || 0), 3);
  const base = instances.reduce((sum, item) => {
    const value = Number(item.portfolio?.total_equity ?? 0);
    return sum + (Number.isFinite(value) ? value : 0);
  }, 0);
  const start = base > 0 ? base * 0.92 : 10000;
  return Array.from({ length: 24 }, (_, index) => {
    const cycle = Math.sin((index + seed) / 3) * 0.018;
    const drift = index * 0.006;
    const value = start * (1 + cycle + drift);
    return {
      label: `${String(index + 1).padStart(2, "0")}:00`,
      total: Number(value.toFixed(2)),
      active: Number((value * (0.24 + Math.sin(index / 4) * 0.04)).toFixed(2))
    };
  });
}
