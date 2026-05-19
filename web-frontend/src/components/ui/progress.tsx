import { cn } from "../../lib/cn";

type ProgressProps = {
  value: number;
  className?: string;
  barClassName?: string;
};

export function Progress({ value, className, barClassName }: ProgressProps) {
  const normalized = Math.max(0, Math.min(100, value));
  return (
    <div className={cn("h-2 overflow-hidden rounded-full bg-slate-800/70", className)}>
      <div
        className={cn("h-full rounded-full bg-accent transition-all duration-150", barClassName)}
        style={{ width: `${normalized}%` }}
      />
    </div>
  );
}
