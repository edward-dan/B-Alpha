import type { HTMLAttributes } from "react";
import { cn } from "../../lib/cn";

type BadgeTone = "neutral" | "success" | "danger" | "warning" | "info" | "accent" | "warm";

const tones: Record<BadgeTone, string> = {
  neutral: "border-white/[0.06] bg-white/[0.04] text-text-muted",
  success: "border-success/20 bg-success/10 text-success",
  danger: "border-danger/20 bg-danger/10 text-danger",
  warning: "border-warning/20 bg-warning/10 text-warning",
  info: "border-info/20 bg-info/10 text-info",
  accent: "border-accent/20 bg-accent/10 text-accent",
  warm: "border-warm/20 bg-warm/10 text-warm"
};

type BadgeProps = HTMLAttributes<HTMLSpanElement> & {
  tone?: BadgeTone;
};

export function Badge({ className, tone = "neutral", ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex h-6 items-center rounded-md border px-2 text-[11px] font-medium leading-none",
        tones[tone],
        className
      )}
      {...props}
    />
  );
}
