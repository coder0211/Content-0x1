import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/shared/lib/cn";

/** Icon 20px + one sentence + one action. No illustrations (SPEC-05 §3, §5). */
export function EmptyState({
  icon: Icon,
  message,
  action,
  className,
}: {
  icon: LucideIcon;
  message: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-neutral-200 px-6 py-12 text-center", className)}>
      <Icon className="size-5 text-neutral-400" aria-hidden />
      <p className="text-body text-neutral-500">{message}</p>
      {action}
    </div>
  );
}
