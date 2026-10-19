import type { HttpHandler } from "msw";
import { accepted, api, fail, noContent, ok, page, rest, withAuth } from "@/shared/api/mock";
import {
  advanceMockJob as advance,
  allMockJobs,
  canModifyMockJob as canModify,
  canViewMockJob as canView,
  clearMockJobs,
  createMockJob,
  findMockJob as find,
  finishMockJob as finish,
  getMockJob,
  isTerminalJob as isTerminal,
  mockUserId,
  pushJobEvent as pushEvent,
  RETRYABLE_JOB_CODES as RETRYABLE_CODES,
  type MockJobDto,
} from "@/shared/api/mock-jobs";

/**
 * Job API mocks (SPEC-04 §2.7, api/openapi/job.yaml) over the shared job store in
 * `@/shared/api/mock-jobs`, which other modules' mocks use (`createMockJob`) to back their
 * 202 responses. SSE is unavailable in mock mode (204 → the client polls every 3s).
 */

// Re-exported for existing tests.
export { createMockJob, getMockJob };

function summary(d: MockJobDto) {
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
      allMockJobs().forEach((j) => advance(j, now));
      const rows = allMockJobs()
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

/** Clears the shared job store; `seedData` re-adds the dev fixtures. Call in tests' `afterEach`. */
export function resetJobMocks(opts: { seedData?: boolean } = {}) {
  clearMockJobs();
  if (opts.seedData) seed();
}

seed();
