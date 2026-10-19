/**
 * MSW handlers for Content Studio (SPEC-04 §2.4) plus the job/asset endpoints
 * for jobs and assets created here. Requests for jobs/assets this module does
 * not know fall through (resolver returns undefined) to the job/library mocks,
 * so module state stays independent. GET /brands, /brands/:id/products and
 * the demo campaign are fallbacks used only when the brand/campaign mocks do
 * not answer first (they are registered earlier in app/mocks/handlers.ts).
 */
import type { HttpHandler } from "msw";
import {
  accepted,
  api,
  fail,
  ok,
  page,
  rest,
  withAuth,
} from "@/shared/api/mock";
import { uuid } from "@/shared/lib/id";
import type {
  AssetDto,
  ContentJsonDto,
  GenerateRequestDto,
  JobDto,
  VersionDto,
} from "./dto";

export const MOCK_BRANDS = [
  { id: "c0000000-0000-4000-8000-0000000000b1", name: "Anh ngữ Sao Mai" },
  { id: "c0000000-0000-4000-8000-0000000000b2", name: "Sao Mai Kids" },
];
export const MOCK_PRODUCTS = [
  {
    id: "c0000000-0000-4000-8000-0000000000p1",
    name: "Khóa giao tiếp 12 tuần",
  },
];
export const MOCK_CAMPAIGN = {
  id: "c0000000-0000-4000-8000-0000000000c1",
  name: "Khóa giao tiếp T11",
  brand_id: MOCK_BRANDS[0]!.id,
  goal: "lead",
  status: "in_progress",
};

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
/** Job polls before a mock job succeeds (queued → running → running → succeeded). */
const DEFAULT_POLLS = 3;
let pollsToFinish = DEFAULT_POLLS;

/** Tests: make jobs finish on the first poll. */
export function setContentMockPolls(n: number): void {
  pollsToFinish = n;
}

interface MockAsset {
  asset: AssetDto;
  ownerId: string;
  versions: VersionDto[];
}

interface MockJob {
  dto: JobDto;
  ownerId: string;
  polls: number;
  complete: () => string[];
}

const assets = new Map<string, MockAsset>();
const jobs = new Map<string, MockJob>();
const idem = new Map<string, string>();

export function resetContentMocks(): void {
  assets.clear();
  jobs.clear();
  idem.clear();
  pollsToFinish = DEFAULT_POLLS;
}

/** Test helper: the stored asset. */
export function getMockContentAsset(id: string): MockAsset | undefined {
  return assets.get(id);
}

const now = () => new Date().toISOString();

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

function addAsset(
  ownerId: string,
  input: {
    title: string;
    subtype: string;
    campaignId: string | null;
    brandId: string | null;
    sourceId?: string | null;
  },
  content: ContentJsonDto,
  jobId: string | null,
  note: string,
): string {
  const id = uuid();
  const v: VersionDto = {
    version_no: 1,
    change_note: note,
    created_at: now(),
    generated_by_job_id: jobId,
    content_json: content,
  };
  assets.set(id, {
    ownerId,
    versions: [v],
    asset: {
      id,
      asset_type: "content",
      subtype: input.subtype,
      title: input.title,
      status: "draft",
      campaign_id: input.campaignId,
      brand_id: input.brandId,
      source_asset_id: input.sourceId ?? null,
      current_version: v,
    },
  });
  return id;
}

function addVersion(
  a: MockAsset,
  content: ContentJsonDto,
  note: string,
  jobId: string | null,
): VersionDto {
  const v: VersionDto = {
    version_no: a.versions.length + 1,
    change_note: note,
    created_at: now(),
    generated_by_job_id: jobId,
    content_json: content,
  };
  a.versions.push(v);
  a.asset.current_version = v;
  if (a.asset.status === "approved") a.asset.status = "draft";
  return v;
}

function createJob(
  ownerId: string,
  type: string,
  key: string | null,
  complete: () => string[],
) {
  if (key) {
    const existing = idem.get(`${ownerId}/${key}`);
    if (existing) return jobs.get(existing)!.dto;
  }
  const id = uuid();
  const dto: JobDto = {
    id,
    job_type: type,
    status: "queued",
    progress: 0,
    result: null,
    error: null,
    cost: { estimate: 0.02, actual: null },
  };
  jobs.set(id, { dto, ownerId, polls: 0, complete });
  if (key) idem.set(`${ownerId}/${key}`, id);
  return dto;
}

function advance(j: MockJob): void {
  if (
    j.dto.status === "succeeded" ||
    j.dto.status === "failed" ||
    j.dto.status === "cancelled"
  )
    return;
  j.polls += 1;
  if (j.polls >= pollsToFinish) {
    j.dto.status = "succeeded";
    j.dto.progress = 100;
    j.dto.result = { asset_ids: j.complete() };
    j.dto.cost = { estimate: 0.02, actual: 0.01 };
  } else {
    j.dto.status = "running";
    j.dto.progress = Math.round((j.polls / pollsToFinish) * 100);
  }
}

function acceptedJob(dto: JobDto) {
  return accepted({
    job_id: dto.id,
    status: dto.status,
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

export const handlers: HttpHandler[] = [
  rest.post(
    api("/content/generations"),
    withAuth("generate.run", async ({ request, user }) => {
      const key = request.headers.get(IDEM_HEADER);
      if (!key)
        return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
      const req = (await request.json()) as GenerateRequestDto;
      const details = [];
      if (!PLATFORMS.includes(req.platform))
        details.push({ field: "platform", issue: "invalid" });
      if (!(req.variants >= 1 && req.variants <= 5))
        details.push({ field: "variants", issue: "out_of_range" });
      if (!req.brand_id && !req.campaign_id)
        details.push({ field: "brand_id", issue: "required" });
      if (details.length)
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", details);
      const format = DEFAULT_FORMAT[req.platform] ?? "facebook_post";
      const brandId = req.brand_id ?? MOCK_CAMPAIGN.brand_id;
      const dto = createJob(user.id, "content_generation", key, () =>
        Array.from({ length: req.variants }, (_, i) => {
          const c = variantContent(req, i, format, dto.id);
          return addAsset(
            user.id,
            {
              title: c.title!,
              subtype: format,
              campaignId: req.campaign_id,
              brandId,
            },
            c,
            dto.id,
            "Tạo bởi AI",
          );
        }),
      );
      return acceptedJob(dto);
    }),
  ),

  rest.post(
    api("/content/:assetId/rewrites"),
    withAuth<{ assetId: string }>(
      "generate.run",
      async ({ request, params, user }) => {
        const key = request.headers.get(IDEM_HEADER);
        if (!key)
          return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
        const a = assets.get(params.assetId);
        if (!a) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
        const isLead =
          user.roles.includes("admin") || user.roles.includes("marketing_lead");
        if (a.ownerId !== user.id && !isLead)
          return fail(
            403,
            "FORBIDDEN",
            "Bạn không có quyền thực hiện thao tác này",
          );
        const req = (await request.json()) as {
          instruction: string;
          tone?: string[];
          custom_prompt?: string;
        };
        if (
          !["shorten", "expand", "change_tone", "custom"].includes(
            req.instruction,
          )
        ) {
          return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [
            { field: "instruction", issue: "invalid" },
          ]);
        }
        if (req.instruction === "custom" && !req.custom_prompt) {
          return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [
            { field: "custom_prompt", issue: "required" },
          ]);
        }
        const base = a.asset.current_version!;
        const dto = createJob(user.id, "content_rewrite", key, () => {
          const c = { ...(base.content_json ?? {}) };
          c.body_text = rewriteBody(
            c.body_text ?? "",
            req.instruction,
            req.tone,
            req.custom_prompt,
          );
          c.body_html = html(c.body_text);
          c.warnings = (c.warnings ?? []).filter((w) =>
            c.body_text!.toLowerCase().includes(w.text.toLowerCase()),
          );
          c.provenance = provenance(dto.id);
          addVersion(a, c, `Viết lại từ phiên bản ${base.version_no}`, dto.id);
          return [a.asset.id];
        });
        return acceptedJob(dto);
      },
    ),
  ),

  rest.post(
    api("/content/:assetId/transforms"),
    withAuth<{ assetId: string }>(
      "generate.run",
      async ({ request, params, user }) => {
        const key = request.headers.get(IDEM_HEADER);
        if (!key)
          return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
        const a = assets.get(params.assetId);
        if (!a) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
        const req = (await request.json()) as { target_type: string };
        if (!FORMATS.includes(req.target_type)) {
          return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [
            { field: "target_type", issue: "invalid" },
          ]);
        }
        const src = a.asset.current_version!.content_json ?? {};
        const dto = createJob(user.id, "content_transform", key, () => {
          const scenes =
            req.target_type === "video_script"
              ? [
                  {
                    narration: src.hook ?? "",
                    visual_hint: "Học viên ngập ngừng trước lớp",
                    duration_sec: 4,
                  },
                  {
                    narration: "Lớp nhỏ, giáo viên bản ngữ.",
                    visual_hint: "Lớp học sáng sủa",
                    duration_sec: 5,
                  },
                  {
                    narration: src.cta ?? "",
                    visual_hint: "Logo trên nền màu thương hiệu",
                    duration_sec: 3,
                  },
                ]
              : [];
          const c: ContentJsonDto = {
            ...src,
            format: req.target_type,
            scenes,
            provenance: provenance(dto.id),
          };
          const title = `${a.asset.title} (${req.target_type})`;
          return [
            addAsset(
              user.id,
              {
                title,
                subtype: req.target_type,
                campaignId: a.asset.campaign_id ?? null,
                brandId: a.asset.brand_id ?? null,
                sourceId: a.asset.id,
              },
              c,
              dto.id,
              "Chuyển định dạng bởi AI",
            ),
          ];
        });
        return acceptedJob(dto);
      },
    ),
  ),

  rest.get(
    api("/jobs/:id"),
    withAuth<{ id: string }>(null, ({ params }) => {
      const j = jobs.get(params.id);
      if (!j) return undefined;
      advance(j);
      return ok({ ...j.dto });
    }),
  ),

  rest.post(
    api("/jobs/:id/cancel"),
    withAuth<{ id: string }>(null, ({ params }) => {
      const j = jobs.get(params.id);
      if (!j) return undefined;
      if (
        j.dto.status === "succeeded" ||
        j.dto.status === "failed" ||
        j.dto.status === "cancelled"
      ) {
        return fail(
          409,
          "INVALID_STATE_TRANSITION",
          "Không thể chuyển sang trạng thái này",
        );
      }
      j.dto.status = "cancelled";
      return accepted({ job_id: j.dto.id, status: "cancelled" });
    }),
  ),

  rest.get(
    api("/assets/:id"),
    withAuth<{ id: string }>("brand.view", ({ params }) => {
      const a = assets.get(params.id);
      return a ? ok(a.asset) : undefined;
    }),
  ),

  rest.get(
    api("/assets/:id/versions"),
    withAuth<{ id: string }>("brand.view", ({ params }) => {
      const a = assets.get(params.id);
      return a ? page([...a.versions].reverse()) : undefined;
    }),
  ),

  rest.post(
    api("/assets/:id/versions"),
    withAuth<{ id: string }>(null, async ({ request, params, user }) => {
      const a = assets.get(params.id);
      if (!a) return undefined;
      const isLead =
        user.roles.includes("admin") || user.roles.includes("marketing_lead");
      if (a.ownerId !== user.id && !isLead)
        return fail(
          403,
          "FORBIDDEN",
          "Bạn không có quyền thực hiện thao tác này",
        );
      const req = (await request.json()) as {
        content_json: ContentJsonDto;
        change_note: string;
        base_version_no: number;
      };
      if (req.base_version_no !== a.versions.length) {
        return fail(
          409,
          "VERSION_CONFLICT",
          "Dữ liệu đã được người khác cập nhật, vui lòng tải lại",
        );
      }
      return ok(
        addVersion(
          a,
          req.content_json,
          req.change_note || "Chỉnh sửa thủ công",
          null,
        ),
        { status: 201 },
      );
    }),
  ),

  rest.post(
    api("/assets/:id/versions/:no/restore"),
    withAuth<{ id: string; no: string }>(null, ({ params }) => {
      const a = assets.get(params.id);
      if (!a) return undefined;
      const v = a.versions.find((x) => x.version_no === Number(params.no));
      if (!v) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      return ok(
        addVersion(
          a,
          v.content_json ?? {},
          `Khôi phục phiên bản ${v.version_no}`,
          null,
        ),
        { status: 201 },
      );
    }),
  ),

  // Fallbacks (brand/campaign mocks answer first when they implement these).
  rest.get(
    api("/brands"),
    withAuth("brand.view", () =>
      page(MOCK_BRANDS.map((b) => ({ ...b, status: "active" }))),
    ),
  ),
  rest.get(
    api("/brands/:id/products"),
    withAuth<{ id: string }>("brand.view", ({ params }) =>
      params.id === MOCK_BRANDS[0]!.id
        ? page(MOCK_PRODUCTS)
        : MOCK_BRANDS.some((b) => b.id === params.id)
          ? page([])
          : undefined,
    ),
  ),
  rest.get(
    api("/campaigns/:id"),
    withAuth<{ id: string }>("brand.view", ({ params }) =>
      params.id === MOCK_CAMPAIGN.id ? ok(MOCK_CAMPAIGN) : undefined,
    ),
  ),
];
