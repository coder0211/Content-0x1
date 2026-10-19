import type { HttpHandler } from "msw";
import { api, fail, HttpResponse, noContent, ok, page, rest, withAuth } from "@/shared/api/mock";
import type { ApiErrorDetail } from "@/shared/kernel";
import { uuid } from "@/shared/lib/id";
import { businessToday, isHexColor, offerStatus } from "../domain/rules";
import type { BrandAssetDto, BrandDto, BrandProfileDto, BrandVersionDto, ProductDto, ProductFactsDto, UploadTicketDto } from "./dto";

/** Fixed id of the mock sample brand (campaign mocks reference the same id). */
export const MOCK_BRAND_ID = "10000000-0000-4000-8000-000000000001";
export const MOCK_PRODUCT_ID = "10000000-0000-4000-8000-000000000101";
/** Seed music brand assets (the video mocks reject the unlicensed one with MUSIC_NOT_LICENSED). */
export const MOCK_MUSIC_LICENSED_ID = "ba000000-0000-4000-8000-000000000001";
export const MOCK_MUSIC_UNLICENSED_ID = "ba000000-0000-4000-8000-000000000002";
const LEAD_ID = "00000000-0000-4000-8000-000000000002";

interface Store {
  brands: Map<string, { brand: Omit<BrandDto, "current_version">; versions: BrandVersionDto[]; deleted: boolean }>;
  products: Map<string, ProductDto>;
  assets: Map<string, BrandAssetDto>;
  files: Map<string, { name: string; mime: string; size: number; complete: boolean }>;
}

const store: Store = { brands: new Map(), products: new Map(), assets: new Map(), files: new Map() };

function emptyProfileDto(): BrandProfileDto {
  return {
    description: "",
    colors: [],
    typography: { heading: "", body: "" },
    tone_of_voice: [],
    preferred_words: [],
    forbidden_words: [],
    default_ctas: [],
    audience_personas: [],
    notes: "",
  };
}

function seed() {
  const now = "2026-10-01T02:00:00Z";
  const profile: BrandProfileDto = {
    ...emptyProfileDto(),
    description: "Trung tâm tiếng Anh giao tiếp cho sinh viên.",
    colors: [
      { name: "primary", hex: "#0055FF" },
      { name: "text", hex: "#1F2937" },
    ],
    typography: { heading: "Be Vietnam Pro", body: "Be Vietnam Pro" },
    tone_of_voice: ["thân thiện", "đáng tin cậy"],
    preferred_words: ["lộ trình rõ ràng"],
    forbidden_words: ["cam kết 100%", "rẻ nhất"],
    default_ctas: ["Đăng ký tư vấn"],
    audience_personas: [{ name: "Sinh viên năm 2–4", pain_points: ["ngại nói"], motivations: ["xin việc"] }],
  };
  store.brands.set(MOCK_BRAND_ID, {
    brand: { id: MOCK_BRAND_ID, name: "Anh ngữ Mẫu", created_by: LEAD_ID, created_at: now, updated_at: now },
    versions: [{ id: uuid(), version_no: 1, profile, created_by: LEAD_ID, created_at: now }],
    deleted: false,
  });
  store.products.set(MOCK_PRODUCT_ID, {
    id: MOCK_PRODUCT_ID,
    brand_id: MOCK_BRAND_ID,
    name: "Khóa giao tiếp 12 tuần",
    description: "Lớp tối, 3 buổi/tuần.",
    facts: {
      usp: ["Lớp tối đa 12 học viên"],
      price: { amount: 2990000, currency: "VND", note: "học phí trọn khóa" },
      offers: [
        { text: "Giảm 20% đến 31/12", valid_from: "2026-10-01", valid_to: "2026-12-31" },
        { text: "Tặng giáo trình tháng 9", valid_from: "2026-09-01", valid_to: "2026-09-30" },
      ],
      policies: ["Hoàn phí trong 7 ngày"],
      proof: [{ type: "testimonial", text: "Tự tin phỏng vấn sau 3 tháng.", source: "Học viên K12", verified: true }],
    },
    status: "active",
    created_by: LEAD_ID,
    created_at: now,
    updated_at: now,
  });
  // Music tracks (OQ-04): one licensed, one without licence — Video Studio demos MUSIC_NOT_LICENSED.
  const track = (id: string, label: string, license: BrandAssetDto["license"]): BrandAssetDto => ({
    id,
    brand_id: MOCK_BRAND_ID,
    file_id: id.replace(/^ba/, "fa"),
    asset_type: "music",
    label,
    license,
    license_valid: !!license,
    file: { mime_type: "audio/mpeg", original_name: `${label}.mp3`, size_bytes: 2_400_000, duration_ms: 30_000, url: null, url_expires_at: null },
    created_by: LEAD_ID,
    created_at: now,
  });
  store.assets.set(MOCK_MUSIC_LICENSED_ID, track(MOCK_MUSIC_LICENSED_ID, "Nhạc nền tươi sáng (có license)", { source: "Thư viện nội bộ", license_type: "royalty_free" }));
  store.assets.set(MOCK_MUSIC_UNLICENSED_ID, track(MOCK_MUSIC_UNLICENSED_ID, "Bản demo chưa có license", null));
}

export function resetBrandMocks(): void {
  store.brands.clear();
  store.products.clear();
  store.assets.clear();
  store.files.clear();
  seed();
}
resetBrandMocks();

const validation = (details: ApiErrorDetail[]) => fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", details);
const notFound = () => fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");

function brandDto(id: string): BrandDto | null {
  const b = store.brands.get(id);
  if (!b || b.deleted) return null;
  return { ...b.brand, current_version: b.versions[b.versions.length - 1]! };
}

function normalizeProfile(p: Partial<BrandProfileDto> | undefined): BrandProfileDto {
  const base = emptyProfileDto();
  const out = { ...base, ...(p ?? {}) };
  out.colors = (out.colors ?? []).map((c) => ({ name: c.name.trim(), hex: c.hex.trim().toUpperCase() }));
  return out;
}

function profileIssues(p: BrandProfileDto): ApiErrorDetail[] {
  const d: ApiErrorDetail[] = [];
  p.colors.forEach((c, i) => {
    if (!c.name) d.push({ field: `profile.colors[${i}].name`, issue: "required" });
    if (!isHexColor(c.hex)) d.push({ field: `profile.colors[${i}].hex`, issue: "invalid_hex" });
  });
  p.audience_personas.forEach((x, i) => {
    if (!x.name?.trim()) d.push({ field: `profile.audience_personas[${i}].name`, issue: "required" });
  });
  return d;
}

function withOfferStatus(p: ProductDto): ProductDto {
  const today = businessToday();
  return {
    ...p,
    facts: { ...p.facts, offers: p.facts.offers.map((o) => ({ ...o, status: offerStatus({ validFrom: o.valid_from, validTo: o.valid_to }, today) })) },
  };
}

function factsIssues(f: ProductFactsDto): ApiErrorDetail[] {
  const d: ApiErrorDetail[] = [];
  f.offers.forEach((o, i) => {
    if (!o.text?.trim()) d.push({ field: `facts.offers[${i}].text`, issue: "required" });
    if (o.valid_from && o.valid_to && o.valid_from > o.valid_to) d.push({ field: `facts.offers[${i}].valid_to`, issue: "before_valid_from" });
  });
  f.proof.forEach((p, i) => {
    if (!p.text?.trim()) d.push({ field: `facts.proof[${i}].text`, issue: "required" });
  });
  if (f.price && f.price.amount < 0) d.push({ field: "facts.price.amount", issue: "negative" });
  return d;
}

function emptyFactsDto(): ProductFactsDto {
  return { usp: [], price: null, offers: [], policies: [], proof: [] };
}

export const handlers: HttpHandler[] = [
  rest.get(
    api("/brands"),
    withAuth("brand.view", ({ request }) => {
      const q = (new URL(request.url).searchParams.get("q") ?? "").toLowerCase();
      const items = [...store.brands.keys()]
        .map(brandDto)
        .filter((b): b is BrandDto => !!b && b.name.toLowerCase().includes(q))
        .sort((a, b) => b.created_at.localeCompare(a.created_at));
      return page(items);
    }),
  ),

  rest.post(
    api("/brands"),
    withAuth("brand.manage", async ({ request, user }) => {
      const body = (await request.json()) as { name?: string; profile?: Partial<BrandProfileDto> };
      const name = (body.name ?? "").trim();
      const profile = normalizeProfile(body.profile);
      const issues = [...(name ? [] : [{ field: "name", issue: "required" }]), ...profileIssues(profile)];
      if (issues.length) return validation(issues);
      if ([...store.brands.values()].some((b) => !b.deleted && b.brand.name.toLowerCase() === name.toLowerCase())) {
        return fail(409, "CONFLICT", "Tên brand đã tồn tại", [{ field: "name", issue: "duplicate" }]);
      }
      const id = uuid();
      const now = new Date().toISOString();
      store.brands.set(id, {
        brand: { id, name, created_by: user.id, created_at: now, updated_at: now },
        versions: [{ id: uuid(), version_no: 1, profile, created_by: user.id, created_at: now }],
        deleted: false,
      });
      return ok(brandDto(id), { status: 201 });
    }),
  ),

  rest.get(
    api("/brands/:id"),
    withAuth("brand.view", ({ params }) => {
      const b = brandDto(String(params.id));
      return b ? ok(b) : notFound();
    }),
  ),

  rest.patch(
    api("/brands/:id"),
    withAuth("brand.manage", async ({ params, request, user }) => {
      const entry = store.brands.get(String(params.id));
      if (!entry || entry.deleted) return notFound();
      const body = (await request.json()) as { name?: string; profile?: Partial<BrandProfileDto>; expected_version_no?: number };
      if (!body.expected_version_no) return validation([{ field: "expected_version_no", issue: "required" }]);
      const current = entry.versions[entry.versions.length - 1]!;
      const profile = body.profile ? normalizeProfile(body.profile) : null;
      if (profile) {
        const issues = profileIssues(profile);
        if (issues.length) return validation(issues);
      }
      if (body.expected_version_no !== current.version_no) {
        return fail(409, "VERSION_CONFLICT", "Dữ liệu đã được người khác cập nhật", [
          { current_version_no: current.version_no, expected_version_no: body.expected_version_no },
        ]);
      }
      const now = new Date().toISOString();
      if (body.name !== undefined && body.name.trim()) entry.brand.name = body.name.trim();
      if (profile && JSON.stringify(profile) !== JSON.stringify(current.profile)) {
        entry.versions.push({ id: uuid(), version_no: current.version_no + 1, profile, created_by: user.id, created_at: now });
      }
      entry.brand.updated_at = now;
      return ok(brandDto(entry.brand.id));
    }),
  ),

  rest.get(
    api("/brands/:id/versions"),
    withAuth("brand.view", ({ params }) => {
      const entry = store.brands.get(String(params.id));
      if (!entry || entry.deleted) return notFound();
      return page([...entry.versions].reverse());
    }),
  ),

  rest.get(
    api("/brands/:id/products"),
    withAuth("brand.view", ({ params, request }) => {
      if (!brandDto(String(params.id))) return notFound();
      const status = new URL(request.url).searchParams.get("status");
      const items = [...store.products.values()]
        .filter((p) => p.brand_id === params.id && (!status || p.status === status))
        .map(withOfferStatus);
      return page(items);
    }),
  ),

  rest.post(
    api("/brands/:id/products"),
    withAuth("brand.manage", async ({ params, request, user }) => {
      if (!brandDto(String(params.id))) return notFound();
      const body = (await request.json()) as Partial<ProductDto>;
      const facts = { ...emptyFactsDto(), ...(body.facts ?? {}) };
      const issues = [...(body.name?.trim() ? [] : [{ field: "name", issue: "required" }]), ...factsIssues(facts)];
      if (issues.length) return validation(issues);
      const now = new Date().toISOString();
      const p: ProductDto = {
        id: uuid(),
        brand_id: String(params.id),
        name: body.name!.trim(),
        description: body.description ?? "",
        facts,
        status: "active",
        created_by: user.id,
        created_at: now,
        updated_at: now,
      };
      store.products.set(p.id, p);
      return ok(withOfferStatus(p), { status: 201 });
    }),
  ),

  rest.get(
    api("/products/:id"),
    withAuth("brand.view", ({ params }) => {
      const p = store.products.get(String(params.id));
      return p ? ok(withOfferStatus(p)) : notFound();
    }),
  ),

  rest.patch(
    api("/products/:id"),
    withAuth("brand.manage", async ({ params, request }) => {
      const p = store.products.get(String(params.id));
      if (!p) return notFound();
      const body = (await request.json()) as Partial<ProductDto>;
      const next: ProductDto = { ...p, ...body, facts: body.facts ? { ...emptyFactsDto(), ...body.facts } : p.facts, updated_at: new Date().toISOString() };
      const issues = [...(next.name?.trim() ? [] : [{ field: "name", issue: "required" }]), ...factsIssues(next.facts)];
      if (issues.length) return validation(issues);
      store.products.set(p.id, next);
      return ok(withOfferStatus(next));
    }),
  ),

  rest.get(
    api("/brands/:id/assets"),
    withAuth("brand.view", ({ params }) => {
      if (!brandDto(String(params.id))) return notFound();
      return page([...store.assets.values()].filter((a) => a.brand_id === params.id));
    }),
  ),

  rest.post(
    api("/brands/:id/assets"),
    withAuth("brand.manage", async ({ params, request, user }) => {
      if (!brandDto(String(params.id))) return notFound();
      const body = (await request.json()) as { file_id?: string; asset_type?: BrandAssetDto["asset_type"]; label?: string; license?: BrandAssetDto["license"] };
      const issues: ApiErrorDetail[] = [];
      if (!body.file_id) issues.push({ field: "file_id", issue: "required" });
      if (!body.asset_type) issues.push({ field: "asset_type", issue: "invalid" });
      if (body.asset_type === "music" && !body.license) issues.push({ field: "license", issue: "required" });
      if (body.license && !body.license.source?.trim()) issues.push({ field: "license.source", issue: "required" });
      if (body.license && !body.license.license_type?.trim()) issues.push({ field: "license.license_type", issue: "required" });
      if (issues.length) return validation(issues);
      const f = store.files.get(body.file_id!);
      if (!f || !f.complete) return notFound();
      const lic = body.license ?? null;
      const a: BrandAssetDto = {
        id: uuid(),
        brand_id: String(params.id),
        file_id: body.file_id!,
        asset_type: body.asset_type!,
        label: body.label ?? "",
        license: lic,
        license_valid: !!lic && (!lic.expires_at || lic.expires_at >= businessToday()),
        file: { mime_type: f.mime, original_name: f.name, size_bytes: f.size, url: null, url_expires_at: null },
        created_by: user.id,
        created_at: new Date().toISOString(),
      };
      store.assets.set(a.id, a);
      return ok(a, { status: 201 });
    }),
  ),

  rest.delete(
    api("/brands/:id/assets/:assetId"),
    withAuth("brand.manage", ({ params }) => {
      const a = store.assets.get(String(params.assetId));
      if (!a || a.brand_id !== params.id) return notFound();
      store.assets.delete(a.id);
      return noContent();
    }),
  ),

  // File module upload flow (no FE file module yet — see DECISIONS "Agent 6").
  rest.post(
    api("/files/upload-url"),
    withAuth(null, async ({ request }) => {
      const body = (await request.json()) as { filename?: string; mime_type?: string; size_bytes?: number; purpose?: string };
      if (!body.filename || !body.mime_type || !body.size_bytes || !body.purpose) {
        return validation([{ field: "filename", issue: "required" }]);
      }
      if (body.size_bytes > 50 * 1024 * 1024) return fail(413, "FILE_TOO_LARGE", "File quá lớn");
      const id = uuid();
      store.files.set(id, { name: body.filename, mime: body.mime_type, size: body.size_bytes, complete: false });
      const ticket: UploadTicketDto = {
        file_id: id,
        url: `${window.location.origin}/mock-storage/${id}`,
        method: "PUT",
        headers: { "Content-Type": body.mime_type },
        expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
      };
      return ok(ticket);
    }),
  ),
  rest.put("/mock-storage/:id", () => new HttpResponse(null, { status: 200 })),
  rest.post(
    api("/files/:id/complete"),
    withAuth(null, ({ params }) => {
      const f = store.files.get(String(params.id));
      if (!f) return notFound();
      f.complete = true;
      return ok({ id: params.id, mime_type: f.mime, size_bytes: f.size, original_name: f.name, upload_status: "ready" });
    }),
  ),
];
