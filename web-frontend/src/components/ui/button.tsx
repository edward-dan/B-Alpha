import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cn } from "../../lib/cn";

type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  size?: "sm" | "md" | "icon";
  icon?: ReactNode;
};

const variants: Record<ButtonVariant, string> = {
  primary: "border-accent bg-accent text-slate-950 shadow-[0_0_22px_rgb(45_212_191/0.16)] hover:bg-accent/90",
  secondary: "border-white/[0.06] bg-white/[0.04] text-text-main hover:bg-white/[0.07]",
  ghost: "border-transparent bg-transparent text-text-muted hover:bg-white/[0.05] hover:text-text-main",
  danger: "border-danger/20 bg-danger/10 text-danger hover:bg-danger/15"
};

const sizes = {
  sm: "h-8 gap-2 px-3 text-xs",
  md: "h-10 gap-2 px-4 text-sm",
  icon: "h-10 w-10 p-0"
};

export function Button({ className, variant = "secondary", size = "md", icon, children, ...props }: ButtonProps) {
  return (
    <button
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-md border font-medium transition duration-150 disabled:cursor-not-allowed disabled:opacity-50",
        variants[variant],
        sizes[size],
        className
      )}
      {...props}
    >
      {icon}
      {children}
    </button>
  );
}
