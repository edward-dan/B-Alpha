import type { StrategyInstance } from "../../types/api";

export type StrategyCatalogItem = {
  id: number;
  strategyId: string;
  name: string;
  description: string;
  color: string;
  exchanges: string[];
  symbols: string[];
  features: {
    evolution: boolean;
    spot: boolean;
  };
};

export const strategyCatalog: StrategyCatalogItem[] = [
  {
    id: 1,
    strategyId: "example_sigmoid_dca_v1",
    name: "动态均衡策略",
    description: "适合长期现货资产配置，在纪律化投入与风险约束之间保持平衡。",
    color: "#2dd4bf",
    exchanges: ["Bitget", "Binance"],
    symbols: ["BTCUSDT"],
    features: { evolution: true, spot: true }
  }
];

export function findCatalogItem(templateId?: number | null, strategyId?: string | null) {
  return (
    strategyCatalog.find((item) => item.id === templateId) ??
    strategyCatalog.find((item) => item.strategyId === strategyId) ??
    strategyCatalog[0]
  );
}

export function catalogItemForInstance(instance: StrategyInstance | null | undefined) {
  if (!instance) return strategyCatalog[0];
  const manifestID = typeof instance.template?.manifest?.id === "string" ? instance.template.manifest.id : null;
  return findCatalogItem(instance.template_id, manifestID);
}
