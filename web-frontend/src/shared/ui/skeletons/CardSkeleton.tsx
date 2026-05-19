import { cn } from "../../../lib/cn";

export function CardSkeleton({ className }: { className?: string }) {
  return (
    <div className={cn("rounded-xl border border-white/[0.04] bg-slate-800/40 p-4 animate-pulse", className)}>
      <div className="h-4 w-1/3 rounded bg-slate-700/50" />
      <div className="mt-4 h-8 w-2/3 rounded bg-slate-700/50" />
      <div className="mt-6 grid gap-2">
        <div className="h-3 rounded bg-slate-700/40" />
        <div className="h-3 w-5/6 rounded bg-slate-700/40" />
      </div>
    </div>
  );
}
