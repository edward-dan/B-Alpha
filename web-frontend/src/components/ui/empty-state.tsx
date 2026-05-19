import type { ReactNode } from "react";
import { Database } from "lucide-react";
import { cn } from "../../lib/cn";

type EmptyStateProps = {
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
};

export function EmptyState({ title, description, action, className }: EmptyStateProps) {
  return (
    <div
      className={cn(
        "flex min-h-44 flex-col items-center justify-center rounded-lg border border-dashed border-white/[0.08] bg-white/[0.02] p-6 text-center",
        className
      )}
    >
      <Database className="h-6 w-6 text-text-weak" />
      <h3 className="mt-3 text-sm font-semibold text-text-main">{title}</h3>
      {description && <p className="mt-1 max-w-md text-xs leading-5 text-text-muted">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
