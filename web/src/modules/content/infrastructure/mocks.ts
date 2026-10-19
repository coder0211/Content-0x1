/**
 * MSW handlers for Content Studio (SPEC-04 §2.4). Jobs live in the shared job store
 * (`@/shared/api/mock-jobs`, served by the job module's mocks) and generated variants are
 * published to the shared asset store (`@/shared/api/mock-assets`, served by the library
 * module's `/assets*` mocks), so the rest of the app (jobs, library, review) sees them.
 * Brands, products and campaigns come from the brand/campaign mocks.
 */
import type { HttpHandler } from "msw";
import { accepted, api, fail, rest, withAuth } from "@/shared/api/mock";
import { mockAssets } from "@/shared/api/mock-assets";
import { createMockJob, type MockJobDto } from "@/shared/api/mock-jobs";
import type { ContentJsonDto, GenerateRequestDto } from "./dto";

/** Seed brand of the brand/campaign mocks (used when a request only names a campaign). */
const DEFAULT_BRAND_ID = "10000000-0000-4000-8000-000000000001";

const PLATFORMS = ["facebook", "instagram", "tiktok", "zalo", "email"];
const FORMATS = [
  "facebook_post",
  "tiktok_script",
  "caption",
  "email",
  "video_script",
];
const DEFAULT_FORMAT: Record<string, string> = {
  facebook: "facebook_post",
  instagram: "caption",
  zalo: "caption",
  tiktok: "tiktok_script",
  email: "email",
};
/** Wall-clock duration of each simulated job step (dev); 0 = finish on the next read (tests). */
const DEFAULT_STEP_MS = 600;
let stepMs = DEFAULT_STEP_MS;

/** Tests: `n ≤ 1` makes jobs finish on the first read of GET /jobs/{id}. */
export function setContentMockPolls(n: number): void {
  stepMs = n <= 1 ? 0 : DEFAULT_STEP_MS;
}

export function resetContentMocks(): void {
  stepMs = DEFAULT_STEP_MS;
}


function html(text: string): string {
  return text
    .split("\n\n")
    .map(
      (p) =>
        `<p>${p.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/\n/g, "<br>")}</p>`,
    )
    .join("");
}

function provenance(jobId: string) {
  return {
    job_id: jobId,
    provider_code: "fake",
    model_code: "fake-text",
    prompt_template_code: "content.facebook_post",
    prompt_template_version: 1,
    brand_version_no: 1,
  };
}

function startJob(
  ownerId: string,
  type: string,
  key: string,
  steps: Array<[string, string]>,
  complete: (jobId: string) => string[],
  extra: { campaignId?: string | null; assetId?: string | null } = {},
): MockJobDto {
  const dto: MockJobDto = createMockJob({
    type,
    createdBy: ownerId,
    idempotencyKey: key,
    providerCode: "fake",
    modelCode: "fake-text",
    estimate: 0.02,
    steps,
    campaignId: extra.campaignId ?? null,
    assetId: extra.assetId ?? null,
    simulate: { stepMs },
    onSucceeded: () => ({ asset_ids: complete(dto.id) }),
  });
  return dto;
}

function acceptedJob(dto: MockJobDto) {
  return accepted({
    job_id: dto.id,
    status: "queued",
    estimated_cost: { amount: 0.02, currency: "USD", is_estimate: true },
  });
}

function variantContent(
  req: GenerateRequestDto,
  i: number,
  format: string,
  jobId: string,
): ContentJsonDto {
  const audience = req.brief.target_audience || "bạn";
  const message = req.brief.key_message || "Tự tin giao tiếp tiếng Anh";
  const cta = req.brief.cta || "Đăng ký tư vấn";
  const angles = [
    "Bạn ngại nói tiếng Anh?",
    "12 tuần để tự tin hơn",
    "Chuyện của Minh, sinh viên năm 3",
  ];
  const hook = angles[i % angles.length]!;
  const extra =
    i === 1 ? "\n\nChỉ hôm nay giảm 50% học phí cho 10 bạn đầu tiên." : "";
  const body = `${hook} ${message} dành cho ${audience}.\n\nLớp nhỏ, giáo viên bản ngữ, lịch học linh hoạt.${extra}`;
  return {
    title: `${message} — phương án ${String.fromCharCode(65 + i)}`,
    hook,
    body_text: body,
    body_html: html(body),
    cta,
    hashtags: ["#tienganh", "#giaotiep"],
    platform: req.platform,
    format,
    language: req.language || "vi",
    scenes: [],
    warnings: i === 1 ? [{ type: "unverified_claim", text: "giảm 50%" }] : [],
    provenance: provenance(jobId),
  };
}

function rewriteBody(
  text: string,
  instruction: string,
  tone: string[] | undefined,
  custom: string | undefined,
): string {
  switch (instruction) {
    case "shorten":
      return text.split("\n\n")[0] ?? text;
    case "expand":
      return `${text}\n\nĐăng ký hôm nay để được kiểm tra trình độ miễn phí.`;
    case "change_tone":
      return `(${(tone ?? []).join(", ")}) ${text}`;
    default:
      return `${text}\n\n${custom ?? ""}`.trim();
  }
}

const IDEM_HEADER = "Idempotency-Key";
const isLead = (roles: readonly string[]) => roles.includes("admin") || roles.includes("marketing_lead");

function publish(
  ownerId: string,
  input: { title: string; subtype: string; campaignId: string | null; brandId: string; sourceId?: string | null },
  content: ContentJsonDto,
  jobId: string,
  jobType: string,
  note: string,
): string {
  return mockAssets.create({
    type: "content",
    subtype: input.subtype,
    title: input.title,
    campaignId: input.campaignId,
    brandId: input.brandId,
    createdBy: ownerId,
    sourceAssetId: input.sourceId ?? null,
    content: content as Record<string, unknown>,
    jobId,
    jobType,
    changeNote: note,
  }).summary.id;
}

export const handlers: HttpHandler[] = [
  rest.post(
    api("/content/generations"),
    withAuth("generate.run", async ({ request, user }) => {
      const key = request.headers.get(IDEM_HEADER);
      if (!key) return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
      const req = (await request.json()) as GenerateRequestDto;
      const details = [];
      if (!PLATFORMS.includes(req.platform)) details.push({ field: "platform", issue: "invalid" });
      if (!(req.variants >= 1 && req.variants <= 5)) details.push({ field: "variants", issue: "out_of_range" });
      if (!req.brand_id && !req.campaign_id) details.push({ field: "brand_id", issue: "required" });
      if (details.length) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", details);
      const format = DEFAULT_FORMAT[req.platform] ?? "facebook_post";
      const brandId = req.brand_id ?? DEFAULT_BRAND_ID;
      const steps = Array.from({ length: req.variants }, (_, i): [string, string] => [`variant:${i + 1}`, "text_generation"]);
      const dto = startJob(
        user.id,
        "content_generation",
        key,
        [...steps, ["checks", "content_checks"]],
        (jobId) =>
          Array.from({ length: req.variants }, (_, i) => {
            const c = variantContent(req, i, format, jobId);
            return publish(user.id, { title: c.title!, subtype: format, campaignId: req.campaign_id, brandId }, c, jobId, "content_generation", "Tạo bởi AI");
          }),
        { campaignId: req.campaign_id },
      );
      return acceptedJob(dto);
    }),
  ),

  rest.post(
    api("/content/:assetId/rewrites"),
    withAuth<{ assetId: string }>("generate.run", async ({ request, params, user }) => {
      const key = request.headers.get(IDEM_HEADER);
      if (!key) return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
      const a = mockAssets.find(params.assetId);
      if (!a || a.summary.type !== "content") return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      if (a.summary.created_by !== user.id && !isLead(user.roles)) return fail(403, "FORBIDDEN", "Bạn không có quyền thực hiện thao tác này");
      const req = (await request.json()) as { instruction: string; tone?: string[]; custom_prompt?: string };
      if (!["shorten", "expand", "change_tone", "custom"].includes(req.instruction)) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "instruction", issue: "invalid" }]);
      }
      if (req.instruction === "custom" && !req.custom_prompt) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "custom_prompt", issue: "required" }]);
      }
      const base = mockAssets.current(a);
      const dto = startJob(
        user.id,
        "content_rewrite",
        key,
        [["rewrite", "text_generation"]],
        (jobId) => {
          const c = { ...(base.content_json as ContentJsonDto) };
          c.body_text = rewriteBody(c.body_text ?? "", req.instruction, req.tone, req.custom_prompt);
          c.body_html = html(c.body_text);
          c.warnings = (c.warnings ?? []).filter((w) => c.body_text!.toLowerCase().includes(w.text.toLowerCase()));
          c.provenance = provenance(jobId);
          mockAssets.addVersion(a, {
            content: c as Record<string, unknown>,
            jobId,
            jobType: "content_rewrite",
            changeNote: `Viết lại từ phiên bản ${base.version_no}`,
            createdBy: user.id,
          });
          return [a.summary.id];
        },
        { campaignId: a.summary.campaign_id, assetId: a.summary.id },
      );
      return acceptedJob(dto);
    }),
  ),

  rest.post(
    api("/content/:assetId/transforms"),
    withAuth<{ assetId: string }>("generate.run", async ({ request, params, user }) => {
      const key = request.headers.get(IDEM_HEADER);
      if (!key) return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
      const a = mockAssets.find(params.assetId);
      if (!a || a.summary.type !== "content") return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      const req = (await request.json()) as { target_type: string };
      if (!FORMATS.includes(req.target_type)) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "target_type", issue: "invalid" }]);
      }
      const src = mockAssets.current(a).content_json as ContentJsonDto;
      const dto = startJob(
        user.id,
        "content_transform",
        key,
        [["transform", "text_generation"]],
        (jobId) => {
          const scenes =
            req.target_type === "video_script"
              ? [
                  { narration: src.hook ?? "", visual_hint: "Học viên ngập ngừng trước lớp", duration_sec: 4 },
                  { narration: "Lớp nhỏ, giáo viên bản ngữ.", visual_hint: "Lớp học sáng sủa", duration_sec: 5 },
                  { narration: src.cta ?? "", visual_hint: "Logo trên nền màu thương hiệu", duration_sec: 3 },
                ]
              : [];
          const c: ContentJsonDto = { ...src, format: req.target_type, scenes, provenance: provenance(jobId) };
          return [
            publish(
              user.id,
              {
                title: `${a.summary.title} (${req.target_type})`,
                subtype: req.target_type,
                campaignId: a.summary.campaign_id,
                brandId: a.summary.brand_id,
                sourceId: a.summary.id,
              },
              c,
              jobId,
              "content_transform",
              "Chuyển định dạng bởi AI",
            ),
          ];
        },
        { campaignId: a.summary.campaign_id, assetId: a.summary.id },
      );
      return acceptedJob(dto);
    }),
  ),
];
