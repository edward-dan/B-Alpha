import { cn } from "../../../lib/cn";

export function TableSkeleton({ rows = 5, className }: { rows?: number; className?: string }) {
  return (
    <div className={cn("grid gap-2 rounded-xl bg-slate-800/40 p-3 animate-pulse", className)}>
      {Array.from({ length: rows }).map((_, index) => (
        // eslint-disable-next-line react/no-array-index-key
        <div key={index} className="grid grid-cols-[1.4fr_1fr_1fr_0.8fr] gap-3 rounded-lg bg-slate-900/40 p-3">
          <div className="h-3 rounded bg-slate-700/50" />
          <div className="h-3 rounded bg-slate-700/40" />
          <div className="h-3 rounded bg-slate-700/40" />
          <div className="h-3 rounded bg-slate-700/40" />
        </div>
      ))}
    </div>
  );
}
