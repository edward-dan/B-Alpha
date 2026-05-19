export function toNumber(value: string | number | null | undefined): number {
  if (value == null || value === "") return 0;
  if (typeof value === "number") return Number.isFinite(value) ? value : 0;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function formatCurrency(value: string | number | null | undefined, currency = "USDT") {
  const amount = toNumber(value);
  return `${amount.toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })} ${currency}`;
}

export function formatPercent(value: string | number | null | undefined) {
  const amount = toNumber(value);
  return `${(amount * 100).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })}%`;
}

export function formatAsset(value: string | number | null | undefined, asset = "BTC") {
  const amount = toNumber(value);
  return `${amount.toLocaleString("zh-CN", {
    minimumFractionDigits: 6,
    maximumFractionDigits: 6
  })} ${asset}`;
}

export function formatDateTime(value?: string | number | null) {
  if (!value) return "暂无";
  const date = typeof value === "number" ? new Date(value) : new Date(value);
  if (Number.isNaN(date.getTime())) return "暂无";
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

export function compactNumber(value: string | number | null | undefined) {
  return toNumber(value).toLocaleString("zh-CN", {
    notation: "compact",
    maximumFractionDigits: 2
  });
}

export function formatRelativeTime(value?: string | number | null) {
  if (!value) return "暂无";
  const date = typeof value === "number" ? new Date(value) : new Date(value);
  if (Number.isNaN(date.getTime())) return "暂无";
  const diffMs = Date.now() - date.getTime();
  const absMs = Math.abs(diffMs);
  const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ["year", 365 * 24 * 60 * 60 * 1000],
    ["month", 30 * 24 * 60 * 60 * 1000],
    ["day", 24 * 60 * 60 * 1000],
    ["hour", 60 * 60 * 1000],
    ["minute", 60 * 1000]
  ];
  const formatter = new Intl.RelativeTimeFormat("zh-CN", { numeric: "auto" });
  for (const [unit, size] of units) {
    if (absMs >= size) {
      return formatter.format(Math.round(-diffMs / size), unit);
    }
  }
  return "刚刚";
}
