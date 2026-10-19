import { CircleAlert } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import { describeError } from "@/shared/lib/errors";
import { Button } from "../button";

/** In-place load error with retry and request_id for IT (SPEC-05 §5). */
export function ErrorState({
  error,
  onRetry,
  title = "Không tải được dữ liệu.",
  className,
}: {
  error: unknown;
  onRetry?: () => void;
  title?: string;
  className?: string;
}) {
  const d = describeError(error);
  return (
    <div role="alert" className={cn("flex items-start gap-3 rounded-lg border border-danger-border bg-danger-bg p-4", className)}>
      <CircleAlert className="mt-0.5 size-4 shrink-0 text-danger" aria-hidden />
      <div className="min-w-0 flex-1">
        <p className="text-body font-medium text-danger">{title}</p>
        <p className="text-body text-neutral-700">{d.message}</p>
        {d.requestId ? (
          <p className="mt-1 text-caption font-normal text-neutral-500">
            Mã yêu cầu: <span className="font-mono">{d.requestId}</span>
          </p>
        ) : null}
      </div>
      {onRetry ? (
        <Button size="sm" onClick={onRetry}>
          Thử lại
        </Button>
      ) : null}
    </div>
  );
}
