import type { HttpHandler } from "msw";
import { api, fail, MOCK_USERS, ok, rest, withAuth } from "@/shared/api/mock";
import { GROUP_BYS, type CostSource, type GroupBy } from "../domain/budget";
import type { BudgetDto, BudgetSettingsDto, EstimateDto, EstimateRequestDto, UsageSummaryDto, UsageTotalsDto } from "./dto";

/** Mirrors the fake-provider seed pricing (migration 0011). */
const PRICES: Record<string, Record<string, number>> = {
  "fake-text": { input_token: 0.000003, output_token: 0.000015 },
  "fake-image": { image_1024_standard: 0.04, image_1024_high: 0.17 },
  "fake-video": { video_second_720p: 0.1, video_second_1080p: 0.15 },
  "fake-voice": { char: 0.000016 },
};
const DEFAULT_MODEL = { text: "fake-text", image: "fake-image", video: "fake-video", voice: "fake-voice" } as const;

const CAMP_A = { id: "c0000000-0000-4000-8000-000000000001", name: "Ra mắt Thu 2026" };
const CAMP_B = { id: "c0000000-0000-4000-8000-000000000002", name: "Tết 2027" };
const CAMPAIGNS = [CAMP_A, CAMP_B];

interface MockUsage {
  occurredAt: string;
  campaignId: string | null;
  provider: string;
  model: string;
  userId: string;
  jobType: string;
  cost: number;
  source: CostSource;
}

const r6 = (n: number) => Math.round(n * 1e6) / 1e6;
const iso = (d: Date) => d.toISOString().slice(0, 10);

function monthStart(now = new Date()) {
  return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1));
}

function seedUsage(): MockUsage[] {
  const out: MockUsage[] = [];
  const start = monthStart();
  const today = new Date();
  const days = Math.max(1, today.getUTCDate());
  const combos: Omit<MockUsage, "occurredAt" | "campaignId" | "userId">[] = [
    { provider: "fake", model: "fake-text", jobType: "content_generation", cost: 0.0213, source: "calculated" },
    { provider: "fake", model: "fake-image", jobType: "image_generation", cost: 0.34, source: "provider_reported" },
    { provider: "fake", model: "fake-video", jobType: "video_production", cost: 3.6, source: "calculated" },
    { provider: "fake", model: "fake-voice", jobType: "video_production", cost: 0.0067, source: "estimated" },
  ];
  for (let d = 0; d < days; d++) {
    const date = new Date(start.getTime() + d * 86_400_000);
    combos.forEach((c, i) => {
      if ((d + i) % 3 === 0) return;
      out.push({
        ...c,
        occurredAt: `${iso(date)}T0${i + 1}:00:00Z`,
        campaignId: i === 3 ? null : ((d + i) % 2 === 0 ? CAMP_A : CAMP_B).id,
        userId: MOCK_USERS[(d + i) % 3]?.id ?? "",
        cost: r6(c.cost * (1 + (d % 4) / 10)),
      });
    });
  }
  return out;
}

function budgetDto(b: Omit<BudgetDto, "remaining_amount" | "percent_used">): BudgetDto {
  const committed = b.spent_amount + b.reserved_amount;
  return {
    ...b,
    remaining_amount: r6(b.limit_amount - committed),
    percent_used: b.limit_amount === 0 ? (committed > 0 ? 100 : 0) : Math.round((committed / b.limit_amount) * 10000) / 100,
  };
}

interface State {
  settings: Omit<BudgetSettingsDto, "global" | "campaigns">;
  global: Omit<BudgetDto, "remaining_amount" | "percent_used">;
  campaigns: Omit<BudgetDto, "remaining_amount" | "percent_used">[];
  usage: MockUsage[];
}

function initialState(): State {
  const usage = seedUsage();
  const now = new Date();
  const start = monthStart(now);
  const end = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + 1, 0));
  const spent = r6(usage.reduce((a, u) => a + u.cost, 0));
  const stamp = now.toISOString();
  return {
    settings: {
      currency: "USD",
      per_job_limits: { text: 0.5, image: 2, video: 8 },
      global_template: { monthly_limit: 1000, alert_thresholds: [80, 100], hard_stop: true },
      max_concurrent_video_jobs_per_user: 3,
      updated_by: null,
      updated_at: stamp,
    },
    global: {
      id: "b0000000-0000-4000-8000-000000000001",
      scope: "global",
      campaign_id: null,
      campaign_name: null,
      period_start: iso(start),
      period_end: iso(end),
      limit_amount: 1000,
      reserved_amount: 12.5,
      spent_amount: spent,
      currency: "USD",
      alert_thresholds: [80, 100],
      alerts_sent: [],
      hard_stop: true,
      updated_at: stamp,
    },
    campaigns: [
      {
        id: "b0000000-0000-4000-8000-000000000002",
        scope: "campaign",
        campaign_id: CAMP_A.id,
        campaign_name: CAMP_A.name,
        period_start: iso(start),
        period_end: iso(new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + 2, 0))),
        limit_amount: 150,
        reserved_amount: 0,
        spent_amount: r6(usage.filter((u) => u.campaignId === CAMP_A.id).reduce((a, u) => a + u.cost, 0)),
        currency: "USD",
        alert_thresholds: [80, 100],
        alerts_sent: [80],
        hard_stop: true,
        updated_at: stamp,
      },
    ],
    usage,
  };
}

let state = initialState();

/** Reset mock state (tests: `afterEach(resetBudgetMocks)`). */
export function resetBudgetMocks(): void {
  state = initialState();
}

/** Replace the mock usage records (tests). */
export function setMockUsage(rows: MockUsage[]): void {
  state.usage = rows;
}
export type { MockUsage };

function settingsDto(): BudgetSettingsDto {
  return { ...state.settings, global: budgetDto(state.global), campaigns: state.campaigns.map(budgetDto) };
}

function estimate(req: EstimateRequestDto): EstimateDto | { missing: string } {
  const kind = req.kind;
  const model: string = req.model || (DEFAULT_MODEL as Record<string, string>)[kind] || DEFAULT_MODEL.text;
  const provider = req.provider || "fake";
  const lines: EstimateDto["breakdown"] = [];
  const add = (item: string, m: string, unit: string, units: number) => {
    const price = PRICES[m]?.[unit];
    if (price === undefined) return unit;
    lines.push({ item, provider, model: m, unit, units, unit_price: price, amount: r6(price * units) });
    return null;
  };
  let missing: string | null = null;
  if (kind === "text") {
    missing = add("generation", model, "input_token", Math.ceil((req.prompt_chars ?? 0) / 3) + 600) ?? add("generation", model, "output_token", req.max_output_tokens || 1024);
  } else if (kind === "image") {
    missing = add("images", model, `image_${req.size || "1024"}_${req.quality || "standard"}`, Math.max(1, req.count ?? 1));
  } else {
    if ((req.clip_seconds ?? 0) > 0) missing = add("scene_clips", model, `video_second_${req.resolution || "1080p"}`, req.clip_seconds ?? 0);
    if (!missing && (req.voiceover_chars ?? 0) > 0) missing = add("voiceover", DEFAULT_MODEL.voice, "char", req.voiceover_chars ?? 0);
    lines.push({ item: "render", provider: "internal", model: "ffmpeg", unit: "render_second", units: 0, unit_price: 0, amount: 0 });
  }
  if (missing) return { missing };
  const total = r6(lines.reduce((a, l) => a + l.amount, 0));
  const perJob = state.settings.per_job_limits[kind as "text" | "image" | "video"];
  const g = budgetDto(state.global);
  const camp = req.campaign_id ? state.campaigns.find((c) => c.campaign_id === req.campaign_id) : undefined;
  const campRemaining = camp ? budgetDto(camp).remaining_amount : null;
  return {
    breakdown: lines,
    total: { amount: total, currency: "USD", is_estimate: true },
    pricing_version: "fake-2026-10",
    limits: { per_job_limit: perJob, campaign_remaining: campRemaining, global_remaining: g.remaining_amount },
    requires_override: total > perJob,
    blocked: (g.hard_stop && total > g.remaining_amount) || (camp !== undefined && camp.hard_stop && total > (campRemaining ?? 0)),
  };
}

function groupKey(u: MockUsage, g: GroupBy): string {
  switch (g) {
    case "day":
      return u.occurredAt.slice(0, 10);
    case "campaign":
      return u.campaignId ?? "";
    case "provider":
      return u.provider;
    case "model":
      return u.model;
    case "user":
      return u.userId;
    default:
      return u.jobType;
  }
}

function label(key: string, g: GroupBy): string {
  if (g === "campaign") return key === "" ? "Không thuộc chiến dịch" : (CAMPAIGNS.find((c) => c.id === key)?.name ?? key);
  if (g === "user") return MOCK_USERS.find((u) => u.id === key)?.name ?? key;
  return key;
}

const zero = (): UsageTotalsDto => ({ provider_reported: 0, calculated: 0, estimated: 0, total: 0, count: 0 });

function addTo(t: UsageTotalsDto, u: MockUsage) {
  t[u.source] = r6(t[u.source] + u.cost);
  t.total = r6(t.total + u.cost);
  t.count += 1;
}

function summary(from: string, to: string, g: GroupBy): UsageSummaryDto {
  const rows = new Map<string, UsageTotalsDto>();
  const totals = zero();
  for (const u of state.usage) {
    const day = u.occurredAt.slice(0, 10);
    if (day < from || day > to) continue;
    const k = groupKey(u, g);
    const row = rows.get(k) ?? zero();
    addTo(row, u);
    rows.set(k, row);
    addTo(totals, u);
  }
  const list = [...rows.entries()].map(([key, t]) => ({ key: key === "" ? null : key, label: label(key, g), ...t }));
  list.sort((a, b) => (g === "day" ? (a.key ?? "").localeCompare(b.key ?? "") : b.total - a.total));
  return { from, to, group_by: g, currency: "USD", rows: list, totals };
}

const DATE = /^\d{4}-\d{2}-\d{2}$/;

export const handlers: HttpHandler[] = [
  rest.post(
    api("/cost-estimates"),
    withAuth("generate.run", async ({ request }) => {
      const body = (await request.json()) as EstimateRequestDto;
      if (!["text", "image", "video"].includes(body.kind)) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "kind", issue: "invalid" }]);
      }
      const res = estimate(body);
      if ("missing" in res) return fail(503, "PRICING_MISSING", "Chưa có bảng giá cho model này", [{ field: "unit", issue: res.missing }]);
      return ok(res);
    }),
  ),
  rest.get(
    api("/usage/summary"),
    withAuth("usage.view", ({ request }) => {
      const url = new URL(request.url);
      const now = new Date();
      const from = url.searchParams.get("from") || iso(monthStart(now));
      const to = url.searchParams.get("to") || iso(now);
      const g = (url.searchParams.get("group_by") || "day") as GroupBy;
      if (!GROUP_BYS.includes(g)) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "group_by", issue: "invalid" }]);
      if (!DATE.test(from) || !DATE.test(to)) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "from", issue: "invalid_date" }]);
      if (to < from) return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "to", issue: "before_from" }]);
      return ok(summary(from, to, g));
    }),
  ),
  rest.get(
    api("/settings/budgets"),
    withAuth("usage.view", () => ok(settingsDto())),
  ),
  rest.patch(
    api("/settings/budgets"),
    withAuth("budget.manage", async ({ request }) => {
      const body = (await request.json()) as {
        per_job_limits?: Partial<Record<"text" | "image" | "video", number>>;
        global?: { limit_amount?: number; alert_thresholds?: number[]; hard_stop?: boolean };
        max_concurrent_video_jobs_per_user?: number;
      };
      const th = body.global?.alert_thresholds;
      if (th && (th.length === 0 || th.some((t) => t < 1 || t > 100))) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "global.alert_thresholds", issue: "invalid" }]);
      }
      state.settings.per_job_limits = { ...state.settings.per_job_limits, ...body.per_job_limits };
      if (body.global?.limit_amount !== undefined) {
        state.global.limit_amount = body.global.limit_amount;
        state.settings.global_template.monthly_limit = body.global.limit_amount;
      }
      if (th) {
        state.global.alert_thresholds = [...th].sort((a, b) => a - b);
        state.settings.global_template.alert_thresholds = state.global.alert_thresholds;
      }
      if (body.global?.hard_stop !== undefined) {
        state.global.hard_stop = body.global.hard_stop;
        state.settings.global_template.hard_stop = body.global.hard_stop;
      }
      if (body.max_concurrent_video_jobs_per_user !== undefined) state.settings.max_concurrent_video_jobs_per_user = body.max_concurrent_video_jobs_per_user;
      state.settings.updated_at = new Date().toISOString();
      return ok(settingsDto());
    }),
  ),
  rest.put(
    api("/campaigns/:id/budget"),
    withAuth<{ id: string }>("budget.manage", async ({ request, params }) => {
      const body = (await request.json()) as { limit_amount?: number; period_start?: string; period_end?: string; alert_thresholds?: number[]; hard_stop?: boolean };
      if (body.limit_amount === undefined || body.limit_amount < 0) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "limit_amount", issue: "required" }]);
      }
      if (body.period_start && body.period_end && body.period_end < body.period_start) {
        return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [{ field: "period_end", issue: "before_start" }]);
      }
      let b = state.campaigns.find((c) => c.campaign_id === params.id);
      if (!b) {
        const today = iso(new Date());
        b = {
          id: crypto.randomUUID(),
          scope: "campaign",
          campaign_id: params.id,
          campaign_name: CAMPAIGNS.find((c) => c.id === params.id)?.name ?? null,
          period_start: today,
          period_end: today,
          limit_amount: 0,
          reserved_amount: 0,
          spent_amount: 0,
          currency: "USD",
          alert_thresholds: [80, 100],
          alerts_sent: [],
          hard_stop: true,
          updated_at: new Date().toISOString(),
        };
        state.campaigns.push(b);
      }
      b.limit_amount = body.limit_amount;
      if (body.period_start) b.period_start = body.period_start;
      if (body.period_end) b.period_end = body.period_end;
      if (body.alert_thresholds) b.alert_thresholds = [...body.alert_thresholds].sort((x, y) => x - y);
      if (body.hard_stop !== undefined) b.hard_stop = body.hard_stop;
      b.updated_at = new Date().toISOString();
      return ok(budgetDto(b));
    }),
  ),
];

