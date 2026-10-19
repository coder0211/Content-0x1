import type { HttpHandler } from "msw";
import type { AssetStatus } from "@/shared/kernel";
import { api, fail, MOCK_USERS, noContent, ok, page, rest, withAuth } from "@/shared/api/mock";
import type { SessionUser } from "@/shared/lib/session";
import type {
  AssetApprovalDto,
  AssetDetailDto,
  AssetDownloadUrlDto,
  AssetSummaryDto,
  AssetVersionDto,
  CampaignOptionDto,
} from "./dto";

/**
 * MSW handlers for the asset API (api/openapi/asset.yaml): library search, detail, versions,
 * submit-review, download — and also the review endpoints (/reviews/queue, approve,
 * request-changes), because they mutate the same in-memory asset store. The review module's
 * mocks.ts stays empty to avoid duplicate handlers.
 */

const [, LEAD, CREATOR, REVIEWER] = MOCK_USERS as [SessionUser, SessionUser, SessionUser, SessionUser];
const BRAND_ID = "10000000-0000-4000-8000-000000000001";

export const MOCK_CAMPAIGNS: CampaignOptionDto[] = [
  { id: "20000000-0000-4000-8000-000000000001", name: "Tết 2027 – Bánh Kem Hương Việt" },
  { id: "20000000-0000-4000-8000-000000000002", name: "Back to school 2026" },
];
const [TET, SCHOOL] = MOCK_CAMPAIGNS as [CampaignOptionDto, CampaignOptionDto];

interface StoredAsset {
  summary: Omit<AssetSummaryDto, "current_version_no" | "thumbnail_url">;
  versions: AssetVersionDto[];
  approvals: AssetApprovalDto[];
  deleted: boolean;
}

let seq = 0;
const id = (prefix: string) => `${prefix}-${String(++seq).padStart(12, "0")}`.replace(/^(.{8})-?/, "$1-0000-4000-8000-");

function svgDataUrl(label: string, fill: string): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="640"><rect width="640" height="640" fill="${fill}"/><text x="320" y="330" font-size="36" text-anchor="middle" fill="#334155" font-family="sans-serif">${label}</text></svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}

function version(
  no: number,
  user: SessionUser,
  createdAt: string,
  content: Record<string, unknown>,
  opts: { note?: string | null; job?: boolean; image?: string; video?: boolean } = {},
): AssetVersionDto {
  const jobId = opts.job ? id("30000000") : null;
  const files: AssetVersionDto["files"] = [];
  if (opts.image) {
    files.push({ file_id: id("40000000"), role: "primary", sort_order: 0, mime_type: "image/svg+xml", kind: "image", width: 640, height: 640, duration_ms: null, url: opts.image });
  }
  if (opts.video) {
    files.push({ file_id: id("40000000"), role: "primary", sort_order: 0, mime_type: "video/mp4", kind: "video", width: 1080, height: 1920, duration_ms: 20000, url: "/mock/video.mp4" });
    files.push({ file_id: id("40000000"), role: "thumbnail", sort_order: 1, mime_type: "image/svg+xml", kind: "image", width: 540, height: 960, duration_ms: null, url: svgDataUrl("Video Tết", "#E2E8F0") });
  }
  return {
    id: id("50000000"),
    version_no: no,
    content_json: content,
    change_note: opts.note ?? null,
    generated_by_job_id: jobId,
    created_by: user.id,
    created_by_name: user.name,
    created_at: createdAt,
    files,
    provenance: jobId
      ? {
          job_id: jobId,
          job_type: opts.video ? "video_production" : opts.image ? "image_generation" : "content_generation",
          provider_code: opts.video ? "google_veo" : opts.image ? "openai" : "anthropic",
          model_code: opts.video ? "veo-3" : opts.image ? "gpt-image-1" : "claude-sonnet-5-5",
          prompt_template_code: opts.image ? "image.generate" : opts.video ? "video.storyboard" : "content.facebook_post",
          prompt_template_version: 2,
          brand_version_id: "60000000-0000-4000-8000-000000000003",
          brand_version_no: 3,
        }
      : null,
  };
}

const post = (title: string, hook: string, body: string) => ({
  title,
  hook,
  body_html: `<p>${body}</p>`,
  body_text: body,
  cta: "Đặt bánh ngay hôm nay",
  hashtags: ["#BanhKemHuongViet", "#Tet2027"],
  platform: "facebook",
  warnings: [],
});

function seed(): StoredAsset[] {
  seq = 0;
  const mk = (
    s: Partial<StoredAsset["summary"]> & Pick<StoredAsset["summary"], "title" | "type" | "status" | "created_at">,
    versions: AssetVersionDto[],
    approvals: AssetApprovalDto[] = [],
    owner: SessionUser = CREATOR,
  ): StoredAsset => ({
    summary: {
      id: id("70000000"),
      campaign_id: null,
      brand_id: BRAND_ID,
      subtype: null,
      tags: [],
      usage_rights: null,
      source_asset_id: null,
      created_by: owner.id,
      created_by_name: owner.name,
      submitted_at: null,
      updated_at: s.created_at,
      ...s,
    },
    versions,
    approvals,
    deleted: false,
  });

  const t = (d: number, h = 9) => new Date(Date.UTC(2026, 9, d, h, 0, 0)).toISOString();
  const assets: StoredAsset[] = [
    mk(
      { title: "Bài Facebook: Bánh kem Tết vị trà xanh", type: "content", subtype: "facebook_post", status: "pending_review", campaign_id: TET.id, tags: ["tết", "facebook"], created_at: t(15), submitted_at: t(17) },
      [
        version(1, CREATOR, t(15), post("Bánh kem Tết vị trà xanh", "Tết này, đổi vị cho cả nhà!", "Bánh kem trà xanh Hương Việt mềm mịn, ít ngọt, hợp khẩu vị cả gia đình."), { job: true }),
        version(2, CREATOR, t(16), post("Bánh kem Tết vị trà xanh", "Tết này, đổi vị cho cả nhà!", "Bánh kem trà xanh Hương Việt mềm mịn, ít ngọt, giao tận nhà trong 2 giờ tại TP.HCM."), { note: "Thêm thông tin giao hàng" }),
      ],
    ),
    mk(
      { title: "Poster khuyến mãi Tết 20%", type: "image", subtype: "poster", status: "pending_review", campaign_id: TET.id, tags: ["tết", "poster"], created_at: t(14), submitted_at: t(16) },
      [version(1, CREATOR, t(14), { prompt: "Poster bánh kem Tết nền đỏ, chữ 'Giảm 20%'", preset: "poster", aspect_ratio: "4:5" }, { job: true, image: svgDataUrl("Poster Tết -20%", "#FEE2E2") })],
    ),
    mk(
      { title: "Video TikTok: Mở hộp bánh Tết", type: "video", subtype: "social_vertical", status: "approved", campaign_id: TET.id, tags: ["tết", "tiktok"], created_at: t(10), usage_rights: "Nhạc nền: thư viện nội bộ, license CC-BY đến 12/2027." },
      [version(1, CREATOR, t(10), { prompt: "Mở hộp bánh kem Tết", preset: "social_vertical", aspect_ratio: "9:16", script: "Cảnh 1: hộp bánh đỏ. Cảnh 2: cắt bánh." }, { job: true, video: true })],
      [],
    ),
    mk(
      { title: "Caption Instagram: Quà Tết cho đối tác", type: "content", subtype: "caption", status: "draft", campaign_id: TET.id, tags: ["tết"], created_at: t(18) },
      [version(1, CREATOR, t(18), post("Quà Tết cho đối tác", "Gửi lời chúc ngọt ngào", "Hộp quà bánh kem Hương Việt – món quà Tết tinh tế cho đối tác."), { job: true })],
    ),
    mk(
      { title: "Ảnh banner Back to school", type: "image", subtype: "banner", status: "changes_requested", campaign_id: SCHOOL.id, tags: ["back to school"], created_at: t(5) },
      [version(1, CREATOR, t(5), { prompt: "Banner học sinh cầm bánh su kem", preset: "banner", aspect_ratio: "16:9" }, { job: true, image: svgDataUrl("Back to school", "#DBEAFE") })],
    ),
    mk(
      { title: "Email: Ưu đãi tựu trường", type: "content", subtype: "email", status: "approved", campaign_id: SCHOOL.id, tags: ["email"], created_at: t(3) },
      [version(1, LEAD, t(3), post("Ưu đãi tựu trường", "Năm học mới, năng lượng mới", "Giảm 15% bánh su kem cho đơn hàng đầu tiên tháng 9."), { job: true })],
      [],
      LEAD,
    ),
    mk(
      { title: "Bài blog: Câu chuyện thương hiệu", type: "content", subtype: "blog", status: "pending_review", campaign_id: null, tags: ["thương hiệu"], created_at: t(12), submitted_at: t(13) },
      [version(1, LEAD, t(12), post("Câu chuyện Hương Việt", "Từ căn bếp nhỏ ở Đà Lạt", "Hương Việt bắt đầu từ một căn bếp nhỏ năm 2010."))],
      [],
      LEAD,
    ),
  ];
  // Review history consistent with statuses.
  for (const a of assets) {
    const cur = a.versions[a.versions.length - 1]!;
    if (a.summary.status === "pending_review" || a.summary.status === "approved" || a.summary.status === "changes_requested") {
      a.approvals.push({ id: id("80000000"), version_id: cur.id, version_no: cur.version_no, reviewer_id: a.summary.created_by, reviewer_name: a.summary.created_by_name, decision: "submitted", comment: null, checklist: a.summary.subtype === "poster" ? { typography_checked: true } : {}, created_at: a.summary.submitted_at ?? a.summary.created_at });
      a.summary.submitted_at ??= a.summary.created_at;
    }
    if (a.summary.status === "approved") {
      a.approvals.unshift({ id: id("80000000"), version_id: cur.id, version_no: cur.version_no, reviewer_id: REVIEWER.id, reviewer_name: REVIEWER.name, decision: "approved", comment: "Ổn, dùng được.", checklist: {}, created_at: a.summary.created_at });
    }
    if (a.summary.status === "changes_requested") {
      a.approvals.unshift({ id: id("80000000"), version_id: cur.id, version_no: cur.version_no, reviewer_id: REVIEWER.id, reviewer_name: REVIEWER.name, decision: "changes_requested", comment: "Logo bị che, đổi bố cục.", checklist: {}, created_at: a.summary.created_at });
    }
  }
  return assets;
}

let store: StoredAsset[] = seed();

/** Test helper: restore the seed data. */
export function resetLibraryMocks(): void {
  store = seed();
}

/** Test helper: read the in-memory store. */
export function libraryMockAssets(): readonly StoredAsset[] {
  return store;
}

// ---------------------------------------------------------------- helpers

const isLeadOrAdmin = (u: SessionUser) => u.roles.includes("admin") || u.roles.includes("marketing_lead");
const canEdit = (u: SessionUser, a: StoredAsset) => a.summary.created_by === u.id || isLeadOrAdmin(u);
const fold = (s: string) => s.normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/đ/g, "d").replace(/Đ/g, "D").toLowerCase();
const current = (a: StoredAsset) => a.versions[a.versions.length - 1]!;

function toSummary(a: StoredAsset): AssetSummaryDto {
  const cur = current(a);
  const thumb = cur.files.find((f) => f.kind === "image" && (f.role === "thumbnail" || f.role === "primary"));
  return { ...a.summary, current_version_no: cur.version_no, thumbnail_url: a.summary.type === "content" ? null : (thumb?.url ?? null) };
}

function toDetail(a: StoredAsset, u: SessionUser): AssetDetailDto {
  const status = a.summary.status;
  return {
    ...toSummary(a),
    current_version: current(a),
    approvals: [...a.approvals].sort((x, y) => y.created_at.localeCompare(x.created_at)),
    permissions: {
      can_edit: canEdit(u, a),
      can_submit: canEdit(u, a) && status === "draft",
      can_review: u.permissions.includes("asset.review") && status === "pending_review" && (a.summary.created_by !== u.id || u.roles.includes("admin")),
      can_download: u.permissions.includes("asset.download") && (status === "approved" || isLeadOrAdmin(u)),
    },
  };
}

function find(idParam: unknown): StoredAsset | undefined {
  return store.find((a) => a.summary.id === idParam && !a.deleted);
}

function paginate<T>(items: T[], url: URL) {
  const limit = Math.min(Math.max(Number(url.searchParams.get("limit") ?? 20) || 20, 1), 100);
  const offset = Number(url.searchParams.get("cursor") ?? 0) || 0;
  const slice = items.slice(offset, offset + limit);
  return page(slice, offset + limit < items.length ? String(offset + limit) : null);
}

const now = () => new Date().toISOString();

function addApproval(a: StoredAsset, u: SessionUser, decision: AssetApprovalDto["decision"], comment: string | null, checklist: Record<string, unknown> = {}) {
  const cur = current(a);
  a.approvals.push({ id: id("80000000"), version_id: cur.id, version_no: cur.version_no, reviewer_id: u.id, reviewer_name: u.name, decision, comment, checklist, created_at: now() });
}

function transition(a: StoredAsset, from: AssetStatus, to: AssetStatus) {
  if (a.summary.status !== from) {
    return fail(409, "INVALID_STATE_TRANSITION", "Không thể chuyển sang trạng thái này", [{ from: a.summary.status, to }]);
  }
  a.summary.status = to;
  a.summary.updated_at = now();
  return null;
}

async function jsonBody(request: Request): Promise<Record<string, unknown>> {
  const text = await request.text();
  if (!text) return {};
  try {
    return JSON.parse(text) as Record<string, unknown>;
  } catch {
    return {};
  }
}

// ---------------------------------------------------------------- handlers

export const handlers: HttpHandler[] = [
  rest.get(
    api("/assets"),
    withAuth("brand.view", ({ request }) => {
      const url = new URL(request.url);
      const p = url.searchParams;
      const q = fold(p.get("q") ?? "").trim();
      const statuses = (p.get("status") ?? "").split(",").filter(Boolean);
      const from = p.get("from");
      const to = p.get("to");
      const items = store
        .filter((a) => !a.deleted)
        .filter((a) => !p.get("type") || a.summary.type === p.get("type"))
        .filter((a) => !statuses.length || statuses.includes(a.summary.status))
        .filter((a) => !p.get("campaign_id") || a.summary.campaign_id === p.get("campaign_id"))
        .filter((a) => !p.get("brand_id") || a.summary.brand_id === p.get("brand_id"))
        .filter((a) => !p.get("created_by") || a.summary.created_by === p.get("created_by"))
        .filter((a) => !p.get("tag") || a.summary.tags.includes((p.get("tag") ?? "").toLowerCase()))
        .filter((a) => !from || a.summary.created_at >= from)
        .filter((a) => !to || a.summary.created_at.slice(0, 10) <= to)
        .filter((a) => {
          if (!q) return true;
          const hay = fold(`${a.summary.title} ${JSON.stringify(current(a).content_json)} ${a.summary.tags.join(" ")}`);
          return q.split(/\s+/).every((w) => hay.includes(w));
        })
        .sort((x, y) => y.summary.created_at.localeCompare(x.summary.created_at))
        .map(toSummary);
      return paginate(items, url);
    }),
  ),

  rest.get(
    api("/assets/:id"),
    withAuth("brand.view", ({ params, user }) => {
      const a = find(params.id);
      return a ? ok(toDetail(a, user)) : fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
    }),
  ),

  rest.patch(
    api("/assets/:id"),
    withAuth(null, async ({ params, request, user }) => {
      const a = find(params.id);
      if (!a || !user.permissions.includes("brand.view")) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      if (!canEdit(user, a)) return fail(403, "FORBIDDEN", "Thiếu quyền");
      const body = await jsonBody(request);
      if (body.title !== undefined) {
        if (typeof body.title !== "string" || !body.title.trim()) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "title", issue: "required" }]);
        a.summary.title = body.title.trim();
      }
      if (Array.isArray(body.tags)) a.summary.tags = (body.tags as string[]).map((t) => t.trim().toLowerCase()).filter(Boolean);
      if (body.usage_rights !== undefined) a.summary.usage_rights = (body.usage_rights as string | null) || null;
      a.summary.updated_at = now();
      return ok(toDetail(a, user));
    }),
  ),

  rest.delete(
    api("/assets/:id"),
    withAuth(null, ({ params, user }) => {
      const a = find(params.id);
      if (!a) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      if (!canEdit(user, a)) return fail(403, "FORBIDDEN", "Thiếu quyền");
      a.deleted = true;
      return noContent();
    }),
  ),

  rest.get(
    api("/assets/:id/versions"),
    withAuth("brand.view", ({ params }) => {
      const a = find(params.id);
      if (!a) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      return page([...a.versions].reverse(), null);
    }),
  ),

  rest.post(
    api("/assets/:id/versions/:versionNo/restore"),
    withAuth(null, ({ params, user }) => {
      const a = find(params.id);
      if (!a) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      if (!canEdit(user, a)) return fail(403, "FORBIDDEN", "Thiếu quyền");
      const src = a.versions.find((v) => v.version_no === Number(params.versionNo));
      if (!src) return fail(404, "NOT_FOUND", "Không tìm thấy phiên bản");
      if (a.summary.status === "archived") return fail(409, "INVALID_STATE_TRANSITION", "Không thể chuyển sang trạng thái này");
      const v: AssetVersionDto = {
        ...src,
        id: id("50000000"),
        version_no: current(a).version_no + 1,
        change_note: `Khôi phục từ phiên bản ${src.version_no}`,
        generated_by_job_id: null,
        provenance: null,
        created_by: user.id,
        created_by_name: user.name,
        created_at: now(),
      };
      a.versions.push(v);
      a.summary.status = "draft";
      a.summary.updated_at = now();
      return ok(v, { status: 201 });
    }),
  ),

  rest.post(
    api("/assets/:id/submit-review"),
    withAuth(null, async ({ params, request, user }) => {
      const a = find(params.id);
      if (!a) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      if (!canEdit(user, a)) return fail(403, "FORBIDDEN", "Thiếu quyền");
      const body = await jsonBody(request);
      const checklist = (body.checklist ?? {}) as Record<string, unknown>;
      if (a.summary.subtype === "poster" && checklist.typography_checked !== true) {
        return fail(400, "VALIDATION_FAILED", "Poster cần kiểm tra chữ trước khi gửi duyệt", [{ field: "checklist.typography_checked", issue: "required" }]);
      }
      const err = transition(a, "draft", "pending_review");
      if (err) return err;
      a.summary.submitted_at = now();
      addApproval(a, user, "submitted", typeof body.comment === "string" ? body.comment : null, checklist);
      return ok(toDetail(a, user));
    }),
  ),

  rest.post(
    api("/assets/:id/approve"),
    withAuth("asset.review", async ({ params, request, user }) => {
      const a = find(params.id);
      if (!a) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      if (a.summary.created_by === user.id && !user.roles.includes("admin")) {
        return fail(403, "SELF_APPROVAL_NOT_ALLOWED", "Không thể tự duyệt asset của mình");
      }
      const body = await jsonBody(request);
      const err = transition(a, "pending_review", "approved");
      if (err) return err;
      addApproval(a, user, "approved", typeof body.comment === "string" && body.comment.trim() ? body.comment.trim() : null);
      return ok(toDetail(a, user));
    }),
  ),

  rest.post(
    api("/assets/:id/request-changes"),
    withAuth("asset.review", async ({ params, request, user }) => {
      const a = find(params.id);
      if (!a) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      const body = await jsonBody(request);
      const comment = typeof body.comment === "string" ? body.comment.trim() : "";
      if (!comment) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "comment", issue: "required" }]);
      const err = transition(a, "pending_review", "changes_requested");
      if (err) return err;
      addApproval(a, user, "changes_requested", comment);
      return ok(toDetail(a, user));
    }),
  ),

  rest.get(
    api("/reviews/queue"),
    withAuth("asset.review", ({ request }) => {
      const url = new URL(request.url);
      const type = url.searchParams.get("type");
      const campaign = url.searchParams.get("campaign_id");
      const items = store
        .filter((a) => !a.deleted && a.summary.status === "pending_review")
        .filter((a) => !type || a.summary.type === type)
        .filter((a) => !campaign || a.summary.campaign_id === campaign)
        .sort((x, y) => (x.summary.submitted_at ?? "").localeCompare(y.summary.submitted_at ?? ""))
        .map(toSummary);
      return paginate(items, url);
    }),
  ),

  rest.post(
    api("/assets/:id/download-url"),
    withAuth("asset.download", async ({ params, request, user }) => {
      const a = find(params.id);
      if (!a) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      if (a.summary.status !== "approved" && !isLeadOrAdmin(user)) {
        return fail(403, "FORBIDDEN", "Chỉ tải được asset đã duyệt");
      }
      const body = await jsonBody(request);
      const files = isLeadOrAdmin(user) ? a.versions.flatMap((v) => v.files) : current(a).files;
      const file = body.file_id ? files.find((f) => f.file_id === body.file_id) : (current(a).files.find((f) => f.role === "primary") ?? current(a).files[0]);
      if (!file) return fail(404, "NOT_FOUND", "Asset không có tệp để tải");
      return ok<AssetDownloadUrlDto>({ url: file.url ?? "", expires_at: new Date(Date.now() + 15 * 60_000).toISOString(), file_id: file.file_id });
    }),
  ),

  // Fallback for the library campaign filter. The campaign module's own handler (registered
  // earlier in app/mocks/handlers.ts) takes precedence once it exists.
  rest.get(
    api("/campaigns"),
    withAuth("brand.view", () => page(MOCK_CAMPAIGNS.map((c) => ({ ...c })), null)),
  ),
];
