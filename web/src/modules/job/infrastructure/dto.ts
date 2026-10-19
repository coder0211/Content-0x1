import type { JobStatus, JobStepStatus } from "@/shared/kernel";
import type { Job, JobHistoryEvent, JobSummary, JobType } from "../domain/job";

/** GET /jobs row (api/openapi/job.yaml JobSummary). */
export interface JobSummaryDto {
  id: string;
  job_type: JobType;
  status: JobStatus;
  progress: number;
  created_by: string;
  created_by_name: string;
  campaign_id: string | null;
  asset_id: string | null;
  provider_code: string | null;
  model_code: string | null;
  cost: { estimate: number | null; actual: number | null; source: string | null; currency: string };
  error: { code: string; message: string } | null;
  cancel_requested: boolean;
  needs_reconciliation: boolean;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
}

export interface JobStepDto {
  id: string;
  step_key: string;
  step_type: string;
  status: JobStepStatus;
  attempt_count: number;
  error_kind: string | null;
  error_message: string | null;
  output: unknown;
  started_at: string | null;
  finished_at: string | null;
}

/** GET /jobs/{id} (JobDetail). */
export interface JobDto extends JobSummaryDto {
  brand_version_id: string | null;
  prompt_template_id: string | null;
  request: Record<string, unknown> | null;
  result: Record<string, unknown> | null;
  steps: JobStepDto[];
}

export interface JobHistoryEventDto {
  id: number;
  step_id: string | null;
  event_type: string;
  payload: Record<string, unknown> | null;
  created_at: string;
}

export interface CancelAcceptedDto {
  job_id: string;
  status: JobStatus;
  cancel_requested: boolean;
}

export interface JobAcceptedDto {
  job_id: string;
  status: JobStatus;
  estimated_cost?: { amount: number; currency: string; is_estimate: boolean } | null;
}

export function toJobSummary(d: JobSummaryDto): JobSummary {
  return {
    id: d.id,
    type: d.job_type,
    status: d.status,
    progress: d.progress,
    createdBy: d.created_by,
    createdByName: d.created_by_name ?? "",
    campaignId: d.campaign_id,
    assetId: d.asset_id,
    providerCode: d.provider_code,
    modelCode: d.model_code,
    cost: { ...d.cost },
    error: d.error,
    cancelRequested: d.cancel_requested,
    needsReconciliation: d.needs_reconciliation,
    createdAt: d.created_at,
    startedAt: d.started_at,
    finishedAt: d.finished_at,
  };
}

export function toJob(d: JobDto): Job {
  return {
    ...toJobSummary(d),
    request: d.request,
    result: d.result,
    steps: (d.steps ?? []).map((s) => ({
      id: s.id,
      key: s.step_key,
      type: s.step_type,
      status: s.status,
      attemptCount: s.attempt_count,
      errorKind: s.error_kind,
      errorMessage: s.error_message,
      startedAt: s.started_at,
      finishedAt: s.finished_at,
    })),
  };
}

export function toHistoryEvent(d: JobHistoryEventDto): JobHistoryEvent {
  return { id: d.id, stepId: d.step_id, type: d.event_type, payload: d.payload ?? {}, createdAt: d.created_at };
}
