import type { HttpHandler } from "msw";
import { accepted, api, fail, MOCK_USERS, noContent, ok, page, rest, withAuth } from "@/shared/api/mock";
import { createMockJob } from "@/shared/api/mock-jobs";
import type { ApiErrorDetail, CampaignStatus } from "@/shared/kernel";
import { uuid } from "@/shared/lib/id";
import { canEditCampaign, validateBrief, validateHeader, validatePlan } from "../domain/rules";
import type { CampaignGoal } from "../domain/types";
import { fromPlan, toBrief, toPlan, type CampaignBriefDto, type CampaignDto, type ContentPlanDto } from "./dto";

/** Same ids as the brand mocks (modules/brand/infrastructure/mocks.ts). */
const MOCK_BRAND_ID = "10000000-0000-4000-8000-000000000001";
const MOCK_PRODUCT_ID = "10000000-0000-4000-8000-000000000101";
export const MOCK_CAMPAIGN_ID = "20000000-0000-4000-8000-000000000001";
export const MOCK_DRAFT_CAMPAIGN_ID = "20000000-0000-4000-8000-000000000002";
const LEAD_ID = "00000000-0000-4000-8000-000000000002";
const CREATOR_ID = "00000000-0000-4000-8000-000000000003";

type Row = Omit<CampaignDto, "next_statuses" | "members">;

const TRANSITIONS: Record<CampaignStatus, CampaignStatus[]> = {
  draft: ["in_progress", "archived"],
  in_progress: ["review", "archived"],
  review: ["in_progress", "completed", "archived"],
  completed: ["archived"],
  archived: [],
};

const store = {
  campaigns: new Map<string, Row>(),
};

/** Duration of the simulated content_plan job (0 = done on the next read, for tests). */
let planStepMs = 800;
export function setCampaignMockPlanStepMs(ms: number): void {
  planStepMs = ms;
}

function seed() {
  const now = "2026-10-05T03:00:00Z";
  const brief: CampaignBriefDto = {
    product_ids: [MOCK_PRODUCT_ID],
    target_audience: "Sinh viên năm 2–4 tại Hà Nội, TP.HCM",
    key_message: "Tự tin giao tiếp sau 12 tuần",
    verified_offer: "Giảm 20% đến 31/12",
    channels: ["facebook", "tiktok"],
    formats: [
      { type: "facebook_post", count: 5 },
      { type: "poster", count: 3 },
      { type: "short_video", count: 2 },
    ],
    deadline: "2026-11-15",
    notes: "",
  };
  store.campaigns.set(MOCK_CAMPAIGN_ID, {
    id: MOCK_CAMPAIGN_ID,
    brand_id: MOCK_BRAND_ID,
    name: "Khóa giao tiếp T11",
    goal: "lead",
    status: "in_progress",
    brief,
    start_date: "2026-11-01",
    end_date: "2026-12-31",
    content_plan: {
      summary: "Hai tuần đầu tập trung nỗi sợ nói, sau đó là ưu đãi.",
      themes: ["Vượt qua nỗi sợ nói"],
      items: [
        { id: "item-1", topic: "3 lý do bạn ngại nói tiếng Anh", angle: "Đồng cảm", channel: "facebook", format: "facebook_post", publish_date: "2026-11-02", notes: "", done: true },
        { id: "item-2", topic: "Poster ưu đãi 20%", angle: "Ưu đãi có hạn", channel: "facebook", format: "poster", publish_date: "2026-11-05", notes: "", done: false },
      ],
    },
    content_plan_approved_by: LEAD_ID,
    content_plan_approved_at: now,
    content_plan_proposal: null,
    content_plan_proposed_at: null,
    member_ids: [CREATOR_ID],
    created_by: LEAD_ID,
    created_at: now,
    updated_at: now,
  });
  store.campaigns.set(MOCK_DRAFT_CAMPAIGN_ID, {
    id: MOCK_DRAFT_CAMPAIGN_ID,
    brand_id: MOCK_BRAND_ID,
    name: "Tuyển sinh Tết",
    goal: "awareness",
    status: "draft",
    brief: { ...brief, product_ids: [], verified_offer: "", channels: ["facebook"], formats: [{ type: "facebook_post", count: 3 }], deadline: "2027-01-10" },
    start_date: null,
    end_date: null,
    content_plan: null,
    content_plan_approved_by: null,
    content_plan_approved_at: null,
    content_plan_proposal: null,
    content_plan_proposed_at: null,
    member_ids: [],
    created_by: LEAD_ID,
    created_at: now,
    updated_at: now,
  });
}

export function resetCampaignMocks(): void {
  store.campaigns.clear();
  planStepMs = 800;
  seed();
}
seed();

const validation = (details: ApiErrorDetail[]) => fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", details);
const notFound = () => fail(404, "NOT_FOUND", "Không tìm thấy campaign");
const details = (e: Record<string, string>): ApiErrorDetail[] => Object.entries(e).map(([field, issue]) => ({ field, issue }));

function present(r: Row, withMembers = false): CampaignDto {
  const dto: CampaignDto = { ...r, next_statuses: TRANSITIONS[r.status] };
  if (withMembers) {
    dto.members = r.member_ids.flatMap((id) => {
      const u = MOCK_USERS.find((m) => m.id === id);
      return u ? [{ id: u.id, name: u.name, email: u.email }] : [];
    });
  }
  return dto;
}

function actorOf(user: { id: string; roles: string[]; permissions: string[] }) {
  return { id: user.id, roles: user.roles as never, permissions: user.permissions };
}

function canEdit(r: Row, user: { id: string; roles: string[]; permissions: string[] }) {
  return canEditCampaign({ createdBy: r.created_by, memberIds: r.member_ids, status: "draft" }, actorOf(user));
}

function proposalFor(r: Row): ContentPlanDto {
  const items: ContentPlanDto["items"] = [];
  r.brief.formats.forEach((f) => {
    for (let i = 1; i <= Math.min(f.count, 3); i++) {
      items.push({
        id: `item-${items.length + 1}`,
        topic: `${r.brief.key_message} — ${f.type} ${i}`,
        angle: i === 1 ? "Đồng cảm" : "Lợi ích",
        channel: r.brief.channels[0] ?? "",
        format: f.type,
        publish_date: "",
        notes: "",
        done: false,
      });
    }
  });
  return { summary: `Kế hoạch cho ${r.name}`, themes: [r.brief.key_message], items };
}

interface CreateBody {
  brand_id?: string;
  name?: string;
  goal?: CampaignGoal | "";
  brief?: CampaignBriefDto;
  start_date?: string | null;
  end_date?: string | null;
  member_ids?: string[];
}

export const handlers: HttpHandler[] = [
  rest.get(
    api("/campaigns"),
    withAuth("brand.view", ({ request, user }) => {
      const u = new URL(request.url);
      const brandId = u.searchParams.get("brand_id");
      const status = u.searchParams.get("status");
      const q = u.searchParams.get("q")?.toLowerCase();
      const mine = u.searchParams.get("mine") === "true";
      const rows = [...store.campaigns.values()]
        .filter((r) => (!brandId || r.brand_id === brandId) && (!status || r.status === status))
        .filter((r) => !q || r.name.toLowerCase().includes(q))
        .filter((r) => !mine || r.created_by === user.id || r.member_ids.includes(user.id))
        .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
      return page(rows.map((r) => present(r)));
    }),
  ),
  rest.get(
    api("/campaigns/member-candidates"),
    withAuth("campaign.manage", () =>
      page(MOCK_USERS.filter((u) => u.permissions.includes("generate.run")).map((u) => ({ id: u.id, name: u.name, email: u.email }))),
    ),
  ),
  rest.get(
    api("/campaigns/:id"),
    withAuth("brand.view", ({ params }) => {
      const r = store.campaigns.get(String(params.id));
      return r ? ok(present(r, true)) : notFound();
    }),
  ),
  rest.post(
    api("/campaigns"),
    withAuth("campaign.manage", async ({ request, user }) => {
      const b = (await request.json()) as CreateBody;
      const brief = toBrief(b.brief);
      const e = {
        ...validateHeader({ brandId: b.brand_id ?? "", name: b.name ?? "", goal: b.goal ?? "", startDate: b.start_date ?? "", endDate: b.end_date ?? "", brief }),
        ...validateBrief(brief),
      };
      if (Object.keys(e).length) return validation(details(e));
      const now = new Date().toISOString();
      const row: Row = {
        id: uuid(),
        brand_id: b.brand_id!,
        name: b.name!.trim(),
        goal: b.goal as CampaignGoal,
        status: "draft",
        brief: b.brief!,
        start_date: b.start_date || null,
        end_date: b.end_date || null,
        content_plan: null,
        content_plan_approved_by: null,
        content_plan_approved_at: null,
        content_plan_proposal: null,
        content_plan_proposed_at: null,
        member_ids: b.member_ids ?? [],
        created_by: user.id,
        created_at: now,
        updated_at: now,
      };
      store.campaigns.set(row.id, row);
      return ok(present(row, true), { status: 201 });
    }),
  ),
  rest.patch(
    api("/campaigns/:id"),
    withAuth("campaign.manage", async ({ request, params, user }) => {
      const r = store.campaigns.get(String(params.id));
      if (!r) return notFound();
      if (!canEdit(r, user)) return fail(403, "FORBIDDEN", "Bạn không thuộc campaign này");
      const b = (await request.json()) as CreateBody & { status?: CampaignStatus };
      if (r.status === "archived") return fail(409, "INVALID_STATE_TRANSITION", "Campaign đã lưu trữ", [{ from: "archived", to: b.status ?? "archived" }]);
      const next: Row = { ...r };
      if (b.name !== undefined) next.name = b.name.trim();
      if (b.goal !== undefined) next.goal = b.goal as CampaignGoal;
      if (b.start_date !== undefined) next.start_date = b.start_date || null;
      if (b.end_date !== undefined) next.end_date = b.end_date || null;
      if (b.brief !== undefined) next.brief = b.brief;
      if (b.member_ids !== undefined) next.member_ids = b.member_ids;
      const e = {
        ...validateHeader({ brandId: next.brand_id, name: next.name, goal: next.goal, startDate: next.start_date ?? "", endDate: next.end_date ?? "", brief: toBrief(next.brief) }),
        ...(b.brief !== undefined ? validateBrief(toBrief(b.brief)) : {}),
      };
      if (Object.keys(e).length) return validation(details(e));
      if (b.status && b.status !== r.status) {
        if (!TRANSITIONS[r.status].includes(b.status)) {
          return fail(409, "INVALID_STATE_TRANSITION", `Không thể chuyển từ ${r.status} sang ${b.status}`, [{ from: r.status, to: b.status }]);
        }
        next.status = b.status;
      }
      next.updated_at = new Date().toISOString();
      store.campaigns.set(r.id, next);
      return ok(present(next, true));
    }),
  ),
  rest.put(
    api("/campaigns/:id/plan"),
    withAuth("campaign.manage", async ({ request, params, user }) => {
      const r = store.campaigns.get(String(params.id));
      if (!r) return notFound();
      if (!canEdit(r, user)) return fail(403, "FORBIDDEN", "Bạn không thuộc campaign này");
      if (r.status === "archived") return fail(409, "INVALID_STATE_TRANSITION", "Campaign đã lưu trữ");
      const { plan } = (await request.json()) as { plan: ContentPlanDto };
      const parsed = toPlan(plan) ?? { summary: "", themes: [], items: [] };
      const e = validatePlan(parsed);
      if (Object.keys(e).length) return validation(details(e));
      const now = new Date().toISOString();
      const next: Row = {
        ...r,
        content_plan: fromPlan(parsed),
        content_plan_approved_by: user.id,
        content_plan_approved_at: now,
        content_plan_proposal: null,
        content_plan_proposed_at: null,
        updated_at: now,
      };
      store.campaigns.set(r.id, next);
      return ok(present(next, true));
    }),
  ),
  rest.delete(
    api("/campaigns/:id/plan-proposal"),
    withAuth("campaign.manage", ({ params }) => {
      const r = store.campaigns.get(String(params.id));
      if (!r) return notFound();
      store.campaigns.set(r.id, { ...r, content_plan_proposal: null, content_plan_proposed_at: null });
      return noContent();
    }),
  ),
  // Endpoint owned by the content module (SPEC-04 §2.3). It is mocked here because the job's only
  // effect is campaigns.content_plan_proposal (CampaignAPI.SaveContentPlanProposal on the backend),
  // which lives in this store. The job itself is in the shared job store (served by the job mocks).
  rest.post(
    api("/campaigns/:id/plan-generations"),
    withAuth("generate.run", ({ params, request, user }) => {
      const key = request.headers.get("Idempotency-Key");
      if (!key) return fail(400, "IDEMPOTENCY_KEY_REQUIRED", "Thiếu Idempotency-Key");
      const r = store.campaigns.get(String(params.id));
      if (!r) return notFound();
      if (r.status === "completed" || r.status === "archived") return fail(409, "INVALID_STATE_TRANSITION", "Campaign không nhận việc mới");
      const job = createMockJob({
        type: "content_plan",
        createdBy: user.id,
        idempotencyKey: key,
        campaignId: r.id,
        estimate: 0.02,
        steps: [["plan", "content_plan"]],
        simulate: { stepMs: planStepMs },
        onSucceeded: () => {
          const cur = store.campaigns.get(r.id) ?? r;
          store.campaigns.set(r.id, { ...cur, content_plan_proposal: proposalFor(cur), content_plan_proposed_at: new Date().toISOString() });
          return { campaign_id: r.id };
        },
      });
      return accepted({ job_id: job.id, status: "queued", estimated_cost: { amount: 0.02, currency: "USD", is_estimate: true } });
    }),
  ),
];
