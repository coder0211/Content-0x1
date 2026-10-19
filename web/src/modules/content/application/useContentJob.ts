import { useJob, type Job } from "@/modules/job";
import type { ID } from "@/shared/kernel";
import type { JobSnapshot } from "../domain/content";

/** Maps the job module's Job to the Studio's snapshot (asset ids from `result.asset_ids`). */
export function toJobSnapshot(j: Job): JobSnapshot {
  const failedStep = j.steps.find((s) => s.status === "failed");
  const ids = j.result?.asset_ids;
  return {
    id: j.id,
    type: j.type,
    status: j.status,
    progress: j.progress,
    assetIds: Array.isArray(ids) ? ids.filter((x): x is string => typeof x === "string") : [],
    errorKind: failedStep?.errorKind ?? j.error?.code ?? null,
    errorMessage: j.error?.message ?? failedStep?.errorMessage ?? null,
    costEstimate: j.cost.estimate,
    costActual: j.cost.actual,
  };
}

/**
 * Live state of a Studio job through the job module (`useJob`: SSE with 3s polling fallback).
 * `null` → idle.
 */
export function useContentJob(jobId: ID | null) {
  const j = useJob(jobId);
  return {
    data: j.job ? toJobSnapshot(j.job) : undefined,
    cancel: j.cancel,
    isCancelling: j.isCancelling,
  };
}
