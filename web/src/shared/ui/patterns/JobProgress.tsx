import type { ErrorKind, JobStatus } from "@/shared/kernel";
import { cn } from "@/shared/lib/cn";
import { describeErrorKind } from "@/shared/lib/errors";
import { formatEta } from "@/shared/lib/format";
import { Button } from "../button";

export interface JobProgressProps {
  status: JobStatus;
  /** 0–100. */
  progress?: number | null;
  /** Concrete step text, e.g. "Đang sinh cảnh 3/5". */
  label?: string;
  etaSeconds?: number | null;
  errorKind?: ErrorKind | string | null;
  /** Detail for `InvalidRequest` or unknown kinds. */
  errorDetail?: string;
  onCancel?: () => void;
  onRetry?: () => void;
  cancelling?: boolean;
  retrying?: boolean;
  className?: string;
}

/** 4px bar + status line + Hủy/Thử lại (SPEC-05 §3, P4). No spinners, no "AI đang suy nghĩ…". */
export function JobProgress({
  status,
  progress,
  label,
  etaSeconds,
  errorKind,
  errorDetail,
  onCancel,
  onRetry,
  cancelling,
  retrying,
  className,
}: JobProgressProps) {
  const active = status === "queued" || status === "running";
  const pct = status === "succeeded" ? 100 : Math.min(100, Math.max(0, progress ?? 0));

  let text: string;
  if (status === "queued") text = label ?? "Đang chờ xử lý";
  else if (status === "running") text = label ?? "Đang xử lý";
  else if (status === "succeeded") text = label ?? "Hoàn thành";
  else if (status === "cancelled") text = "Đã hủy";
  else text = describeErrorKind(errorKind, errorDetail);

  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={active || status === "succeeded" ? pct : undefined}
        aria-label={text}
        className="h-1 w-full overflow-hidden rounded-sm bg-neutral-200"
      >
        <div
          className={cn(
            "h-full transition-[width] duration-200",
            status === "failed" ? "bg-danger" : status === "succeeded" ? "bg-success" : status === "cancelled" ? "bg-neutral-400" : "bg-accent-600",
          )}
          style={{ width: `${status === "failed" || status === "cancelled" ? 100 : pct}%` }}
        />
      </div>
      <div className="flex min-h-7 items-center gap-2">
        <p className={cn("min-w-0 flex-1 text-small", status === "failed" ? "text-danger" : "text-neutral-700")} aria-live="polite">
          {text}
          {active && etaSeconds != null && etaSeconds > 0 ? (
            <span className="tabular text-neutral-500"> · còn {formatEta(etaSeconds)}</span>
          ) : null}
        </p>
        {active && onCancel ? (
          <Button variant="ghost" size="sm" onClick={onCancel} loading={cancelling}>
            Hủy
          </Button>
        ) : null}
        {status === "failed" && onRetry ? (
          <Button variant="secondary" size="sm" onClick={onRetry} loading={retrying}>
            Thử lại
          </Button>
        ) : null}
      </div>
    </div>
  );
}
