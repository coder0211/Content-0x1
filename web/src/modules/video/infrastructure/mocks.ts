import type { HttpHandler } from "msw";
import { accepted, api, fail, ok, page, rest, withAuth } from "@/shared/api/mock";
import { uuid } from "@/shared/lib/id";
import { DEFAULT_LIMITS, PRESETS, validateStoryboard, type SceneDraft } from "../domain/video";
import type {
  BrandAssetDto,
  BrandDto,
  CreateVideoDto,
  EstimateDto,
  JobStepDto,
  SceneDto,
  SceneInputDto,
  VideoJobDto,
  VideoProjectDto,
} from "./dto";

/**
 * In-memory simulation of Flow C (SPEC-01): every GET /videos/{id} advances the current job one tick,
 * so polling drives storyboard → production → render → ready. A scene whose visual prompt contains
 * "[lỗi]" fails during production (to demo partial failure + "Tạo lại cảnh").
 */

const PRICES = { video_second_1080p: 0.15, char: 0.000016, request: 0.002 };
const PER_JOB_LIMIT = 8;
const GLOBAL_REMAINING = 990;
const FAIL_MARK = "[lỗi]";

interface MockJob {
  dto: VideoJobDto;
  ticks: number;
  sceneId?: string;
}

interface MockProject {
  dto: VideoProjectDto;
  job: MockJob | null;
}

const MOCK_BRAND: BrandDto = { id: "b0000000-0000-4000-8000-000000000001", name: "Anh ngữ Sao Mai" };
const MOCK_TRACKS: BrandAssetDto[] = [
  { id: "ba000000-0000-4000-8000-000000000001", asset_type: "music", label: "Nhạc nền tươi sáng (có license)", license: { type: "royalty_free", source: "Thư viện nội bộ" } },
  { id: "ba000000-0000-4000-8000-000000000002", asset_type: "music", label: "Bản demo chưa có license", license: null },
];

let projects = new Map<string, MockProject>();
let idem = new Map<string, unknown>();

export function resetVideoMocks(): void {
  projects = new Map();
  idem = new Map();
}

/** Test helper: seed a project directly. */
export function seedVideoProject(dto: VideoProjectDto, job: MockJob | null = null): void {
  projects.set(dto.id, { dto, job });
}

export function getMockVideo(id: string): VideoProjectDto | undefined {
  return projects.get(id)?.dto;
}

const now = () => new Date().toISOString();
const round6 = (n: number) => Math.round(n * 1e6) / 1e6;

function step(key: string, status: JobStepDto["status"] = "pending"): JobStepDto {
  return { step_key: key, step_type: key.split(":")[0], status, attempt_count: status === "pending" ? 0 : 1, error_kind: null, error_message: null };
}

function newJob(type: VideoJobDto["job_type"], steps: JobStepDto[], sceneId?: string): MockJob {
  return {
    dto: { id: uuid(), job_type: type, status: "queued", progress: 0, steps, error_code: null, error_message: null },
    ticks: 0,
    sceneId,
  };
}

const SAMPLE_SCENES = [
  { v: "Cận cảnh sinh viên mở laptop trong quán cà phê, ánh sáng tự nhiên", n: "Bạn ngại nói tiếng Anh khi gặp người nước ngoài?", s: "Ngại nói tiếng Anh?" },
  { v: "Lớp học nhỏ, giáo viên bản xứ trò chuyện với học viên", n: "Lớp giao tiếp 8 người, giáo viên bản xứ kèm sát.", s: "Lớp 8 người, GV bản xứ" },
  { v: "Học viên luyện nói theo cặp, cười tươi", n: "Mỗi buổi đều luyện nói theo tình huống thật.", s: "Luyện nói tình huống thật" },
  { v: "Học viên tự tin thuyết trình trước lớp", n: "Sau 12 tuần, bạn tự tin giao tiếp hằng ngày.", s: "Tự tin sau 12 tuần" },
  { v: "Logo trung tâm trên nền xanh, số điện thoại tư vấn", n: "Đăng ký tư vấn miễn phí ngay hôm nay.", s: "Đăng ký tư vấn miễn phí" },
];

function buildScenes(targetSeconds: number): SceneDto[] {
  const n = Math.min(DEFAULT_LIMITS.maxScenes, Math.max(1, Math.round(targetSeconds / 4)));
  const each = Math.min(8000, Math.max(2000, Math.round((targetSeconds * 1000) / n / 500) * 500));
  return Array.from({ length: n }, (_, i) => {
    const sample = SAMPLE_SCENES[i % SAMPLE_SCENES.length]!;
    return {
      id: uuid(),
      sequence_no: i + 1,
      duration_ms: each,
      visual_prompt: sample.v,
      narration: sample.n,
      subtitle_text: sample.s,
      source_file_id: null,
      clip_file_id: null,
      status: "draft",
      attempt_count: 0,
      last_error: null,
      preview_url: null,
      thumbnail_url: null,
    };
  });
}

const total = (scenes: SceneDto[]) => scenes.reduce((s, x) => s + x.duration_ms, 0);

function setSteps(job: MockJob, pred: (k: string) => boolean, status: JobStepDto["status"]) {
  for (const s of job.dto.steps) {
    if (pred(s.step_key)) {
      s.status = status;
      if (status !== "pending") s.attempt_count = Math.max(1, s.attempt_count);
    }
  }
}

function finishRender(p: MockProject) {
  const d = p.dto;
  const job = p.job!;
  setSteps(job, () => true, "succeeded");
  job.dto.status = "succeeded";
  job.dto.progress = 100;
  d.version_no += 1;
  d.output = {
    file_id: uuid(),
    url: "",
    thumbnail_url: null,
    duration_ms: total(d.scenes),
    version_no: d.version_no,
    subtitle_file_id: d.settings.subtitles.enabled ? uuid() : null,
    subtitle_url: null,
  };
  d.stage = "ready";
  if (d.status === "approved") d.status = "draft";
}

function markSceneReady(s: SceneDto) {
  s.status = "ready";
  s.attempt_count += 1;
  s.clip_file_id = uuid();
  s.preview_url = "";
  s.last_error = null;
}

function markSceneFailed(s: SceneDto) {
  s.status = "failed";
  s.attempt_count += 1;
  s.last_error = "Nhà cung cấp từ chối nội dung cảnh (ContentPolicy).";
}

/** One simulation tick, called on every GET. */
function advance(p: MockProject): void {
  const job = p.job;
  const d = p.dto;
  if (!job || job.dto.status === "succeeded" || job.dto.status === "failed" || job.dto.status === "cancelled") return;
  job.ticks += 1;
  d.updated_at = now();
  const t = job.ticks;
  switch (job.dto.job_type) {
    case "video_storyboard":
      if (t === 1) {
        job.dto.status = "running";
        job.dto.progress = 50;
        setSteps(job, (k) => k === "script_generation", "succeeded");
        setSteps(job, (k) => k === "storyboard_generation", "running");
      } else {
        d.scenes = buildScenes(d.settings.target_duration_seconds);
        d.total_duration_ms = total(d.scenes);
        d.script = d.scenes.map((s) => s.narration).join(" ");
        setSteps(job, () => true, "succeeded");
        job.dto.status = "succeeded";
        job.dto.progress = 100;
        d.stage = "editing";
      }
      return;
    case "video_production":
      if (t === 1) {
        job.dto.status = "running";
        job.dto.progress = 20;
        d.scenes.forEach((s) => (s.status = "generating"));
        setSteps(job, (k) => k.startsWith("scene_generation"), "waiting_provider");
        setSteps(job, (k) => k === "voiceover_generation", "running");
      } else if (t === 2) {
        d.scenes.forEach((s, i) => {
          const failed = s.visual_prompt.includes(FAIL_MARK);
          if (failed) markSceneFailed(s);
          else markSceneReady(s);
          const st = job.dto.steps.find((x) => x.step_key === `scene_generation:${i + 1}`);
          if (st) {
            st.status = failed ? "failed" : "succeeded";
            if (failed) {
              st.error_kind = "ContentPolicy";
              st.error_message = s.last_error;
            }
          }
        });
        setSteps(job, (k) => k === "voiceover_generation" || k === "subtitle_generation", "succeeded");
        if (d.scenes.some((s) => s.status === "failed")) {
          job.dto.status = "failed";
          job.dto.error_code = "SCENE_FAILED";
          job.dto.error_message = "Có cảnh sinh lỗi. Tạo lại riêng cảnh lỗi rồi render.";
          job.dto.progress = 60;
          d.stage = "failed";
        } else {
          job.dto.progress = 70;
          setSteps(job, (k) => k === "video_render", "running");
        }
      } else {
        finishRender(p);
      }
      return;
    case "video_scene_regenerate": {
      const scene = d.scenes.find((s) => s.id === job.sceneId);
      if (t === 1) {
        job.dto.status = "running";
        job.dto.progress = 30;
        if (scene) scene.status = "generating";
        setSteps(job, (k) => k.startsWith("scene_generation"), "waiting_provider");
      } else if (t === 2) {
        if (scene) markSceneReady(scene);
        setSteps(job, (k) => k.startsWith("scene_generation"), "succeeded");
        setSteps(job, (k) => k === "video_render", "running");
        job.dto.progress = 70;
      } else {
        finishRender(p);
      }
      return;
    }
    case "video_render":
      if (t === 1) {
        job.dto.status = "running";
        job.dto.progress = 50;
        setSteps(job, (k) => k === "video_render", "running");
      } else {
        finishRender(p);
      }
      return;
  }
}

function snapshot(p: MockProject): VideoProjectDto {
  const d = p.dto;
  d.total_duration_ms = total(d.scenes);
  d.current_job = p.job ? structuredClone(p.job.dto) : null;
  return structuredClone(d);
}

function jobActive(p: MockProject) {
  return !!p.job && (p.job.dto.status === "queued" || p.job.dto.status === "running");
}

function canView(p: MockProject, user: { id: string; roles: string[] }) {
  return canEdit(p, user) || user.roles.includes("reviewer");
}

function canEdit(p: MockProject, user: { id: string; roles: string[] }) {
  return p.dto.created_by === user.id || user.roles.includes("admin") || user.roles.includes("marketing_lead");
}

function drafts(scenes: SceneDto[]): SceneDraft[] {
  return scenes.map((s) => ({
    key: s.id,
    id: s.id,
    durationMs: s.duration_ms,
    visualPrompt: s.visual_prompt,
    narration: s.narration ?? "",
    subtitleText: s.subtitle_text ?? "",
    sourceFileId: s.source_file_id,
  }));
}

function estimate(p: MockProject): EstimateDto {
  const d = p.dto;
  const seconds = total(d.scenes) / 1000;
  const chars = d.settings.voiceover.enabled ? d.scenes.reduce((n, s) => n + (s.narration?.length ?? 0), 0) : 0;
  const breakdown: EstimateDto["breakdown"] = [
    { item: "scene_clips", provider: "fake", model: "fake-video", units: seconds, unit: "video_second_1080p", amount: round6(seconds * PRICES.video_second_1080p) },
  ];
  if (d.settings.voiceover.enabled) {
    breakdown.push({ item: "voiceover", provider: "fake", model: "fake-voice", units: chars, unit: "char", amount: round6(chars * PRICES.char) });
  }
  if (d.settings.subtitles.enabled) {
    breakdown.push({ item: "subtitles_script", provider: "fake", model: "fake-text", units: 1, unit: "request", amount: PRICES.request });
  }
  const sum = round6(breakdown.reduce((s, b) => s + b.amount, 0));
  const violations = validateStoryboard(drafts(d.scenes)).map((i) => ({ field: i.field, issue: i.message }));
  return {
    scene_count: d.scenes.length,
    total_clip_seconds: seconds,
    breakdown,
    total: { amount: sum, currency: "USD", is_estimate: true },
    limits: { per_job_limit: PER_JOB_LIMIT, campaign_remaining: null, global_remaining: GLOBAL_REMAINING },
    requires_override: sum > PER_JOB_LIMIT,
    blocked: sum > GLOBAL_REMAINING,
    violations,
  };
}

function jobAccepted(p: MockProject, est: number) {
  return { job_id: p.job!.dto.id, status: "queued", estimated_cost: { amount: est, currency: "USD", is_estimate: true }, asset_id: p.dto.id };
}

function remember(key: string | null, userId: string, body: unknown) {
  if (key) idem.set(`${userId}/${key}`, body);
  return body;
}

function recall(key: string | null, userId: string): unknown {
  return key ? idem.get(`${userId}/${key}`) : undefined;
}

type IdParams = { id: string };
type SceneParams = { id: string; sceneId: string };

export const handlers: HttpHandler[] = [
  rest.post(
    api("/videos"),
    withAuth("generate.run", async ({ request, user }) => {
      const key = request.headers.get("Idempotency-Key");
      if (!key) return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
      const prev = recall(key, user.id);
      if (prev) return accepted(prev);
      const body = (await request.json()) as CreateVideoDto;
      const details: { field: string; issue: string }[] = [];
      if (!body.campaign_id && !body.brand_id) details.push({ field: "brand_id", issue: "required" });
      if (!PRESETS[body.preset]) details.push({ field: "preset", issue: "invalid" });
      if (body.aspect_ratio && PRESETS[body.preset] && PRESETS[body.preset].aspectRatio !== body.aspect_ratio) {
        details.push({ field: "aspect_ratio", issue: "mismatch_preset" });
      }
      if (body.source.type === "script" && !body.script_from_asset_id) details.push({ field: "script_from_asset_id", issue: "required" });
      if (body.source.type !== "script" && !body.prompt?.trim()) details.push({ field: "prompt", issue: "required" });
      if (body.target_duration_seconds < 4 || body.target_duration_seconds > 60) {
        details.push({ field: "target_duration_seconds", issue: "between_4_60" });
      }
      if (body.music.enabled && !body.music.track_brand_asset_id) details.push({ field: "music.track_brand_asset_id", issue: "required" });
      if (details.length) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", details);
      if (body.music.enabled) {
        const track = MOCK_TRACKS.find((t) => t.id === body.music.track_brand_asset_id);
        if (!track?.license) {
          return fail(422, "MUSIC_NOT_LICENSED", "Nhạc nền chưa có license hợp lệ", [{ field: "music.track_brand_asset_id", issue: "not_licensed" }]);
        }
      }
      const preset = PRESETS[body.preset];
      const id = uuid();
      const p: MockProject = {
        dto: {
          id,
          title: body.title?.trim() || (body.prompt ? body.prompt.slice(0, 60) : "Video từ kịch bản"),
          status: "draft",
          stage: "storyboarding",
          campaign_id: body.campaign_id,
          brand_id: body.brand_id ?? MOCK_BRAND.id,
          created_by: user.id,
          prompt: body.prompt,
          script: null,
          settings: {
            preset: body.preset,
            aspect_ratio: preset.aspectRatio,
            width: preset.width,
            height: preset.height,
            fps: 30,
            target_duration_seconds: body.target_duration_seconds,
            resolution: "1080p",
            voiceover: body.voiceover,
            subtitles: body.subtitles,
            music: body.music,
            source: body.source,
          },
          scenes: [],
          total_duration_ms: 0,
          limits: { max_total_seconds: 60, max_scenes: 10, min_scene_ms: 2000, max_scene_ms: 8000 },
          output: null,
          current_job: null,
          version_no: 1,
          created_at: now(),
          updated_at: now(),
        },
        job: newJob("video_storyboard", [step("script_generation"), step("storyboard_generation")]),
      };
      projects.set(id, p);
      const res = { asset_id: id, job_id: p.job!.dto.id, status: "queued", estimated_cost: { amount: 0.004, currency: "USD", is_estimate: true } };
      return accepted(remember(key, user.id, res));
    }),
  ),

  rest.get(
    api("/videos"),
    withAuth("brand.view", ({ request, user }) => {
      const campaignId = new URL(request.url).searchParams.get("campaign_id");
      const items = [...projects.values()]
        .filter((p) => canView(p, user) && (!campaignId || p.dto.campaign_id === campaignId))
        .sort((a, b) => b.dto.created_at.localeCompare(a.dto.created_at))
        .map(snapshot);
      return page(items);
    }),
  ),

  rest.get(
    api("/videos/:id"),
    withAuth<IdParams>("brand.view", ({ params, user }) => {
      const p = projects.get(params.id);
      if (!p || !canView(p, user)) return fail(404, "NOT_FOUND", "Không tìm thấy video");
      advance(p);
      return ok(snapshot(p));
    }),
  ),

  rest.put(
    api("/videos/:id/scenes"),
    withAuth<IdParams>("generate.run", async ({ params, request, user }) => {
      const p = projects.get(params.id);
      if (!p || !canEdit(p, user)) return fail(404, "NOT_FOUND", "Không tìm thấy video");
      if (jobActive(p)) return fail(409, "VIDEO_JOB_IN_PROGRESS", "Video đang có tác vụ chạy, vui lòng chờ hoàn tất");
      const body = (await request.json()) as { scenes: SceneInputDto[] };
      const old = new Map(p.dto.scenes.map((s) => [s.id, s]));
      const next: SceneDto[] = body.scenes.map((s, i) => {
        const prev = s.id ? old.get(s.id) : undefined;
        const changed = !prev || prev.visual_prompt !== s.visual_prompt || prev.duration_ms !== s.duration_ms;
        return {
          id: prev?.id ?? uuid(),
          sequence_no: i + 1,
          duration_ms: s.duration_ms,
          visual_prompt: s.visual_prompt,
          narration: s.narration,
          subtitle_text: s.subtitle_text,
          source_file_id: s.source_file_id,
          clip_file_id: changed ? null : (prev?.clip_file_id ?? null),
          status: changed ? "draft" : (prev?.status ?? "draft"),
          attempt_count: prev?.attempt_count ?? 0,
          last_error: changed ? null : (prev?.last_error ?? null),
          preview_url: changed ? null : (prev?.preview_url ?? null),
          thumbnail_url: changed ? null : (prev?.thumbnail_url ?? null),
        };
      });
      const issues = validateStoryboard(drafts(next)).map((x) => ({ field: x.field, issue: x.message }));
      if (issues.length) return fail(400, "VALIDATION_FAILED", "Storyboard vượt giới hạn", issues);
      p.dto.scenes = next;
      if (p.dto.stage === "ready" || p.dto.stage === "failed") p.dto.stage = "editing";
      p.dto.updated_at = now();
      return ok(snapshot(p));
    }),
  ),

  rest.post(
    api("/videos/:id/estimate"),
    withAuth<IdParams>("generate.run", ({ params }) => {
      const p = projects.get(params.id);
      if (!p) return fail(404, "NOT_FOUND", "Không tìm thấy video");
      return ok(estimate(p));
    }),
  ),

  rest.post(
    api("/videos/:id/productions"),
    withAuth<IdParams>("generate.run", async ({ params, request, user }) => {
      const key = request.headers.get("Idempotency-Key");
      if (!key) return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
      const p = projects.get(params.id);
      if (!p || !canEdit(p, user)) return fail(404, "NOT_FOUND", "Không tìm thấy video");
      const prev = recall(key, user.id);
      if (prev) return accepted(prev);
      if (jobActive(p)) return fail(409, "VIDEO_JOB_IN_PROGRESS", "Video đang có tác vụ chạy, vui lòng chờ hoàn tất");
      const body = (await request.json().catch(() => ({}))) as { budget_override?: boolean };
      const est = estimate(p);
      if (est.violations?.length) return fail(400, "VALIDATION_FAILED", "Storyboard vượt giới hạn", est.violations);
      const isLead = user.roles.includes("admin") || user.roles.includes("marketing_lead");
      if (body.budget_override && !isLead) return fail(403, "FORBIDDEN", "Chỉ Lead/Admin được vượt hạn mức");
      if (est.requires_override && !body.budget_override) {
        return fail(429, "JOB_BUDGET_EXCEEDED", "Chi phí dự toán vượt hạn mức cho một tác vụ", [
          { estimate: est.total.amount, per_job_limit: PER_JOB_LIMIT },
        ]);
      }
      const steps = [
        ...p.dto.scenes.map((_, i) => step(`scene_generation:${i + 1}`)),
        ...(p.dto.settings.voiceover.enabled ? [step("voiceover_generation")] : []),
        ...(p.dto.settings.subtitles.enabled ? [step("subtitle_generation")] : []),
        step("video_render"),
        step("quality_check"),
        step("asset_publish_ready"),
      ];
      p.job = newJob("video_production", steps);
      p.dto.stage = "producing";
      return accepted(remember(key, user.id, jobAccepted(p, est.total.amount)));
    }),
  ),

  rest.post(
    api("/videos/:id/scenes/:sceneId/regenerations"),
    withAuth<SceneParams>("generate.run", async ({ params, request, user }) => {
      const key = request.headers.get("Idempotency-Key");
      if (!key) return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
      const p = projects.get(params.id);
      if (!p || !canEdit(p, user)) return fail(404, "NOT_FOUND", "Không tìm thấy video");
      const prev = recall(key, user.id);
      if (prev) return accepted(prev);
      const scene = p.dto.scenes.find((s) => s.id === params.sceneId);
      if (!scene) return fail(404, "NOT_FOUND", "Không tìm thấy cảnh");
      if (jobActive(p)) return fail(409, "VIDEO_JOB_IN_PROGRESS", "Video đang có tác vụ chạy, vui lòng chờ hoàn tất");
      const body = (await request.json().catch(() => ({}))) as { visual_prompt?: string | null };
      if (body.visual_prompt?.trim()) scene.visual_prompt = body.visual_prompt.trim();
      else scene.visual_prompt = scene.visual_prompt.replace(FAIL_MARK, "").trim();
      p.job = newJob(
        "video_scene_regenerate",
        [step(`scene_generation:${scene.sequence_no}`), step("video_render"), step("quality_check"), step("asset_publish_ready")],
        scene.id,
      );
      p.dto.stage = "producing";
      const amount = round6((scene.duration_ms / 1000) * PRICES.video_second_1080p);
      return accepted(remember(key, user.id, jobAccepted(p, amount)));
    }),
  ),

  rest.post(
    api("/videos/:id/renders"),
    withAuth<IdParams>("generate.run", async ({ params, request, user }) => {
      const key = request.headers.get("Idempotency-Key");
      if (!key) return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
      const p = projects.get(params.id);
      if (!p || !canEdit(p, user)) return fail(404, "NOT_FOUND", "Không tìm thấy video");
      const prev = recall(key, user.id);
      if (prev) return accepted(prev);
      if (jobActive(p)) return fail(409, "VIDEO_JOB_IN_PROGRESS", "Video đang có tác vụ chạy, vui lòng chờ hoàn tất");
      if (p.dto.scenes.length === 0 || p.dto.scenes.some((s) => s.status !== "ready")) {
        return fail(409, "INVALID_STATE_TRANSITION", "Mọi cảnh phải sẵn sàng trước khi render");
      }
      const body = (await request.json().catch(() => ({}))) as Partial<Pick<CreateVideoDto, "subtitles" | "music" | "voiceover">>;
      if (body.subtitles) p.dto.settings.subtitles = body.subtitles;
      if (body.music) p.dto.settings.music = body.music;
      if (body.voiceover) p.dto.settings.voiceover = body.voiceover;
      p.job = newJob("video_render", [step("video_render"), step("quality_check"), step("asset_publish_ready")]);
      p.dto.stage = "producing";
      return accepted(remember(key, user.id, jobAccepted(p, 0)));
    }),
  ),

  // Asset module owns this endpoint; answer only for video ids simulated here, otherwise fall through.
  rest.post(api("/assets/:id/submit-review"), (info) => {
    const p = projects.get(String(info.params.id));
    if (!p) return undefined;
    return withAuth<IdParams>(null, ({ user }) => {
      if (!canEdit(p, user)) return fail(404, "NOT_FOUND", "Không tìm thấy asset");
      if (p.dto.status !== "draft" && p.dto.status !== "changes_requested") {
        return fail(409, "INVALID_STATE_TRANSITION", "Không thể gửi duyệt ở trạng thái này");
      }
      p.dto.status = "pending_review";
      return ok({ id: p.dto.id, status: p.dto.status });
    })(info as never);
  }),

  // Fallbacks so the wizard works in mock mode before the brand module ships its mocks
  // (the brand module's handlers are registered first and win when present).
  rest.get(
    api("/brands"),
    withAuth("brand.view", () => page([MOCK_BRAND])),
  ),
  rest.get(
    api("/brands/:id/assets"),
    withAuth<IdParams>("brand.view", ({ params }) => page(params.id === MOCK_BRAND.id ? MOCK_TRACKS : [])),
  ),
];
