import { cn } from "../../../lib/cn";

export function PnLChartSkeleton({ className }: { className?: string }) {
  return (
    <div className={cn("h-72 rounded-xl bg-slate-800/40 p-4 animate-pulse", className)}>
      <div className="mb-6 h-4 w-32 rounded bg-slate-700/50" />
      <div className="flex h-52 items-end gap-2">
        {Array.from({ length: 18 }).map((_, index) => (
          <div
            // eslint-disable-next-line react/no-array-index-key
            key={index}
            className="flex-1 rounded-t bg-slate-700/50"
            style={{ height: `${28 + ((index * 17) % 62)}%` }}
          />
        ))}
      </div>
    </div>
  );
}
