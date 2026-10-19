/**
 * Shared in-memory job store for MSW mocks (promoted from modules/job/infrastructure/mocks.ts).
 * The job module's mocks serve `/jobs*` over this store; studio mocks (content, campaign plan,
 * image prompt suggestions…) back their 202 responses with `createMockJob()` and react to
 * completion through `onSucceeded`, so every job is visible on /jobs and through useJob().
 *
 * Simulated jobs advance with wall-clock time: each step takes `stepMs` (0 = finish on the
 * next read). Never imported by production code paths — only by mock handler files and tests.
 */
import type { JobStatus, JobStepStatus } from "@/shared/kernel";
import { uuid } from "@/shared/lib/id";
import type { SessionUser } from "@/shared/lib/session";
import { MOCK_USERS, mockSession } from "./mock";

/** api/openapi/job.yaml JobStep. */
export interface MockJobStepDto {
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

/** api/openapi/job.yaml JobDetail. */
export interface MockJobDto {
  id: string;
  job_type: string;
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
  brand_version_id: string | null;
  prompt_template_id: string | null;
  request: Record<string, unknown> | null;
  result: Record<string, unknown> | null;
  steps: MockJobStepDto[];
}

export interface MockJobHistoryEventDto {
  id: number;
  step_id: string | null;
  event_type: string;
  payload: Record<string, unknown>;
  created_at: string;
}

interface Simulation {
  /** Epoch ms when step 0 started. */
  startedAt: number;
  stepMs: number;
  /** Index of the step that fails (job → failed), if any. */
  failAt?: number;
  failKind?: string;
}

export interface MockJob {
  dto: MockJobDto;
  sim?: Simulation;
  history: MockJobHistoryEventDto[];
  /** Runs once when the job succeeds; its return value becomes `result`. */
  onSucceeded?: () => Record<string, unknown> | void;
}

export interface MockJobInput {
  type?: string;
  status?: JobStatus;
  createdBy?: string;
  providerCode?: string | null;
  modelCode?: string | null;
  campaignId?: string | null;
  assetId?: string | null;
  /** Steps as `[key, type]`; default one step matching the job type. */
  steps?: Array<[string, string]>;
  /** Simulate progress over time. Omit for a static job (tests). `stepMs: 0` finishes on the next read. */
  simulate?: { stepMs?: number; failAt?: number; failKind?: string };
  estimate?: number | null;
  createdAt?: string;
  errorCode?: string;
  needsReconciliation?: boolean;
  request?: Record<string, unknown> | null;
  result?: Record<string, unknown> | null;
  /** Side effect on success (create assets, save a plan…); returns the job `result`. */
  onSucceeded?: () => Record<string, unknown> | void;
  /** De-duplicates creation per (user, key) like the backend (SPEC-04 §1.6). */
  idempotencyKey?: string | null;
}

let jobs: MockJob[] = [];
let eventSeq = 0;
let idem = new Map<string, string>();

export const RETRYABLE_JOB_CODES = new Set([
  "STEP_FAILED",
  "STEP_TIMEOUT",
  "LEASE_EXPIRED",
  "PROVIDER_WAIT_TIMEOUT",
  "PROVIDER_TRANSIENT",
  "PROVIDER_RATE_LIMITED",
  "PROVIDER_UNKNOWN",
]);

export function mockUserId(email: string): string {
  return MOCK_USERS.find((u) => u.email === email)?.id ?? email;
}

export const isTerminalJob = (s: JobStatus) => s === "succeeded" || s === "failed" || s === "cancelled";

export function pushJobEvent(j: MockJob, type: string, payload: Record<string, unknown>, stepId: string | null = null, at = new Date()) {
  j.history.push({ id: ++eventSeq, step_id: stepId, event_type: type, payload, created_at: at.toISOString() });
}

function step(key: string, type: string): MockJobStepDto {
  return {
    id: uuid(),
    step_key: key,
    step_type: type,
    status: "pending",
    attempt_count: 0,
    error_kind: null,
    error_message: null,
    output: null,
    started_at: null,
    finished_at: null,
  };
}

const DEFAULT_STEP: Record<string, [string, string]> = {
  content_generation: ["generate", "text_generation"],
  image_generation: ["generate", "image_generation"],
  video_render: ["render", "video_render"],
};

/** Adds a job to the shared mock store and returns (a copy of) its DTO. */
export function createMockJob(input: MockJobInput = {}): MockJobDto {
  const owner = input.createdBy ?? mockSession.get()?.id ?? mockUserId("creator@acs.local");
  if (input.idempotencyKey) {
    const existing = jobs.find((j) => j.dto.id === idem.get(`${owner}/${input.idempotencyKey}`));
    if (existing) return structuredClone(existing.dto);
  }
  const type = input.type ?? "content_generation";
  const status: JobStatus = input.status ?? (input.simulate ? "running" : "queued");
  const createdAt = input.createdAt ?? new Date().toISOString();
  const stepDefs = input.steps ?? [DEFAULT_STEP[type] ?? ["generate", type]];
  const steps = stepDefs.map(([k, t]) => step(k, t));
  const dto: MockJobDto = {
    id: uuid(),
    job_type: type,
    status,
    progress: 0,
    created_by: owner,
    created_by_name: MOCK_USERS.find((u) => u.id === owner)?.name ?? "",
    campaign_id: input.campaignId ?? null,
    asset_id: input.assetId ?? null,
    provider_code: input.providerCode === undefined ? "fake" : input.providerCode,
    model_code: input.modelCode === undefined ? "fake-model" : input.modelCode,
    cost: { estimate: input.estimate ?? 0.12, actual: null, source: null, currency: "USD" },
    error: null,
    cancel_requested: false,
    needs_reconciliation: input.needsReconciliation ?? false,
    created_at: createdAt,
    started_at: status === "queued" ? null : createdAt,
    finished_at: null,
    brand_version_id: null,
    prompt_template_id: null,
    request: input.request ?? { note: "mock" },
    result: null,
    steps,
  };
  const job: MockJob = { dto, history: [], onSucceeded: input.onSucceeded };
  pushJobEvent(job, "created", { job_type: type, steps: steps.length }, null, new Date(createdAt));
  if (input.simulate) {
    job.sim = { startedAt: Date.parse(createdAt), stepMs: input.simulate.stepMs ?? 1500, ...input.simulate };
  } else {
    applyStaticStatus(job, status, input);
  }
  jobs.unshift(job);
  if (input.idempotencyKey) idem.set(`${owner}/${input.idempotencyKey}`, dto.id);
  return structuredClone(dto);
}

function applyStaticStatus(job: MockJob, status: JobStatus, input: MockJobInput) {
  const d = job.dto;
  const at = d.created_at;
  if (status === "running") {
    d.steps.forEach((s, i) => {
      s.status = i === 0 ? "running" : "pending";
      s.attempt_count = i === 0 ? 1 : 0;
      s.started_at = i === 0 ? at : null;
    });
  } else if (status === "succeeded") {
    d.steps.forEach((s) => Object.assign(s, { status: "succeeded", attempt_count: 1, started_at: at, finished_at: at }));
    d.progress = 100;
    d.finished_at = at;
    d.cost.actual = d.cost.estimate;
    d.cost.source = "provider_reported";
    d.result = (job.onSucceeded?.() as Record<string, unknown> | undefined) ?? input.result ?? { asset_ids: [uuid()] };
  } else if (status === "failed") {
    const last = d.steps.length - 1;
    d.steps.forEach((s, i) => {
      if (i < last) Object.assign(s, { status: "succeeded", attempt_count: 1, started_at: at, finished_at: at });
      else Object.assign(s, { status: "failed", attempt_count: 3, error_kind: "Transient", error_message: "Provider timeout", started_at: at, finished_at: at });
    });
    d.progress = d.steps.length > 1 ? Math.round((last / d.steps.length) * 100) : 0;
    d.finished_at = at;
    d.error = { code: input.errorCode ?? "STEP_FAILED", message: "Bước cuối thất bại sau 3 lần thử" };
  } else if (status === "cancelled") {
    d.steps.forEach((s) => (s.status = "cancelled"));
    d.finished_at = at;
    d.error = { code: "CANCELLED", message: "Tác vụ đã bị hủy" };
  }
}

/** Advances a simulated job to "now" (idempotent). */
export function advanceMockJob(job: MockJob, now = Date.now()) {
  const sim = job.sim;
  const d = job.dto;
  if (!sim || isTerminalJob(d.status) || d.status === "queued") return;
  const total = d.steps.length;
  const elapsed = Math.max(0, now - sim.startedAt);
  const reached = sim.stepMs <= 0 ? total : Math.min(total, Math.floor(elapsed / sim.stepMs));
  for (const [i, s] of d.steps.entries()) {
    const stepStart = new Date(sim.startedAt + i * sim.stepMs).toISOString();
    const stepEnd = new Date(Math.min(now, sim.startedAt + (i + 1) * sim.stepMs));
    if (s.status === "succeeded") continue;
    if (i < reached) {
      if (sim.failAt === i) {
        Object.assign(s, {
          status: "failed",
          attempt_count: 3,
          error_kind: sim.failKind ?? "Transient",
          error_message: "Provider timeout",
          started_at: s.started_at ?? stepStart,
          finished_at: stepEnd.toISOString(),
        });
        pushJobEvent(job, "step_status_changed", { step_key: s.step_key, status: "failed" }, s.id, stepEnd);
        finishMockJob(job, "failed", stepEnd, { code: "STEP_FAILED", message: `Bước ${s.step_key} thất bại` });
        return;
      }
      Object.assign(s, { status: "succeeded", attempt_count: Math.max(1, s.attempt_count), started_at: s.started_at ?? stepStart, finished_at: stepEnd.toISOString() });
      pushJobEvent(job, "step_status_changed", { step_key: s.step_key, status: "succeeded" }, s.id, stepEnd);
      if (job.dto.cancel_requested) {
        d.steps.slice(i + 1).forEach((r) => (r.status = "cancelled"));
        finishMockJob(job, "cancelled", stepEnd, { code: "CANCELLED", message: "Tác vụ đã bị hủy" });
        return;
      }
    } else if (i === reached && s.status !== "running") {
      Object.assign(s, { status: "running", attempt_count: Math.max(1, s.attempt_count), started_at: stepStart });
    }
  }
  const done = d.steps.filter((s) => s.status === "succeeded").length;
  d.progress = Math.round((done / total) * 100);
  if (done === total) {
    finishMockJob(job, "succeeded", new Date(Math.min(now, sim.startedAt + total * sim.stepMs)), null);
  }
}

export function finishMockJob(job: MockJob, status: JobStatus, at: Date, error: { code: string; message: string } | null) {
  const d = job.dto;
  d.status = status;
  d.finished_at = at.toISOString();
  d.error = error;
  if (status === "succeeded") {
    d.progress = 100;
    d.cost.actual = d.cost.estimate;
    d.cost.source = "provider_reported";
    const result = job.onSucceeded?.();
    d.result = result ?? d.result ?? { asset_ids: [uuid()] };
  }
  pushJobEvent(job, "status_changed", { to: status }, null, at);
}

export function canViewMockJob(user: SessionUser, j: MockJobDto) {
  return j.created_by === user.id || user.permissions.includes("provider.view");
}

export function canModifyMockJob(user: SessionUser, j: MockJobDto) {
  return j.created_by === user.id || user.roles.includes("admin");
}

/** Raw stored job (mutable) — for the job module's handlers. */
export function findMockJob(id: string | readonly string[] | undefined): MockJob | undefined {
  return jobs.find((j) => j.dto.id === id);
}

/** Every stored job (mutable), newest first. */
export function allMockJobs(): MockJob[] {
  return jobs;
}

/** Clears the store (tests). The job module re-seeds its dev fixtures on top. */
export function clearMockJobs() {
  jobs = [];
  eventSeq = 0;
  idem = new Map();
}

/** Test helper: a copy of a stored job, advanced to now. */
export function getMockJob(id: string): MockJobDto | undefined {
  const j = findMockJob(id);
  if (j) advanceMockJob(j);
  return j ? structuredClone(j.dto) : undefined;
}

/** Test helper: finish every active simulated job now (runs their `onSucceeded`). */
export function completeMockJobs() {
  for (const j of jobs) {
    if (isTerminalJob(j.dto.status)) continue;
    if (!j.sim) j.sim = { startedAt: Date.now(), stepMs: 0 };
    if (j.dto.status === "queued") j.dto.status = "running";
    advanceMockJob(j, Number.MAX_SAFE_INTEGER / 2);
  }
}
