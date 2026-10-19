import type { ReactNode } from "react";
import { cn } from "@/shared/lib/cn";

/** Page title row: title (text-page) + optional description + right-aligned actions (≤ 1 primary). */
export function PageHeader({
  title,
  description,
  actions,
  meta,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  /** e.g. status badge or budget left, shown next to the title. */
  meta?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <header className={cn("mb-6 flex items-start justify-between gap-4", className)}>
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <h1 className="truncate text-page text-neutral-900">{title}</h1>
          {meta}
        </div>
        {description ? <p className="mt-1 text-body text-neutral-500">{description}</p> : null}
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </header>
  );
}
