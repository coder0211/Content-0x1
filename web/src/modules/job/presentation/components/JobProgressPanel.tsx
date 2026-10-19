import { useNavigate } from "react-router";
import type { ID } from "@/shared/kernel";
import { describeErrorKind } from "@/shared/lib/errors";
import { Skeleton } from "@/shared/ui/skeleton";
import { ErrorState, JobProgress } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";
import { useJobProgress } from "../../application/useJobProgress";
import { JOB_TYPE_LABELS, jobErrorKind, type Job } from "../../domain/job";

export interface JobProgressPanelProps {
  jobId: ID;
  /** Toast when the job finishes while this panel is mounted (default true). */
  notify?: boolean;
  onFinished?: (job: Job) => void;
  /** Hide the cancel button (e.g. read-only viewers). */
  hideCancel?: boolean;
  className?: string;
}

/** Live `<JobProgress/>` for one job: SSE + polling fallback, Hủy / Thử lại, completion toast (ACS-019). */
export function JobProgressPanel({ jobId, notify = true, onFinished, hideCancel, className }: JobProgressPanelProps) {
  const navigate = useNavigate();
  const p = useJobProgress(jobId, {
    onFinished: (job) => {
      onFinished?.(job);
      if (!notify) return;
      const view = { label: "Xem", onClick: () => void navigate(`/jobs/${job.id}`) };
      const name = JOB_TYPE_LABELS[job.type] ?? job.type;
      if (job.status === "succeeded") toast.success(`${name}: đã hoàn thành`, { action: view });
      else if (job.status === "failed")
        toast.warning(`${name}: thất bại`, { description: describeErrorKind(jobErrorKind(job), job.error?.message), action: view });
      else toast.info(`${name}: đã hủy`);
    },
  });

  if (p.error) return <ErrorState error={p.error} onRetry={p.refetch} />;
  if (!p.job) return <Skeleton className="h-9 w-full" aria-label="Đang tải tiến độ" />;
  const job = p.job;
  return (
    <JobProgress
      className={className}
      status={job.status}
      progress={job.progress}
      label={p.label}
      errorKind={p.errorKind}
      errorDetail={p.errorDetail}
      onCancel={!hideCancel && p.canCancel ? () => void p.cancel().catch(() => undefined) : undefined}
      onRetry={p.canRetry ? () => void p.retry().catch(() => undefined) : undefined}
      cancelling={p.isCancelling}
      retrying={p.isRetrying}
    />
  );
}
