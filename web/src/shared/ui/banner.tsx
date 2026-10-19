import type { ReactNode } from "react";
import { AlertTriangle, CircleAlert, CircleCheck, Info } from "lucide-react";
import { cn } from "@/shared/lib/cn";

const TONES = {
  info: { cls: "border-info-border bg-info-bg text-info", Icon: Info },
  success: { cls: "border-success-border bg-success-bg text-success", Icon: CircleCheck },
  warning: { cls: "border-warning-border bg-warning-bg text-warning", Icon: AlertTriangle },
  danger: { cls: "border-danger-border bg-danger-bg text-danger", Icon: CircleAlert },
} as const;

/** Inline banner, e.g. over-budget warning at the generate area (SPEC-05 §5). */
export function Banner({
  tone = "info",
  title,
  children,
  action,
  className,
}: {
  tone?: keyof typeof TONES;
  title?: ReactNode;
  children?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  const { cls, Icon } = TONES[tone];
  return (
    <div role={tone === "danger" || tone === "warning" ? "alert" : "status"} className={cn("flex items-start gap-2 rounded border px-3 py-2", cls, className)}>
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden />
      <div className="min-w-0 flex-1 text-body">
        {title ? <p className="font-medium">{title}</p> : null}
        {children ? <div className="text-neutral-700">{children}</div> : null}
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}
