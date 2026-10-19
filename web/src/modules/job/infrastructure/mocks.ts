import type { HttpHandler } from "msw";
import type { JobStatus, JobStepStatus } from "@/shared/kernel";
import { accepted, api, fail, mockSession, MOCK_USERS, noContent, ok, page, rest, withAuth } from "@/shared/api/mock";
import { uuid } from "@/shared/lib/id";
import type { SessionUser } from "@/shared/lib/session";
import type { JobType } from "../domain/job";
import type { JobDto, JobHistoryEventDto, JobStepDto } from "./dto";

/**
 * In-memory job store mirroring SPEC-04 §2.7 (api/openapi/job.yaml).
 * Simulated jobs advance with wall-clock time: each step takes `stepMs`, so polling
 * GET /jobs/{id} in `pnpm dev:mock` shows real progress. SSE is unavailable in mock
 * mode (204 → the client falls back to polling every 3s, as SPEC-04 §3 allows).
 */
interface Simulation {
  /** Epoch ms when step 0 started. */
  startedAt: number;
  stepMs: number;
  /** Index of the step that fails (job → failed), if any. */
  failAt?: number;
  failKind?: string;
}

interface MockJob {
  dto: JobDto;
  sim?: Simulation;
  history: JobHistoryEventDto[];
}

let jobs: MockJob[] = [];
let eventSeq = 0;

const RETRYABLE_CODES = new Set([
  "STEP_FAILED",
  "STEP_TIMEOUT",
  "LEASE_EXPIRED",
  "PROVIDER_WAIT_TIMEOUT",
  "PROVIDER_TRANSIENT",
  "PROVIDER_RATE_LIMITED",
  "PROVIDER_UNKNOWN",
]);

function mockUserId(email: string): string {
  return MOCK_USERS.find((u) => u.email === email)?.id ?? email;
}

const isTerminal = (s: JobStatus) => s === "succeeded" || s === "failed" || s === "cancelled";

function pushEvent(j: MockJob, type: string, payload: Record<string, unknown>, stepId: string | null = null, at = new Date()) {
  j.history.push({ id: ++eventSeq, step_id: stepId, event_type: type, payload, created_at: at.toISOString() });
}

function step(key: string, type: string, status: JobStepStatus = "pending", extra: Partial<JobStepDto> = {}): JobStepDto {
  return {
    id: uuid(),
    step_key: key,
    step_type: type,
    status,
    attempt_count: status === "pending" ? 0 : 1,
    error_kind: null,
    error_message: null,
    output: null,
    started_at: null,
    finished_at: null,
    ...extra,
  };
}

export interface MockJobInput {
  type?: JobType;
  status?: JobStatus;
  createdBy?: string;
  providerCode?: string | null;
  modelCode?: string | null;
  campaignId?: string | null;
  /** Steps as `[key, type]`; default one step matching the job type. */
  steps?: Array<[string, string]>;
  /** Simulate progress over time (dev mock). Omit for a static job (tests). */
  simulate?: { stepMs?: number; failAt?: number; failKind?: string };
  estimate?: number | null;
  createdAt?: string;
  errorCode?: string;
  needsReconciliation?: boolean;
  result?: Record<string, unknown> | null;
}

const DEFAULT_STEP: Record<string, [string, string]> = {
  content_generation: ["generate", "text_generation"],
  image_generation: ["generate", "image_generation"],
  video_render: ["render", "video_render"],
};

/** Adds a job to the mock store and returns its DTO. Other modules' mocks may use it to back their 202 responses. */
export function createMockJob(input: MockJobInput = {}): JobDto {
  const type = input.type ?? "content_generation";
  const status: JobStatus = input.status ?? (input.simulate ? "running" : "queued");
  const createdAt = input.createdAt ?? new Date().toISOString();
  const stepDefs = input.steps ?? [DEFAULT_STEP[type] ?? ["generate", type]];
  const steps = stepDefs.map(([k, t]) => step(k, t));
  const dto: JobDto = {
    id: uuid(),
    job_type: type,
    status,
    progress: 0,
    created_by: input.createdBy ?? mockSession.get()?.id ?? mockUserId("creator@acs.local"),
    campaign_id: input.campaignId ?? null,
    asset_id: null,
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
    request: { note: "mock" },
    result: null,
    steps,
  };
  const job: MockJob = { dto, history: [] };
  pushEvent(job, "created", { job_type: type, steps: steps.length }, null, new Date(createdAt));
  if (input.simulate) {
    job.sim = { startedAt: Date.parse(createdAt), stepMs: input.simulate.stepMs ?? 1500, ...input.simulate };
  } else {
    applyStaticStatus(job, status, input);
  }
  jobs.unshift(job);
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
    d.result = input.result ?? { asset_ids: [uuid()] };
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
function advance(job: MockJob, now = Date.now()) {
  const sim = job.sim;
  const d = job.dto;
  if (!sim || isTerminal(d.status) || d.status === "queued") return;
  const total = d.steps.length;
  const elapsed = Math.max(0, now - sim.startedAt);
  const reached = Math.min(total, Math.floor(elapsed / sim.stepMs));
  for (const [i, s] of d.steps.entries()) {
    const stepStart = new Date(sim.startedAt + i * sim.stepMs).toISOString();
    const stepEnd = new Date(sim.startedAt + (i + 1) * sim.stepMs);
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
        pushEvent(job, "step_status_changed", { step_key: s.step_key, status: "failed" }, s.id, stepEnd);
        finish(job, "failed", stepEnd, { code: "STEP_FAILED", message: `Bước ${s.step_key} thất bại` });
        return;
      }
      Object.assign(s, { status: "succeeded", attempt_count: Math.max(1, s.attempt_count), started_at: s.started_at ?? stepStart, finished_at: stepEnd.toISOString() });
      pushEvent(job, "step_status_changed", { step_key: s.step_key, status: "succeeded" }, s.id, stepEnd);
      if (job.dto.cancel_requested) {
        d.steps.slice(i + 1).forEach((r) => (r.status = "cancelled"));
        finish(job, "cancelled", stepEnd, { code: "CANCELLED", message: "Tác vụ đã bị hủy" });
        return;
      }
    } else if (i === reached && s.status !== "running") {
      Object.assign(s, { status: "running", attempt_count: Math.max(1, s.attempt_count), started_at: stepStart });
    }
  }
  const done = d.steps.filter((s) => s.status === "succeeded").length;
  d.progress = Math.round((done / total) * 100);
  if (done === total) {
    finish(job, "succeeded", new Date(sim.startedAt + total * sim.stepMs), null);
  }
}

function finish(job: MockJob, status: JobStatus, at: Date, error: { code: string; message: string } | null) {
  const d = job.dto;
  d.status = status;
  d.finished_at = at.toISOString();
  d.error = error;
  if (status === "succeeded") {
    d.progress = 100;
    d.cost.actual = d.cost.estimate;
    d.cost.source = "provider_reported";
    d.result = d.result ?? { asset_ids: [uuid()] };
  }
  pushEvent(job, "status_changed", { to: status }, null, at);
}

function canView(user: SessionUser, j: JobDto) {
  return j.created_by === user.id || user.permissions.includes("provider.view");
}

function canModify(user: SessionUser, j: JobDto) {
  return j.created_by === user.id || user.roles.includes("admin");
}

function find(id: string | readonly string[] | undefined) {
  return jobs.find((j) => j.dto.id === id);
}

function summary(d: JobDto) {
  const { steps: _s, request: _r, result: _res, brand_version_id: _b, prompt_template_id: _p, ...rest } = d;
  return rest;
}

const notFound = () => fail(404, "NOT_FOUND", "Không tìm thấy tác vụ");

export const handlers: HttpHandler[] = [
  rest.get(
    api("/jobs"),
    withAuth(null, ({ request, user }) => {
      const q = new URL(request.url).searchParams;
      const statuses = q.get("status")?.split(",").filter(Boolean);
      const type = q.get("type");
      const provider = q.get("provider_code");
      const createdBy = user.permissions.includes("provider.view") ? q.get("created_by") : user.id;
      const recon = q.get("needs_reconciliation");
      const limit = Math.min(100, Math.max(1, Number(q.get("limit") ?? 20)));
      const offset = Number(q.get("cursor") ?? 0) || 0;
      const now = Date.now();
      jobs.forEach((j) => advance(j, now));
      const rows = jobs
        .map((j) => j.dto)
        .filter((d) => canView(user, d))
        .filter((d) => !statuses?.length || statuses.includes(d.status))
        .filter((d) => !type || d.job_type === type)
        .filter((d) => !provider || d.provider_code === provider)
        .filter((d) => !createdBy || d.created_by === createdBy)
        .filter((d) => recon == null || String(d.needs_reconciliation) === recon)
        .sort((a, b) => b.created_at.localeCompare(a.created_at));
      const slice = rows.slice(offset, offset + limit).map((d) => structuredClone(summary(d)));
      return page(slice, offset + limit < rows.length ? String(offset + limit) : null);
    }),
  ),
  rest.get(
    api("/jobs/:id"),
    withAuth<{ id: string }>(null, ({ params, user }) => {
      const j = find(params.id);
      if (!j || !canView(user, j.dto)) return notFound();
      advance(j);
      return ok(structuredClone(j.dto));
    }),
  ),
  rest.get(
    api("/jobs/:id/events"),
    withAuth<{ id: string }>(null, ({ params, user }) => {
      const j = find(params.id);
      if (!j || !canView(user, j.dto)) return notFound();
      // No streaming in mock mode: 204 tells EventSource to stop; the client polls instead.
      return noContent();
    }),
  ),
  rest.get(
    api("/jobs/:id/history"),
    withAuth<{ id: string }>(null, ({ params, user }) => {
      const j = find(params.id);
      if (!j || !canView(user, j.dto)) return notFound();
      advance(j);
      return page(structuredClone(j.history));
    }),
  ),
  rest.post(
    api("/jobs/:id/cancel"),
    withAuth<{ id: string }>(null, ({ params, user }) => {
      const j = find(params.id);
      if (!j || !canModify(user, j.dto)) return notFound();
      advance(j);
      const d = j.dto;
      if (isTerminal(d.status)) return fail(409, "INVALID_STATE_TRANSITION", "Tác vụ đã kết thúc, không thể hủy");
      if (!d.cancel_requested) {
        d.cancel_requested = true;
        pushEvent(j, "cancel_requested", { by: user.id });
        const inFlight = d.steps.some((s) => s.status === "running" || s.status === "waiting_provider");
        if (!inFlight || !j.sim) {
          d.steps.forEach((s) => {
            if (s.status !== "succeeded") s.status = "cancelled";
          });
          finish(j, "cancelled", new Date(), { code: "CANCELLED", message: "Tác vụ đã bị hủy" });
        }
      }
      return accepted({ job_id: d.id, status: d.status, cancel_requested: d.cancel_requested });
    }),
  ),
  rest.post(
    api("/jobs/:id/retry"),
    withAuth<{ id: string }>(null, async ({ params, request, user }) => {
      const j = find(params.id);
      if (!j || !canModify(user, j.dto)) return notFound();
      advance(j);
      const body = (await request.json().catch(() => null)) as { force?: boolean } | null;
      const force = body?.force === true;
      const d = j.dto;
      if (d.status !== "failed") return fail(409, "INVALID_STATE_TRANSITION", "Chỉ có thể chạy lại tác vụ đã thất bại");
      if (force && !user.roles.includes("admin")) return fail(403, "FORBIDDEN", "Thiếu quyền");
      if (!force && !RETRYABLE_CODES.has(d.error?.code ?? "")) {
        return fail(409, "INVALID_STATE_TRANSITION", "Lỗi này không thể chạy lại tự động", [
          { field: "error_code", issue: d.error?.code ?? "" },
        ]);
      }
      const prev = d.error?.code ?? null;
      let reset = 0;
      for (const s of d.steps) {
        if (s.status === "failed" || s.status === "cancelled" || s.status === "skipped") {
          Object.assign(s, { status: "pending", attempt_count: 0, error_kind: null, error_message: null, started_at: null, finished_at: null });
          reset++;
        }
      }
      const done = d.steps.filter((s) => s.status === "succeeded").length;
      Object.assign(d, { status: "running", error: null, finished_at: null, cancel_requested: false, progress: Math.round((done / d.steps.length) * 100) });
      const stepMs = j.sim?.stepMs ?? 1500;
      j.sim = { startedAt: Date.now() - done * stepMs, stepMs };
      pushEvent(j, "retried", { by: user.id, force, previous_error: prev, steps: reset });
      return accepted({ job_id: d.id, status: "queued", estimated_cost: null });
    }),
  ),
];

/** Seed data for `pnpm dev:mock`: a mix of statuses, owners and types. */
function seed() {
  const admin = { id: mockUserId("admin@acs.local") };
  const lead = { id: mockUserId("lead@acs.local") };
  const creator = { id: mockUserId("creator@acs.local") };
  const ago = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
  createMockJob({ type: "content_generation", status: "succeeded", createdBy: creator.id, providerCode: "openai", modelCode: "gpt-4o-mini", createdAt: ago(180) });
  createMockJob({
    type: "image_generation",
    status: "failed",
    createdBy: creator.id,
    providerCode: "openai",
    modelCode: "gpt-image-1",
    steps: [["generate:1", "image_generation"], ["generate:2", "image_generation"], ["publish", "asset_publish_ready"]],
    createdAt: ago(120),
  });
  createMockJob({ type: "content_rewrite", status: "cancelled", createdBy: lead.id, providerCode: "anthropic", modelCode: "claude-sonnet", createdAt: ago(90) });
  createMockJob({
    type: "video_production",
    createdBy: lead.id,
    providerCode: "runway",
    modelCode: "gen-4",
    estimate: 3.2,
    steps: [
      ["script", "script_generation"],
      ["storyboard", "storyboard_generation"],
      ["scene:1", "scene_generation"],
      ["scene:2", "scene_generation"],
      ["scene:3", "scene_generation"],
      ["voiceover", "voiceover_generation"],
      ["render", "video_render"],
      ["publish", "asset_publish_ready"],
    ],
    simulate: { stepMs: 8000 },
    createdAt: ago(0.5),
  });
  createMockJob({ type: "image_edit", status: "succeeded", createdBy: admin.id, providerCode: "fake", modelCode: "fake-image", createdAt: ago(30), needsReconciliation: true });
  createMockJob({
    type: "content_generation",
    createdBy: creator.id,
    providerCode: "openai",
    modelCode: "gpt-4o-mini",
    steps: [["variant:1", "text_generation"], ["variant:2", "text_generation"], ["variant:3", "text_generation"]],
    simulate: { stepMs: 5000 },
    createdAt: ago(0.1),
  });
}

/** Clears the store; `seedData` re-adds the dev fixtures. Call in tests' `afterEach`. */
export function resetJobMocks(opts: { seedData?: boolean } = {}) {
  jobs = [];
  eventSeq = 0;
  if (opts.seedData) seed();
}

/** Test helper: raw access to a stored job. */
export function getMockJob(id: string): JobDto | undefined {
  const j = find(id);
  if (j) advance(j);
  return j ? structuredClone(j.dto) : undefined;
}

seed();
