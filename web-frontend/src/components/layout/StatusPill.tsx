import type { ReactNode } from "react";
import { cn } from "../../lib/cn";

type StatusPillProps = {
  tone: "success" | "danger" | "warning" | "neutral" | "info";
  children: ReactNode;
};

const tones = {
  success: "border-success/20 bg-success/10 text-success",
  danger: "border-danger/20 bg-danger/10 text-danger",
  warning: "border-warning/20 bg-warning/10 text-warning",
  neutral: "border-white/[0.06] bg-white/[0.04] text-text-muted",
  info: "border-info/20 bg-info/10 text-info"
};

export function StatusPill({ tone, children }: StatusPillProps) {
  return (
    <span className={cn("inline-flex h-8 items-center gap-2 rounded-md border px-3 text-xs font-medium", tones[tone])}>
      <span className="h-1.5 w-1.5 rounded-full bg-current" />
      {children}
    </span>
  );
}
