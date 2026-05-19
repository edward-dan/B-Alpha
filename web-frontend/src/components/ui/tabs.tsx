import type { ButtonHTMLAttributes, HTMLAttributes } from "react";
import { cn } from "../../lib/cn";

export function TabsList({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("inline-flex rounded-lg border border-white/[0.06] bg-white/[0.03] p-1", className)}
      {...props}
    />
  );
}

type TabsTriggerProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  active?: boolean;
};

export function TabsTrigger({ active, className, ...props }: TabsTriggerProps) {
  return (
    <button
      className={cn(
        "h-8 rounded-md px-3 text-xs font-medium text-text-muted transition duration-150 hover:text-text-main",
        active && "bg-accent/[0.08] text-accent",
        className
      )}
      {...props}
    />
  );
}
