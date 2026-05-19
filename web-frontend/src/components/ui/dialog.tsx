import type { ReactNode } from "react";
import { X } from "lucide-react";
import { Button } from "./button";
import { cn } from "../../lib/cn";

type DialogProps = {
  open: boolean;
  title: string;
  children: ReactNode;
  onClose: () => void;
  className?: string;
};

export function Dialog({ open, title, children, onClose, className }: DialogProps) {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 p-4 backdrop-blur-sm">
      <section className={cn("qs-glass w-full max-w-lg rounded-lg", className)} role="dialog" aria-modal="true">
        <div className="flex items-center justify-between gap-4 border-b border-white/[0.05] p-4">
          <h2 className="text-sm font-semibold text-text-main">{title}</h2>
          <Button aria-label="关闭" title="关闭" variant="ghost" size="icon" icon={<X className="h-4 w-4" />} onClick={onClose} />
        </div>
        <div className="p-4">{children}</div>
      </section>
    </div>
  );
}
