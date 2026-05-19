import type { StrategyInstanceStatus } from "../types/api";

export function instanceStatusLabel(status?: StrategyInstanceStatus | string) {
  switch (status) {
    case "RUNNING":
      return "运行中";
    case "STOPPED":
      return "已暂停";
    case "ERROR":
      return "异常";
    case "DELETED":
      return "已删除";
    default:
      return "未知";
  }
}

export function executionStatusLabel(status?: string) {
  switch ((status ?? "").toLowerCase()) {
    case "pending":
      return "等待回报";
    case "filled":
      return "已成交";
    case "failed":
      return "执行失败";
    default:
      return "暂无状态";
  }
}

export function geneRoleLabel(role?: string) {
  switch (role) {
    case "challenger":
      return "候选参数";
    case "champion":
      return "当前最优参数";
    case "retired":
      return "历史参数";
    default:
      return "参数记录";
  }
}

export function actionLabel(action?: string) {
  switch ((action ?? "").toUpperCase()) {
    case "BUY":
      return "买入";
    case "SELL":
      return "卖出";
    default:
      return "调整";
  }
}

export function engineLabel(engine?: string) {
  switch ((engine ?? "").toUpperCase()) {
    case "MACRO":
      return "长期配置";
    case "MICRO":
      return "活跃调仓";
    default:
      return "策略决策";
  }
}

export function businessStrategyName(name?: string) {
  const raw = (name ?? "").trim();
  if (!raw) return "动态平衡现货策略";
  if (raw.toLowerCase().includes("dynamic")) return "动态平衡现货策略";
  return raw
    .replaceAll("DCA", "定投")
    .replaceAll("Spot", "现货")
    .replaceAll("Sigmoid", "动态平衡");
}

export function strategyLabelFromID(strategyID?: string) {
  const raw = (strategyID ?? "").trim();
  if (!raw) return "动态平衡现货策略";
  if (raw.includes("sigmoid") || raw.includes("dca") || raw.includes("example")) {
    return "动态平衡现货策略";
  }
  return businessStrategyName(raw.replaceAll("_", " "));
}

export function businessStrategyDescription(description?: string) {
  const raw = (description ?? "").trim();
  if (!raw) {
    return "面向现货资产的长期配置与活跃调仓组合，强调资金复利和风险约束。";
  }
  return raw
    .replaceAll("DCA", "定投")
    .replaceAll("Sigmoid", "动态平衡")
    .replaceAll("micro", "活跃")
    .replaceAll("macro", "长期")
    .replace(/deadbtc/gi, "长期持仓")
    .replace(/floatbtc/gi, "活跃仓位")
    .replace(/coldsealedbtc/gi, "封存资产")
    .replaceAll("semantic", "资产语义");
}
