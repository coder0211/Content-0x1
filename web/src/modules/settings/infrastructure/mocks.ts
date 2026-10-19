import type { HttpHandler } from "msw";
import type { ActiveStatus, Role, UserStatus } from "@/shared/kernel";
import { api, fail, MOCK_USERS, ok, page, rest, withAuth } from "@/shared/api/mock";
import { uuid } from "@/shared/lib/id";
import type {
  AuditLogDto,
  CreateModelDto,
  CreatePricingDto,
  CreateUserDto,
  ModelDto,
  PricingVersionDto,
  ProviderDto,
  UpdateModelDto,
  UpdateProviderDto,
  UpdateUserDto,
  UserDto,
} from "./dto";

/** Internal store row: one unit of one pricing version (the API groups rows into versions). */
interface PricingRowStore {
  model_id: string;
  pricing_version: string;
  unit: string;
  unit_price: string;
  currency: string;
  effective_from: string;
  effective_to: string | null;
}

/* In-memory stores (reset with resetSettingsMocks). Mirrors SPEC-04 §2.1 / §2.9 and the audit API. */

const ROLE_CODES: Role[] = ["admin", "marketing_lead", "content_creator", "reviewer"];
const USER_STATUSES: UserStatus[] = ["active", "locked", "disabled"];
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

let users: UserDto[] = [];
let audit: AuditLogDto[] = [];
let providers: ProviderDto[] = [];
let models: ModelDto[] = [];
let pricing: PricingRowStore[] = [];
let auditSeq = 0;

const T0 = Date.parse("2026-10-18T08:00:00Z");
const iso = (minutesAgo: number) => new Date(T0 - minutesAgo * 60_000).toISOString();

function seed(): void {
  users = [
    ...MOCK_USERS.map((u, i) => ({
      id: u.id,
      email: u.email,
      name: u.name,
      status: "active" as UserStatus,
      roles: [...u.roles],
      locked_until: null,
      last_login_at: iso(30 + i * 7),
      created_at: iso(60 * 24 * 30),
    })),
    {
      id: "00000000-0000-4000-8000-000000000005",
      email: "intern@acs.local",
      name: "Võ Thanh Tâm",
      status: "locked",
      roles: ["content_creator"],
      locked_until: null,
      last_login_at: iso(60 * 24 * 3),
      created_at: iso(60 * 24 * 10),
    },
  ];

  providers = [
    provider("p-anthropic", "anthropic", "Anthropic Claude", ["text"], "PROVIDER_ANTHROPIC_API_KEY", true, { timeout_sec: 60 }),
    provider("p-openai", "openai", "OpenAI Images", ["image"], "PROVIDER_OPENAI_API_KEY", true, { timeout_sec: 120 }),
    provider("p-veo", "veo", "Google Veo", ["video"], "PROVIDER_VEO_API_KEY", false, { region: "us-central1" }),
    provider("p-kenburns", "kenburns", "Ken Burns (nội bộ)", ["video"], "NONE", true, {}),
    provider("p-tts", "gcp_tts", "Google Cloud TTS", ["voice"], "PROVIDER_GCP_TTS_API_KEY", true, { language: "vi-VN" }),
    provider("p-fake", "fake", "Fake (local/test)", ["text", "image", "video", "voice"], "NONE", true, {}, "disabled"),
  ];

  models = [
    model("m-claude", "p-anthropic", "anthropic", "claude-sonnet-5-5", "text", { languages: ["vi", "en"] }, 4, true),
    model("m-gpt-image", "p-openai", "openai", "gpt-image-1", "image", { aspect_ratios: ["1:1", "16:9", "9:16"], max_variants: 4, supports_ref_image: true }, 2, true),
    model("m-veo", "p-veo", "veo", "veo-3", "video", { aspect_ratios: ["16:9", "9:16"], max_duration_sec: 8 }, 2, true),
    model("m-kenburns", "p-kenburns", "kenburns", "kenburns", "video", { max_duration_sec: 8 }, 4, false),
    model("m-tts", "p-tts", "gcp_tts", "vi-VN-Neural2-A", "voice", { languages: ["vi"] }, 4, true),
  ];

  pricing = [
    price("m-claude", "anthropic-2026-07", "input_token", 0.000003, iso(60 * 24 * 100), iso(60 * 24 * 20)),
    price("m-claude", "anthropic-2026-07", "output_token", 0.000015, iso(60 * 24 * 100), iso(60 * 24 * 20)),
    price("m-claude", "anthropic-2026-10", "input_token", 0.0000028, iso(60 * 24 * 20), null),
    price("m-claude", "anthropic-2026-10", "output_token", 0.000014, iso(60 * 24 * 20), null),
    price("m-gpt-image", "openai-2026-09", "image_1024_standard", 0.04, iso(60 * 24 * 40), null),
    price("m-veo", "veo-2026-09", "video_second_1080p", 0.5, iso(60 * 24 * 40), null),
    price("m-tts", "gcp-2026-01", "char", 0.000016, iso(60 * 24 * 200), null),
  ];

  auditSeq = 0;
  audit = [];
  const actions: [string, string][] = [
    ["asset.approve", "asset"],
    ["asset.download", "asset"],
    ["budget.update", "budget"],
    ["user.create", "user"],
    ["provider.update", "provider"],
    ["role.assign", "user"],
  ];
  for (let i = 0; i < 60; i++) {
    const [action, resourceType] = actions[i % actions.length]!;
    const actor = MOCK_USERS[i % 2]!; // admin / lead
    audit.push({
      id: ++auditSeq,
      actor_id: actor.id,
      actor_email: actor.email,
      action,
      resource_type: resourceType,
      resource_id: resourceType === "budget" ? "global" : `00000000-0000-4000-9000-${String(i).padStart(12, "0")}`,
      before: action === "budget.update" ? { limit_amount: 1000, hard_stop: true } : null,
      after: action === "budget.update" ? { limit_amount: 1200, hard_stop: true } : { status: "approved" },
      ip: "10.0.0.12",
      request_id: `req-${i}`,
      created_at: iso(60 * 24 * 7 - i * 120),
    });
  }
  audit.reverse(); // newest first
}

function provider(
  id: string,
  code: string,
  name: string,
  kinds: ProviderDto["kinds"],
  secretRef: string,
  configured: boolean,
  config: Record<string, unknown>,
  status: ActiveStatus = "active",
): ProviderDto {
  return { id, code, name, kinds, status, secret_ref: secretRef, secret_configured: configured, adapter_available: true, config_json: config, updated_at: iso(60 * 24) };
}

function model(
  id: string,
  providerId: string,
  providerCode: string,
  code: string,
  kind: ModelDto["kind"],
  capabilities: Record<string, unknown>,
  concurrency: number,
  isDefault: boolean,
): ModelDto {
  return {
    id,
    provider_id: providerId,
    provider_code: providerCode,
    provider_status: "active",
    model_code: code,
    kind,
    capabilities,
    concurrency_limit: concurrency,
    is_default: isDefault,
    status: "active",
    adapter_available: true,
    usable: true,
  };
}

function price(modelId: string, version: string, unit: string, unitPrice: number, from: string, to: string | null): PricingRowStore {
  return { model_id: modelId, pricing_version: version, unit, unit_price: String(unitPrice), currency: "USD", effective_from: from, effective_to: to };
}

/** Groups store rows into `AIPricingVersion`s, newest effective_from first. */
function pricingVersions(modelId: string): PricingVersionDto[] {
  const now = Date.now();
  const byVersion = new Map<string, PricingRowStore[]>();
  for (const r of pricing) {
    if (r.model_id !== modelId) continue;
    byVersion.set(r.pricing_version, [...(byVersion.get(r.pricing_version) ?? []), r]);
  }
  return [...byVersion.values()]
    .map((rows) => {
      const f = rows[0]!;
      return {
        pricing_version: f.pricing_version,
        currency: f.currency,
        effective_from: f.effective_from,
        effective_to: f.effective_to,
        is_current: Date.parse(f.effective_from) <= now && (f.effective_to === null || Date.parse(f.effective_to) > now),
        items: rows.map((r) => ({ unit: r.unit, unit_price: r.unit_price })),
      };
    })
    .sort((a, b) => b.effective_from.localeCompare(a.effective_from));
}

function record(actor: { id: string; email: string }, action: string, resourceType: string, resourceId: string, before: unknown, after: unknown) {
  audit.unshift({
    id: ++auditSeq,
    actor_id: actor.id,
    actor_email: actor.email,
    action,
    resource_type: resourceType,
    resource_id: resourceId,
    before: before ?? null,
    after: after ?? null,
    ip: "127.0.0.1",
    request_id: uuid(),
    created_at: new Date().toISOString(),
  });
}

/** Offset cursor over an array (opaque to the client). */
function paginate<T>(items: T[], url: URL, defaultLimit = 20) {
  const limit = Math.min(Math.max(Number(url.searchParams.get("limit") ?? defaultLimit) || defaultLimit, 1), 100);
  const offset = Number(url.searchParams.get("cursor") ?? 0) || 0;
  const slice = items.slice(offset, offset + limit);
  const next = offset + limit < items.length ? String(offset + limit) : null;
  return page(slice, next);
}

/** Secrets are never part of a response — strip anything that is not on the DTO allowlist. */
function publicProvider(p: ProviderDto): ProviderDto {
  const { id, code, name, kinds, status, secret_ref, secret_configured, adapter_available, config_json, updated_at } = p;
  return { id, code, name, kinds, status, secret_ref, secret_configured, adapter_available, config_json, updated_at };
}

seed();

export const handlers: HttpHandler[] = [
  // ---- Users (identity) ----
  rest.get(
    api("/users"),
    withAuth("user.manage", ({ request }) => {
      const url = new URL(request.url);
      const q = (url.searchParams.get("q") ?? "").toLowerCase();
      const status = url.searchParams.get("status");
      if (status && !USER_STATUSES.includes(status as UserStatus)) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "status", issue: "invalid" }]);
      }
      const items = users.filter(
        (u) => (!q || u.email.toLowerCase().includes(q) || u.name.toLowerCase().includes(q)) && (!status || u.status === status),
      );
      return paginate(items, url, 20);
    }),
  ),

  rest.post(
    api("/users"),
    withAuth("user.manage", async ({ request, user }) => {
      const b = (await request.json()) as Partial<CreateUserDto>;
      const details: { field: string; issue: string }[] = [];
      const email = (b.email ?? "").trim().toLowerCase();
      if (!email) details.push({ field: "email", issue: "required" });
      else if (!EMAIL_RE.test(email)) details.push({ field: "email", issue: "invalid" });
      if (!(b.name ?? "").trim()) details.push({ field: "name", issue: "required" });
      if (!b.role_codes?.length) details.push({ field: "role_codes", issue: "required" });
      else if (b.role_codes.some((r) => !ROLE_CODES.includes(r))) details.push({ field: "role_codes", issue: "invalid" });
      if ((b.temp_password ?? "").length < 10) details.push({ field: "temp_password", issue: "too_short" });
      if (details.length) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", details);
      if (users.some((u) => u.email === email)) {
        return fail(409, "CONFLICT", "Email đã tồn tại", [{ field: "email", issue: "taken" }]);
      }
      const created: UserDto = {
        id: uuid(),
        email,
        name: b.name!.trim(),
        status: "active",
        roles: [...new Set(b.role_codes!)],
        locked_until: null,
        last_login_at: null,
        created_at: new Date().toISOString(),
      };
      users.push(created);
      record(user, "user.create", "user", created.id, null, { email, roles: created.roles });
      return ok(created, { status: 201 });
    }),
  ),

  rest.patch(
    api("/users/:id"),
    withAuth<{ id: string }>("user.manage", async ({ request, params, user }) => {
      const target = users.find((u) => u.id === params.id);
      if (!target) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      const b = (await request.json()) as UpdateUserDto;
      if (b.status && !USER_STATUSES.includes(b.status)) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "status", issue: "invalid" }]);
      }
      if (b.role_codes && b.role_codes.length === 0) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "role_codes", issue: "required" }]);
      }
      if (target.id === user.id && ((b.status && b.status !== "active") || (b.role_codes && !b.role_codes.includes("admin")))) {
        return fail(403, "FORBIDDEN", "Không thể tự khóa hoặc gỡ quyền Admin của chính mình");
      }
      const before = { name: target.name, status: target.status, roles: target.roles };
      if (b.name !== undefined) target.name = b.name.trim();
      if (b.status !== undefined) target.status = b.status;
      if (b.role_codes !== undefined) target.roles = [...new Set(b.role_codes)];
      record(user, b.role_codes ? "role.assign" : "user.update", "user", target.id, before, {
        name: target.name,
        status: target.status,
        roles: target.roles,
      });
      return ok(target);
    }),
  ),

  // ---- Audit log ----
  rest.get(
    api("/audit-logs"),
    withAuth("audit.view", ({ request }) => {
      const url = new URL(request.url);
      const sp = url.searchParams;
      const actor = sp.get("actor_id");
      const rt = sp.get("resource_type");
      const rid = sp.get("resource_id");
      const action = sp.get("action");
      const from = sp.get("from");
      const to = sp.get("to");
      const items = audit.filter(
        (e) =>
          (!actor || e.actor_id === actor) &&
          (!rt || e.resource_type === rt) &&
          (!rid || e.resource_id === rid) &&
          (!action || e.action === action) &&
          (!from || e.created_at >= from) &&
          (!to || e.created_at < to),
      );
      return paginate(items, url, 20);
    }),
  ),

  // ---- Providers / models / pricing ----
  rest.get(api("/settings/providers"), withAuth("provider.view", () => ok(providers.map(publicProvider)))),

  rest.patch(
    api("/settings/providers/:id"),
    withAuth<{ id: string }>("provider.manage", async ({ request, params, user }) => {
      const p = providers.find((x) => x.id === params.id);
      if (!p) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      const b = (await request.json()) as UpdateProviderDto;
      if (b.status && b.status !== "active" && b.status !== "disabled") {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "status", issue: "invalid" }]);
      }
      if (b.secret_ref !== undefined && !b.secret_ref.trim()) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "secret_ref", issue: "required" }]);
      }
      const before = { status: p.status, secret_ref: p.secret_ref, config_json: p.config_json };
      if (b.status) p.status = b.status;
      if (b.secret_ref !== undefined) {
        p.secret_ref = b.secret_ref.trim();
        p.secret_configured = p.secret_ref.startsWith("PROVIDER_");
      }
      if (b.config_json !== undefined) p.config_json = b.config_json;
      p.updated_at = new Date().toISOString();
      record(user, "provider.update", "provider", p.id, before, { status: p.status, secret_ref: p.secret_ref, config_json: p.config_json });
      return ok(publicProvider(p));
    }),
  ),

  rest.get(
    api("/settings/models"),
    withAuth("provider.view", ({ request }) => {
      const kind = new URL(request.url).searchParams.get("kind");
      return ok(models.filter((m) => !kind || m.kind === kind));
    }),
  ),

  rest.post(
    api("/settings/models"),
    withAuth("provider.manage", async ({ request, user }) => {
      const b = (await request.json()) as Partial<CreateModelDto>;
      const p = providers.find((x) => x.id === b.provider_id);
      const details: { field: string; issue: string }[] = [];
      if (!p) details.push({ field: "provider_id", issue: "invalid" });
      if (!(b.model_code ?? "").trim()) details.push({ field: "model_code", issue: "required" });
      if (!b.kind || !["text", "image", "video", "voice"].includes(b.kind)) details.push({ field: "kind", issue: "invalid" });
      if (!b.concurrency_limit || b.concurrency_limit < 1) details.push({ field: "concurrency_limit", issue: "min" });
      if (details.length) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", details);
      if (models.some((m) => m.provider_id === b.provider_id && m.model_code === b.model_code!.trim())) {
        return fail(409, "CONFLICT", "Model đã tồn tại", [{ field: "model_code", issue: "taken" }]);
      }
      const m = model(uuid(), p!.id, p!.code, b.model_code!.trim(), b.kind!, b.capabilities ?? {}, b.concurrency_limit!, Boolean(b.is_default));
      if (m.is_default) for (const o of models) if (o.kind === m.kind) o.is_default = false;
      models.push(m);
      record(user, "model.create", "model", m.id, null, m);
      return ok(m, { status: 201 });
    }),
  ),

  rest.patch(
    api("/settings/models/:id"),
    withAuth<{ id: string }>("provider.manage", async ({ request, params, user }) => {
      const m = models.find((x) => x.id === params.id);
      if (!m) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      const b = (await request.json()) as UpdateModelDto;
      if (b.concurrency_limit !== undefined && (!Number.isInteger(b.concurrency_limit) || b.concurrency_limit < 1)) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "concurrency_limit", issue: "min" }]);
      }
      const before = { ...m };
      if (b.capabilities !== undefined) m.capabilities = b.capabilities;
      if (b.concurrency_limit !== undefined) m.concurrency_limit = b.concurrency_limit;
      if (b.status !== undefined) m.status = b.status;
      if (b.is_default !== undefined) {
        if (b.is_default) for (const o of models) if (o.kind === m.kind) o.is_default = false;
        m.is_default = b.is_default;
      }
      record(user, "model.update", "model", m.id, before, m);
      return ok(m);
    }),
  ),

  rest.get(
    api("/settings/models/:id/pricing"),
    withAuth<{ id: string }>("provider.view", ({ params }) => {
      if (!models.some((m) => m.id === params.id)) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      return ok(pricingVersions(params.id));
    }),
  ),

  rest.post(
    api("/settings/models/:id/pricing"),
    withAuth<{ id: string }>("provider.manage", async ({ request, params, user }) => {
      if (!models.some((m) => m.id === params.id)) return fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu");
      const b = (await request.json()) as Partial<CreatePricingDto>;
      const details: { field: string; issue: string }[] = [];
      const version = (b.pricing_version ?? "").trim();
      if (!version) details.push({ field: "pricing_version", issue: "required" });
      const from = b.effective_from ?? new Date().toISOString();
      if (Number.isNaN(Date.parse(from))) details.push({ field: "effective_from", issue: "invalid" });
      else if (Date.parse(from) < Date.now() - 5 * 60_000) details.push({ field: "effective_from", issue: "in_past" });
      if (!b.items?.length) details.push({ field: "items", issue: "required" });
      else if (b.items.some((p) => !p.unit || !(Number(p.unit_price) >= 0))) details.push({ field: "items", issue: "invalid" });
      if (details.length) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", details);
      if (pricing.some((p) => p.model_id === params.id && p.pricing_version === version)) {
        return fail(409, "CONFLICT", "Phiên bản giá đã tồn tại", [{ field: "pricing_version", issue: "taken" }]);
      }
      // Append-only: close the current version (effective_to), never edit its prices.
      for (const p of pricing) if (p.model_id === params.id && p.effective_to === null) p.effective_to = from;
      const rows = b.items!.map((p) => price(params.id, version, p.unit, Number(p.unit_price), from, null));
      for (const r of rows) r.currency = b.currency ?? "USD";
      pricing.push(...rows);
      record(user, "pricing.create", "model", params.id, null, { pricing_version: version, items: b.items });
      return ok(pricingVersions(params.id).find((v) => v.pricing_version === version)!, { status: 201 });
    }),
  ),
];

/** Test helper: restore seeded stores. */
export function resetSettingsMocks(): void {
  seed();
}
