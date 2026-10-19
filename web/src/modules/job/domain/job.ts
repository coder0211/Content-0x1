import type { ID, ISODateTime, JobStatus, JobStepStatus } from "@/shared/kernel";

/** ai_jobs.job_type (SPEC-03). */
export type JobType =
  | "content_generation"
  | "content_rewrite"
  | "content_transform"
  | "content_plan"
  | "image_generation"
  | "image_edit"
  | "image_prompt_suggest"
  | "video_storyboard"
  | "video_production"
  | "video_scene_regenerate"
  | "video_render";

export const JOB_TYPES: JobType[] = [
  "content_generation",
  "content_rewrite",
  "content_transform",
  "content_plan",
  "image_generation",
  "image_edit",
  "image_prompt_suggest",
  "video_storyboard",
  "video_production",
  "video_scene_regenerate",
  "video_render",
];

export const JOB_TYPE_LABELS: Record<JobType, string> = {
  content_generation: "Viết nội dung",
  content_rewrite: "Viết lại nội dung",
  content_transform: "Chuyển đổi nội dung",
  content_plan: "Lập kế hoạch nội dung",
  image_generation: "Sinh ảnh",
  image_edit: "Chỉnh sửa ảnh",
  image_prompt_suggest: "Gợi ý mô tả ảnh",
  video_storyboard: "Kịch bản & storyboard",
  video_production: "Sản xuất video",
  video_scene_regenerate: "Sinh lại cảnh",
  video_render: "Render video",
};

export const JOB_STATUSES: JobStatus[] = ["queued", "running", "succeeded", "failed", "cancelled"];

export interface JobCost {
  estimate: number | null;
  actual: number | null;
  source: string | null;
  currency: string;
}

export interface JobError {
  code: string;
  message: string;
}

export interface JobStep {
  id: ID;
  key: string;
  type: string;
  status: JobStepStatus;
  attemptCount: number;
  errorKind: string | null;
  errorMessage: string | null;
  startedAt: ISODateTime | null;
  finishedAt: ISODateTime | null;
}

export interface JobSummary {
  id: ID;
  type: JobType;
  status: JobStatus;
  progress: number;
  createdBy: ID;
  campaignId: ID | null;
  assetId: ID | null;
  providerCode: string | null;
  modelCode: string | null;
  cost: JobCost;
  error: JobError | null;
  cancelRequested: boolean;
  needsReconciliation: boolean;
  createdAt: ISODateTime;
  startedAt: ISODateTime | null;
  finishedAt: ISODateTime | null;
}

export interface Job extends JobSummary {
  steps: JobStep[];
  request: Record<string, unknown> | null;
  /** e.g. `{ asset_ids: [...] }` once succeeded. */
  result: Record<string, unknown> | null;
}

export interface JobHistoryEvent {
  id: number;
  stepId: ID | null;
  type: string;
  payload: Record<string, unknown>;
  createdAt: ISODateTime;
}

export interface JobFilter {
  status?: JobStatus;
  type?: JobType;
  providerCode?: string;
  createdBy?: ID;
}

/** SSE events (SPEC-04 §3). */
export type JobStreamEvent =
  | { type: "status"; data: { status?: JobStatus; progress?: number; cancel_requested?: boolean } }
  | { type: "progress"; data: { progress?: number; step_key?: string; step_progress?: number; message?: string } }
  | { type: "step"; data: { step_key?: string; status?: JobStepStatus; attempt_count?: number; error_kind?: string } }
  | { type: "done"; data: { status?: JobStatus; result?: Record<string, unknown> | null; error?: JobError | null } };

export function isTerminal(status: JobStatus): boolean {
  return status === "succeeded" || status === "failed" || status === "cancelled";
}

export function canCancel(job: Pick<JobSummary, "status" | "cancelRequested">): boolean {
  return !isTerminal(job.status) && !job.cancelRequested;
}

export function canRetry(job: Pick<JobSummary, "status">): boolean {
  return job.status === "failed";
}

const STEP_LABELS: Record<string, string> = {
  text_generation: "Viết nội dung",
  content_generation: "Viết nội dung",
  image_generation: "Sinh ảnh",
  image_edit: "Chỉnh sửa ảnh",
  script_generation: "Viết kịch bản",
  storyboard_generation: "Dựng storyboard",
  scene_generation: "Sinh cảnh",
  voiceover_generation: "Tạo giọng đọc",
  subtitle_generation: "Tạo phụ đề",
  media_download: "Tải và chuẩn hóa clip",
  video_render: "Render video",
  quality_check: "Kiểm tra chất lượng",
  asset_publish_ready: "Lưu vào thư viện",
};

/** Human label of a step: "Sinh cảnh 3" for `scene_generation:3`. */
export function stepLabel(step: Pick<JobStep, "key" | "type">): string {
  const base = STEP_LABELS[step.type] ?? step.type.replace(/_/g, " ");
  const suffix = step.key.includes(":") ? step.key.slice(step.key.lastIndexOf(":") + 1) : "";
  return suffix ? `${base} ${suffix}` : base;
}

/** Concrete progress text (SPEC-05 P4), e.g. "Đang sinh cảnh 3 · bước 4/9". */
export function progressLabel(job: Job): string | undefined {
  if (job.cancelRequested && !isTerminal(job.status)) return "Đang hủy sau bước hiện tại";
  if (job.status !== "running" && job.status !== "queued") return undefined;
  const total = job.steps.length;
  const done = job.steps.filter((s) => s.status === "succeeded" || s.status === "skipped").length;
  const active = job.steps.filter((s) => s.status === "running" || s.status === "waiting_provider");
  if (active.length === 0) return job.status === "queued" ? "Đang chờ xử lý" : total > 1 ? `Đã xong ${done}/${total} bước` : undefined;
  const names = active.map(stepLabel);
  const head = names.length > 2 ? `${names.slice(0, 2).join(", ")} và ${names.length - 2} bước khác` : names.join(", ");
  const waiting = active.every((s) => s.status === "waiting_provider") ? " (chờ nhà cung cấp)" : "";
  return total > 1 ? `Đang ${lowerFirst(head)}${waiting} · bước ${Math.min(done + 1, total)}/${total}` : `Đang ${lowerFirst(head)}${waiting}`;
}

function lowerFirst(s: string): string {
  return s.charAt(0).toLowerCase() + s.slice(1);
}

/** The step that failed the job, if any. */
export function failedStep(job: Job): JobStep | undefined {
  return job.steps.find((s) => s.status === "failed");
}

/** error_kind for `describeErrorKind`: the failed step's kind, else the job code. */
export function jobErrorKind(job: Job): string | null {
  return failedStep(job)?.errorKind ?? job.error?.code ?? null;
}

/** Applies one SSE event to a cached job (immutable). */
export function applyStreamEvent(job: Job, ev: JobStreamEvent): Job {
  switch (ev.type) {
    case "status":
      return {
        ...job,
        status: ev.data.status ?? job.status,
        progress: Math.max(job.progress, ev.data.progress ?? 0),
        cancelRequested: ev.data.cancel_requested ?? job.cancelRequested,
      };
    case "progress":
      return { ...job, progress: Math.max(job.progress, ev.data.progress ?? 0) };
    case "step": {
      const key = ev.data.step_key;
      if (!key) return job;
      return {
        ...job,
        steps: job.steps.map((s) =>
          s.key === key
            ? {
                ...s,
                status: ev.data.status ?? s.status,
                attemptCount: ev.data.attempt_count ?? s.attemptCount,
                errorKind: ev.data.error_kind ?? (ev.data.status === "succeeded" ? null : s.errorKind),
              }
            : s,
        ),
      };
    }
    case "done":
      return {
        ...job,
        status: ev.data.status ?? job.status,
        progress: ev.data.status === "succeeded" ? 100 : job.progress,
        result: ev.data.result ?? job.result,
        error: ev.data.error ?? job.error,
      };
  }
}
