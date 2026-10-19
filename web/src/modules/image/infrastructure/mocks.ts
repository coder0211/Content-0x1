import type { HttpHandler } from "msw";
import { accepted, api, fail, mockSession, ok, page, rest, withAuth } from "@/shared/api/mock";
import { uuid } from "@/shared/lib/id";
import type {
  BrandDto,
  CampaignDto,
  GenerationRequestDto,
  ImageBatchDto,
  ImageEstimateDto,
  ImageFileDto,
  ImageReferenceDto,
  JobDto,
  UploadTicketDto,
} from "./dto";

/**
 * Image Studio mocks (SPEC-04 §2.5 + the image-specific endpoints in api/openapi/image.yaml).
 * Also answers, only as a fallback, the few foreign endpoints the Studio calls
 * (`/campaigns`, `/brands`, `/files/*`, `/jobs/{id}` of prompt-suggestion jobs,
 * `/assets/{id}/submit-review` of image assets): handlers registered earlier by the
 * owning modules win, and ids this store does not know fall through to later handlers.
 */

export const MOCK_BRAND_ID = "6b0f3a52-1c1e-4d7b-9a61-0000000000b1";
export const MOCK_CAMPAIGN_ID = "6b0f3a52-1c1e-4d7b-9a61-0000000000c1";
export const MOCK_EMPTY_CAMPAIGN_ID = "6b0f3a52-1c1e-4d7b-9a61-0000000000c2";

const CAMPAIGNS: CampaignDto[] = [
  { id: MOCK_CAMPAIGN_ID, name: "Khai giảng IELTS tháng 11", brand_id: MOCK_BRAND_ID, status: "in_progress" },
  { id: MOCK_EMPTY_CAMPAIGN_ID, name: "Tết 2027 — Ưu đãi học phí", brand_id: MOCK_BRAND_ID, status: "draft" },
];
const BRANDS: BrandDto[] = [{ id: MOCK_BRAND_ID, name: "Anh ngữ Sao Mai" }];

/** Price per image by unit (matches the fake provider seed, pricing fake-2026-10). */
const PRICES: Record<string, number> = { image_1024_standard: 0.04, image_1024_high: 0.17, image_1536_standard: 0.06, image_1536_high: 0.25 };

interface Caps {
  aspectRatios: string[];
  qualities: string[];
  maxVariants: number;
  supportsRefImage: boolean;
  supportsEdit: boolean;
}
const DEFAULT_CAPS: Caps = { aspectRatios: ["1:1", "4:5", "9:16", "16:9"], qualities: ["standard", "high"], maxVariants: 4, supportsRefImage: true, supportsEdit: true };

interface State {
  batches: ImageBatchDto[];
  suggestionJobs: Map<string, { createdBy: string; prompts: string[] }>;
  /** batch id → asset id an edit/variation adds a version to. */
  targetAsset: Map<string, string>;
  files: Map<string, { mime: string; name: string }>;
  caps: Caps;
  perJobLimit: number;
}

const DIMS: Record<string, [number, number]> = { "1:1": [1024, 1024], "4:5": [1024, 1280], "9:16": [864, 1536], "16:9": [1536, 864] };
const HUES = [212, 152, 28, 340, 262, 190];

/** Offline placeholder image (no network) with the real aspect ratio. */
function svgImage(ratio: string, n: number, label: string): string {
  const [w, h] = DIMS[ratio] ?? [1024, 1024];
  const hue = HUES[n % HUES.length]!;
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">` +
    `<rect width="100%" height="100%" fill="hsl(${hue} 45% 82%)"/>` +
    `<rect x="${w * 0.12}" y="${h * 0.12}" width="${w * 0.76}" height="${h * 0.76}" fill="hsl(${hue} 40% 70%)"/>` +
    `<text x="50%" y="50%" font-family="sans-serif" font-size="${Math.round(w / 18)}" text-anchor="middle" fill="hsl(${hue} 30% 25%)">${label}</text>` +
    `</svg>`;
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}

function makeFiles(ratio: string, count: number, parent: string | null, seed: number): ImageFileDto[] {
  const [w, h] = DIMS[ratio] ?? [1024, 1024];
  return Array.from({ length: count }, (_, i) => {
    const url = svgImage(ratio, seed + i, `Ảnh ${i + 1}`);
    return {
      file_id: uuid(),
      role: count > 1 || parent ? "variant" : "primary",
      sort_order: i,
      mime_type: "image/png",
      width: w,
      height: h,
      parent_file_id: parent,
      url,
      thumbnail_file_id: uuid(),
      thumbnail_url: url,
      url_expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
    };
  });
}

function seedBatch(): ImageBatchDto {
  const createdBy = "00000000-0000-4000-8000-000000000003";
  return {
    id: uuid(),
    operation: "generate",
    campaign_id: MOCK_CAMPAIGN_ID,
    brand_id: MOCK_BRAND_ID,
    prompt: "Poster khai giảng khóa IELTS, học viên trẻ trong lớp học sáng, màu xanh dương của brand",
    preset: "poster",
    aspect_ratio: "4:5",
    quality: "standard",
    count: 2,
    model_code: "fake-image",
    source_file_id: null,
    created_by: createdBy,
    created_at: new Date(Date.now() - 3600_000).toISOString(),
    job: { id: uuid(), status: "succeeded", progress: 100, error_code: null, error_kind: null, error_message: null, cost_estimate: { amount: 0.12, currency: "USD", is_estimate: true }, cost_actual: { amount: 0.12, currency: "USD", is_estimate: false } },
    asset: { id: uuid(), title: "Poster khai giảng khóa IELTS", status: "draft", version_no: 1, current_version_no: 1, requires_typography_check: true, images: makeFiles("4:5", 2, null, 0) },
  };
}

const fresh = (): State => ({ batches: [seedBatch()], suggestionJobs: new Map(), targetAsset: new Map(), files: new Map(), caps: { ...DEFAULT_CAPS }, perJobLimit: 2 });
let state = fresh();

/** Test helper: restore the seed data. */
export function resetImageMocks(): void {
  state = fresh();
}

/** Test helper: restrict the mock model (422 CAPABILITY_NOT_SUPPORTED). */
export function setImageMockCapabilities(caps: Partial<Caps>): void {
  state.caps = { ...state.caps, ...caps };
}

/** Test helper: per-job limit in USD (over-budget banner). */
export function setImageMockPerJobLimit(amount: number): void {
  state.perJobLimit = amount;
}

/** Test helper: finish every active job now. */
export function completeImageMockJobs(): void {
  for (const b of state.batches) if (b.job.status === "queued" || b.job.status === "running") finish(b);
}

/** Test helper: fail every active job with the given provider error kind. */
export function failImageMockJobs(kind = "ContentPolicy"): void {
  for (const b of state.batches) {
    if (b.job.status !== "queued" && b.job.status !== "running") continue;
    b.job = { ...b.job, status: "failed", error_code: "PROVIDER_ERROR", error_kind: kind, error_message: "provider rejected" };
  }
}

const isLead = (roles: readonly string[]) => roles.includes("admin") || roles.includes("marketing_lead");

function unitFor(ratio: string, quality: string): string {
  return `image_${ratio === "1:1" ? 1024 : 1536}_${quality}`;
}

function price(ratio: string, quality: string, count: number): number {
  return Math.round((PRICES[unitFor(ratio, quality)] ?? 0.04) * count * 1e6) / 1e6;
}

function capabilityDetails(o: { aspect_ratio: string; quality: string; count: number; refs: number; edit?: boolean }) {
  const c = state.caps;
  const details: { field: string; supported: (string | number)[]; issue?: string }[] = [];
  if (!c.aspectRatios.includes(o.aspect_ratio)) details.push({ field: "aspect_ratio", supported: c.aspectRatios });
  if (!c.qualities.includes(o.quality)) details.push({ field: "quality", supported: c.qualities });
  if (o.count > c.maxVariants) details.push({ field: "count", supported: Array.from({ length: c.maxVariants }, (_, i) => i + 1) });
  if (o.refs > 0 && !c.supportsRefImage) details.push({ field: "reference_file_ids", issue: "reference_image_not_supported", supported: [] });
  if (o.edit && !c.supportsEdit) details.push({ field: "prompt", issue: "edit_not_supported", supported: [] });
  return details;
}

function budgetError(amount: number, override: boolean, roles: readonly string[]) {
  if (override && !isLead(roles)) return fail(403, "FORBIDDEN", "Chỉ Marketing Lead hoặc Admin được duyệt vượt hạn mức");
  if (amount > state.perJobLimit && !(override && isLead(roles))) {
    return fail(429, "JOB_BUDGET_EXCEEDED", "Dự toán vượt hạn mức mỗi job");
  }
  return null;
}

function finish(b: ImageBatchDto): void {
  const actual = b.job.cost_estimate ? { ...b.job.cost_estimate, is_estimate: false } : null;
  b.job = { ...b.job, status: "succeeded", progress: 100, cost_actual: actual };
  const seed = state.batches.length * 3;
  if (b.operation === "generate") {
    b.asset = {
      id: b.asset?.id ?? uuid(),
      title: b.prompt.split("\n")[0]!.slice(0, 80),
      status: "draft",
      version_no: 1,
      current_version_no: 1,
      requires_typography_check: b.preset === "poster",
      images: makeFiles(b.aspect_ratio, b.count, null, seed),
    };
    return;
  }
  // edit / variation: a new version of the source asset.
  const assetId = state.targetAsset.get(b.id)!;
  const src = state.batches.find((x) => x.asset?.id === assetId);
  const versionNo = Math.max(0, ...state.batches.filter((x) => x.asset?.id === assetId).map((x) => x.asset!.version_no)) + 1;
  const images = makeFiles(b.aspect_ratio, b.operation === "edit" ? 1 : b.count, b.source_file_id, seed);
  b.asset = {
    id: assetId,
    title: src?.asset?.title ?? b.prompt.slice(0, 80),
    status: "draft",
    version_no: versionNo,
    current_version_no: versionNo,
    requires_typography_check: b.preset === "poster",
    images,
  };
  for (const x of state.batches) if (x.asset?.id === b.asset.id) x.asset.current_version_no = versionNo;
}

/** Each gallery poll moves active jobs forward: queued → running 30% → 75% → succeeded. */
function advance(): void {
  for (const b of state.batches) {
    if (b.job.status === "queued") b.job = { ...b.job, status: "running", progress: 30 };
    else if (b.job.status === "running" && b.job.progress < 75) b.job = { ...b.job, progress: 75 };
    else if (b.job.status === "running") finish(b);
  }
}

function newBatch(init: Pick<ImageBatchDto, "operation" | "campaign_id" | "brand_id" | "prompt" | "preset" | "aspect_ratio" | "quality" | "count" | "source_file_id" | "created_by">, assetId: string | null, cost: number): ImageBatchDto {
  const b: ImageBatchDto = {
    ...init,
    id: uuid(),
    model_code: "fake-image",
    created_at: new Date().toISOString(),
    job: { id: uuid(), status: "queued", progress: 0, error_code: null, error_kind: null, error_message: null, cost_estimate: { amount: cost, currency: "USD", is_estimate: true }, cost_actual: null },
    asset: null, // pending edits/variations show no images until the job finishes
  };
  if (assetId) state.targetAsset.set(b.id, assetId);
  state.batches.unshift(b);
  return b;
}

function requireKey(request: Request) {
  return /^[0-9a-f-]{36}$/i.test(request.headers.get("Idempotency-Key") ?? "")
    ? null
    : fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu hoặc sai định dạng Idempotency-Key (UUID)");
}

function findAsset(assetId: string, userId: string, roles: readonly string[]) {
  const b = state.batches.find((x) => x.asset?.id === assetId && x.asset.current_version_no === x.asset.version_no);
  if (!b?.asset) return null;
  if (b.created_by !== userId && !isLead(roles)) return null; // IDOR → 404
  return b;
}

const jobAccepted = (b: ImageBatchDto) => accepted({ job_id: b.job.id, status: "queued", estimated_cost: b.job.cost_estimate });

export const handlers: HttpHandler[] = [
  rest.get(
    api("/images/generations"),
    withAuth("generate.run", ({ request, user }) => {
      const url = new URL(request.url);
      const campaignId = url.searchParams.get("campaign_id");
      const assetId = url.searchParams.get("asset_id");
      advance();
      const items = state.batches.filter((b) => {
        if (assetId) return b.asset?.id === assetId || state.targetAsset.get(b.id) === assetId;
        if (campaignId) return b.campaign_id === campaignId;
        return b.created_by === user.id;
      });
      return page(items, null);
    }),
  ),

  rest.post(
    api("/images/estimate"),
    withAuth("generate.run", async ({ request }) => {
      const body = (await request.json()) as { operation?: string; aspect_ratio?: string; quality?: string; count?: number };
      const ratio = body.aspect_ratio || "1:1";
      const quality = body.quality || "standard";
      const count = body.operation === "edit" ? 1 : (body.count ?? 1);
      if (count < 1 || count > 4) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "count", issue: "out_of_range" }]);
      const details = capabilityDetails({ aspect_ratio: ratio, quality, count, refs: 0 });
      if (details.length) return fail(422, "CAPABILITY_NOT_SUPPORTED", "Model không hỗ trợ tham số", details);
      const total = price(ratio, quality, count);
      const dto: ImageEstimateDto = {
        total: { amount: total, currency: "USD", is_estimate: true },
        lines: [{ item: "images", provider_code: "fake", model_code: "fake-image", unit: unitFor(ratio, quality), units: count, amount: { amount: total, currency: "USD", is_estimate: true } }],
        pricing_version: "fake-2026-10",
        per_job_limit: { amount: state.perJobLimit, currency: "USD" },
        campaign_remaining: null,
        global_remaining: { amount: 912.4, currency: "USD" },
        requires_override: total > state.perJobLimit,
        blocked: false,
      };
      return ok(dto);
    }),
  ),

  rest.post(
    api("/images/generations"),
    withAuth("generate.run", async ({ request, user }) => {
      const keyErr = requireKey(request);
      if (keyErr) return keyErr;
      const body = (await request.json()) as Partial<GenerationRequestDto>;
      const issues: { field: string; issue: string }[] = [];
      const prompt = (body.prompt ?? "").trim();
      if (!prompt) issues.push({ field: "prompt", issue: "required" });
      if (!body.preset || !["social_post", "poster", "product_photo", "thumbnail"].includes(body.preset)) issues.push({ field: "preset", issue: "invalid" });
      const count = body.count ?? 1;
      if (count < 1 || count > 4) issues.push({ field: "count", issue: "out_of_range" });
      if (!body.campaign_id && !body.brand_id) issues.push({ field: "campaign_id", issue: "required" });
      if ((body.reference_file_ids ?? []).length > 4) issues.push({ field: "reference_file_ids", issue: "too_many" });
      if (issues.length) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", issues);
      const ratio = body.aspect_ratio || "1:1";
      const quality = body.quality || "standard";
      const details = capabilityDetails({ aspect_ratio: ratio, quality, count, refs: body.reference_file_ids?.length ?? 0 });
      if (details.length) return fail(422, "CAPABILITY_NOT_SUPPORTED", "Model không hỗ trợ tham số", details);
      const cost = price(ratio, quality, count);
      const bErr = budgetError(cost, body.budget_override ?? false, user.roles);
      if (bErr) return bErr;
      const campaign = CAMPAIGNS.find((c) => c.id === body.campaign_id);
      const b = newBatch(
        {
          operation: "generate",
          campaign_id: body.campaign_id ?? null,
          brand_id: campaign?.brand_id ?? body.brand_id ?? MOCK_BRAND_ID,
          prompt,
          preset: body.preset!,
          aspect_ratio: ratio,
          quality,
          count,
          source_file_id: null,
          created_by: user.id,
        },
        null,
        cost,
      );
      return jobAccepted(b);
    }),
  ),

  rest.post(
    api("/images/:assetId/edits"),
    withAuth<{ assetId: string }>("generate.run", async ({ request, params, user }) => {
      const keyErr = requireKey(request);
      if (keyErr) return keyErr;
      const src = findAsset(params.assetId, user.id, user.roles);
      if (!src?.asset) return fail(404, "NOT_FOUND", "Không tìm thấy");
      const body = (await request.json()) as { file_id?: string; prompt?: string; budget_override?: boolean };
      if (!(body.prompt ?? "").trim()) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "prompt", issue: "required" }]);
      if (!src.asset.images.some((i) => i.file_id === body.file_id)) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "file_id", issue: "not_in_asset" }]);
      const details = capabilityDetails({ aspect_ratio: src.aspect_ratio, quality: src.quality, count: 1, refs: 0, edit: true });
      if (details.length) return fail(422, "CAPABILITY_NOT_SUPPORTED", "Model không hỗ trợ tham số", details);
      const cost = price(src.aspect_ratio, src.quality, 1);
      const bErr = budgetError(cost, body.budget_override ?? false, user.roles);
      if (bErr) return bErr;
      const b = newBatch(
        { operation: "edit", campaign_id: src.campaign_id, brand_id: src.brand_id, prompt: body.prompt!.trim(), preset: src.preset, aspect_ratio: src.aspect_ratio, quality: src.quality, count: 1, source_file_id: body.file_id!, created_by: user.id },
        src.asset.id,
        cost,
      );
      return jobAccepted(b);
    }),
  ),

  rest.post(
    api("/images/:assetId/variations"),
    withAuth<{ assetId: string }>("generate.run", async ({ request, params, user }) => {
      const keyErr = requireKey(request);
      if (keyErr) return keyErr;
      const src = findAsset(params.assetId, user.id, user.roles);
      if (!src?.asset) return fail(404, "NOT_FOUND", "Không tìm thấy");
      const body = (await request.json()) as { file_id?: string; count?: number; budget_override?: boolean };
      const count = body.count ?? 1;
      if (count < 1 || count > 4) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "count", issue: "out_of_range" }]);
      if (!src.asset.images.some((i) => i.file_id === body.file_id)) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "file_id", issue: "not_in_asset" }]);
      const details = capabilityDetails({ aspect_ratio: src.aspect_ratio, quality: src.quality, count, refs: 1 });
      if (details.length) return fail(422, "CAPABILITY_NOT_SUPPORTED", "Model không hỗ trợ tham số", details);
      const cost = price(src.aspect_ratio, src.quality, count);
      const bErr = budgetError(cost, body.budget_override ?? false, user.roles);
      if (bErr) return bErr;
      const b = newBatch(
        { operation: "variation", campaign_id: src.campaign_id, brand_id: src.brand_id, prompt: src.prompt, preset: src.preset, aspect_ratio: src.aspect_ratio, quality: src.quality, count, source_file_id: body.file_id!, created_by: user.id },
        src.asset.id,
        cost,
      );
      return jobAccepted(b);
    }),
  ),

  rest.get(
    api("/images/references"),
    withAuth("generate.run", ({ request, user }) => {
      const url = new URL(request.url);
      const campaignId = url.searchParams.get("campaign_id");
      const brandId = url.searchParams.get("brand_id");
      if (!campaignId && !brandId) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "campaign_id", issue: "required" }]);
      const expires = new Date(Date.now() + 15 * 60_000).toISOString();
      const brand: ImageReferenceDto[] = [
        { file_id: "6b0f3a52-1c1e-4d7b-9a61-0000000000f1", source: "brand", label: "Logo Sao Mai", mime_type: "image/png", width: 512, height: 512, url: svgImage("1:1", 0, "Logo"), thumbnail_url: svgImage("1:1", 0, "Logo"), url_expires_at: expires },
        { file_id: "6b0f3a52-1c1e-4d7b-9a61-0000000000f2", source: "brand", label: "Ảnh lớp học mẫu", mime_type: "image/jpeg", width: 1536, height: 864, url: svgImage("16:9", 2, "Lớp học"), thumbnail_url: svgImage("16:9", 2, "Lớp học"), url_expires_at: expires },
      ];
      const library: ImageReferenceDto[] = state.batches
        .filter((b) => b.asset && (campaignId ? b.campaign_id === campaignId : b.created_by === user.id))
        .flatMap((b) =>
          b.asset!.images.map((i) => ({ file_id: i.file_id, source: "library" as const, label: b.asset!.title, mime_type: i.mime_type, width: i.width, height: i.height, url: i.url, thumbnail_url: i.thumbnail_url, url_expires_at: expires })),
        );
      return page([...brand, ...library], null);
    }),
  ),

  rest.post(
    api("/images/prompt-suggestions"),
    withAuth("generate.run", async ({ request, user }) => {
      const keyErr = requireKey(request);
      if (keyErr) return keyErr;
      const body = (await request.json()) as { preset?: string };
      const kind = body.preset === "poster" ? "Poster" : body.preset === "thumbnail" ? "Thumbnail" : "Ảnh";
      const id = uuid();
      state.suggestionJobs.set(id, {
        createdBy: user.id,
        prompts: [
          `${kind} khai giảng khóa IELTS: học viên 18–22 tuổi cười tươi trong lớp học sáng sủa, tông xanh dương #0055FF của brand, khoảng trống phía trên cho tiêu đề`,
          `${kind} tối giản nền trắng, logo Anh ngữ Sao Mai góc trên, sách và tai nghe trên bàn gỗ, điểm nhấn xanh dương #0055FF, phong cách thân thiện, đáng tin cậy`,
          `${kind} ngoài trời: nhóm bạn trẻ luyện nói tiếng Anh ở quán cà phê, ánh sáng tự nhiên, trang phục có chi tiết xanh dương #0055FF, cảm giác gần gũi`,
        ],
      });
      return accepted({ job_id: id, status: "queued", estimated_cost: { amount: 0.01, currency: "USD", is_estimate: true } });
    }),
  ),

  // Fallback for prompt-suggestion jobs only; other job ids fall through to the job module.
  rest.get(api("/jobs/:id"), ({ params }) => {
    const job = state.suggestionJobs.get(String(params.id));
    if (!job) return undefined;
    const user = mockSession.get();
    if (!user) return fail(401, "UNAUTHENTICATED", "Chưa đăng nhập");
    if (user.id !== job.createdBy && !user.permissions.includes("provider.view")) return fail(404, "NOT_FOUND", "Không tìm thấy");
    const dto: JobDto = { id: String(params.id), status: "succeeded", result: { prompts: job.prompts }, error: null };
    return ok({ ...dto, job_type: "image_prompt_suggest", progress: 100, steps: [] });
  }),

  // Fallback for jobs of this store; other job ids fall through to the job module.
  rest.post(api("/jobs/:id/retry"), ({ params }) => {
    const b = state.batches.find((x) => x.job.id === params.id);
    if (!b) return undefined;
    const user = mockSession.get();
    if (!user) return fail(401, "UNAUTHENTICATED", "Chưa đăng nhập");
    if (b.created_by !== user.id && !user.roles.includes("admin")) return fail(404, "NOT_FOUND", "Không tìm thấy");
    if (b.job.status !== "failed") return fail(409, "INVALID_STATE_TRANSITION", "Chỉ thử lại job lỗi");
    b.job = { ...b.job, status: "queued", progress: 0, error_code: null, error_kind: null, error_message: null };
    return accepted({ job_id: b.job.id, status: "queued" });
  }),

  // Fallback for image assets of this store; other assets fall through to the asset/review modules.
  rest.post(api("/assets/:id/submit-review"), async ({ request, params }) => {
    const owned = state.batches.filter((b) => b.asset?.id === params.id);
    if (owned.length === 0) return undefined;
    const user = mockSession.get();
    if (!user) return fail(401, "UNAUTHENTICATED", "Chưa đăng nhập");
    if (owned[0]!.created_by !== user.id && !isLead(user.roles)) return fail(404, "NOT_FOUND", "Không tìm thấy");
    const body = (await request.json().catch(() => ({}))) as { checklist?: { typography_checked?: boolean } };
    const current = owned.find((b) => b.asset!.version_no === b.asset!.current_version_no)!;
    if (current.asset!.status !== "draft" && current.asset!.status !== "changes_requested") {
      return fail(409, "INVALID_STATE_TRANSITION", "Không thể gửi duyệt ở trạng thái hiện tại");
    }
    if (current.asset!.requires_typography_check && body.checklist?.typography_checked !== true) {
      return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "checklist.typography_checked", issue: "required" }]);
    }
    for (const b of owned) b.asset!.status = "pending_review";
    return ok({ id: params.id, status: "pending_review" });
  }),

  // Presigned upload fallback (owned by the file module on the backend; no FE module mocks it yet).
  rest.post(
    api("/files/upload-url"),
    withAuth(null, async ({ request }) => {
      const body = (await request.json()) as { filename?: string; mime_type?: string; size_bytes?: number };
      if (!body.mime_type || !["image/png", "image/jpeg", "image/webp", "image/svg+xml", "application/pdf", "video/mp4", "audio/mpeg"].includes(body.mime_type)) {
        return fail(415, "UNSUPPORTED_MEDIA_TYPE", "Định dạng không được hỗ trợ");
      }
      if ((body.size_bytes ?? 0) > 20 * 1024 * 1024) return fail(413, "FILE_TOO_LARGE", "Tệp quá lớn");
      const id = uuid();
      state.files.set(id, { mime: body.mime_type, name: body.filename ?? "upload" });
      const ticket: UploadTicketDto = {
        file_id: id,
        url: new URL(`/mock-storage/${id}`, window.location.origin).toString(),
        method: "PUT",
        headers: { "Content-Type": body.mime_type },
        expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
      };
      return ok(ticket, { status: 201 });
    }),
  ),
  rest.put("*/mock-storage/:id", () => new Response(null, { status: 200 })),
  rest.post(
    api("/files/:id/complete"),
    withAuth<{ id: string }>(null, ({ params }) => {
      const f = state.files.get(params.id);
      if (!f) return undefined;
      return ok({ id: params.id, mime_type: f.mime, original_name: f.name, upload_status: "ready", kind: "image" });
    }),
  ),

  // Target pickers fallback (campaign/brand modules register theirs earlier).
  rest.get(api("/campaigns"), withAuth("brand.view", () => page(CAMPAIGNS, null))),
  rest.get(api("/brands"), withAuth("brand.view", () => page(BRANDS, null))),
];
