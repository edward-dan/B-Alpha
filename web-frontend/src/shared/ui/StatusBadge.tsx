import { useI18n } from "../../i18n/useI18n";
import { cn } from "../../lib/cn";

export type StatusBadgeStatus = "running" | "stopped" | "error" | "halted" | "queued" | "succeeded" | "failed" | "canceled";

type StatusBadgeProps = {
  status?: StatusBadgeStatus | string | null;
  label?: string;
  className?: string;
};

const toneClasses: Record<StatusBadgeStatus, string> = {
  running: "border-accent/20 bg-accent/10 text-accent before:bg-accent",
  stopped: "border-slate-500/20 bg-slate-500/10 text-slate-400 before:bg-slate-500",
  error: "border-red-400/20 bg-red-400/10 text-red-300 before:bg-red-400",
  halted: "border-red-400/20 bg-red-400/10 text-red-300 before:bg-red-400",
  queued: "border-sky-400/20 bg-sky-400/10 text-sky-300 before:bg-sky-400",
  succeeded: "border-emerald-400/20 bg-emerald-400/10 text-emerald-300 before:bg-emerald-400",
  failed: "border-red-400/20 bg-red-400/10 text-red-300 before:bg-red-400",
  canceled: "border-slate-500/20 bg-slate-500/10 text-slate-400 before:bg-slate-500"
};

function normalizeStatus(status?: string | null): StatusBadgeStatus {
  switch ((status ?? "").toLowerCase()) {
    case "running":
    case "run":
      return "running";
    case "stopped":
    case "stop":
    case "paused":
    case "deleted":
      return "stopped";
    case "error":
      return "error";
    case "halted":
      return "halted";
    case "queued":
      return "queued";
    case "succeeded":
    case "success":
      return "succeeded";
    case "failed":
      return "failed";
    case "canceled":
    case "cancelled":
      return "canceled";
    default:
      return "stopped";
  }
}

export function StatusBadge({ status, label, className }: StatusBadgeProps) {
  const { t } = useI18n();
  const normalized = normalizeStatus(status);
  return (
    <span
      className={cn(
        "inline-flex h-7 items-center gap-2 rounded-full border px-2.5 text-xs font-medium before:h-1.5 before:w-1.5 before:rounded-full",
        toneClasses[normalized],
        className
      )}
    >
      {label ?? t(`status.${normalized}`)}
    </span>
  );
}
