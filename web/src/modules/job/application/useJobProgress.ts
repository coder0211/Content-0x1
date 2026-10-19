import type { ID } from "@/shared/kernel";
import { jobErrorKind, progressLabel, failedStep, canCancel, canRetry } from "../domain/job";
import { useJob, type UseJobOptions, type UseJobResult } from "./useJob";

export interface UseJobProgressResult extends UseJobResult {
  /** Concrete step text for `<JobProgress label>`, e.g. "Đang sinh cảnh 3 · bước 4/9". */
  label: string | undefined;
  /** error_kind for `<JobProgress errorKind>` when failed. */
  errorKind: string | null;
  errorDetail: string | undefined;
  canCancel: boolean;
  canRetry: boolean;
}

/**
 * `useJob` plus the derived values `<JobProgress/>` needs. Exported from the module's index for
 * other modules (content, image, video) per FE-CONVENTIONS §5.
 */
export function useJobProgress(jobId: ID | null | undefined, opts: UseJobOptions = {}): UseJobProgressResult {
  const r = useJob(jobId, opts);
  const job = r.job;
  return {
    ...r,
    label: job ? progressLabel(job) : undefined,
    errorKind: job ? jobErrorKind(job) : null,
    errorDetail: job ? (failedStep(job)?.errorMessage ?? job.error?.message ?? undefined) : undefined,
    canCancel: job ? canCancel(job) : false,
    canRetry: job ? canRetry(job) : false,
  };
}
