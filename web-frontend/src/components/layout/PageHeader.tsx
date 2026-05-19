import type { ReactNode } from "react";

type PageHeaderProps = {
  title: string;
  description?: string;
  action?: ReactNode;
};

export function PageHeader({ title, description, action }: PageHeaderProps) {
  return (
    <div className="mb-5 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h1 className="text-2xl font-semibold tracking-normal text-text-main">{title}</h1>
        {description && <p className="mt-2 max-w-3xl text-sm leading-6 text-text-muted">{description}</p>}
      </div>
      {action}
    </div>
  );
}
