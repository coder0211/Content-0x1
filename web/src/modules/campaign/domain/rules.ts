import type { CampaignStatus, ID, Role } from "@/shared/kernel";
import type { Campaign, CampaignAssetType, CampaignBrief, CampaignDraft, CampaignGoal, ContentPlan, PlanItem } from "./types";

export const GOALS: { value: CampaignGoal; label: string }[] = [
  { value: "awareness", label: "Nhận diện" },
  { value: "engagement", label: "Tương tác" },
  { value: "lead", label: "Thu lead" },
  { value: "conversion", label: "Chuyển đổi" },
];

export const GOAL_LABELS: Record<CampaignGoal, string> = Object.fromEntries(GOALS.map((g) => [g.value, g.label])) as Record<CampaignGoal, string>;

export const CHANNELS: { value: string; label: string }[] = [
  { value: "facebook", label: "Facebook" },
  { value: "tiktok", label: "TikTok" },
  { value: "instagram", label: "Instagram" },
  { value: "youtube", label: "YouTube" },
  { value: "zalo", label: "Zalo" },
  { value: "email", label: "Email" },
  { value: "website", label: "Website" },
];

export const FORMATS: { value: string; label: string; assetType: CampaignAssetType }[] = [
  { value: "facebook_post", label: "Bài Facebook", assetType: "content" },
  { value: "tiktok_script", label: "Kịch bản TikTok", assetType: "content" },
  { value: "caption", label: "Caption", assetType: "content" },
  { value: "email", label: "Email", assetType: "content" },
  { value: "poster", label: "Poster", assetType: "image" },
  { value: "banner", label: "Banner", assetType: "image" },
  { value: "short_video", label: "Video ngắn", assetType: "video" },
];

export function channelLabel(v: string): string {
  return CHANNELS.find((c) => c.value === v)?.label ?? v;
}

export function formatLabel(v: string): string {
  return FORMATS.find((f) => f.value === v)?.label ?? v;
}

/** Studio route per asset type (pre-assigned module paths). */
export const STUDIO_PATH: Record<CampaignAssetType, string> = { content: "/content", image: "/images", video: "/videos" };

/** "Tạo mới" target: the studio opens with the campaign (and its brief) preselected (ACS-026). */
export function studioLink(type: CampaignAssetType, campaignId: ID, planItemId?: string): string {
  const qs = new URLSearchParams({ campaign_id: campaignId });
  if (planItemId) qs.set("plan_item_id", planItemId);
  return `${STUDIO_PATH[type]}?${qs.toString()}`;
}

/** Action label for moving a campaign to `to` (verb + object). */
export const TRANSITION_LABELS: Record<CampaignStatus, string> = {
  draft: "Chuyển về nháp",
  in_progress: "Bắt đầu thực hiện",
  review: "Chuyển sang duyệt",
  completed: "Đánh dấu hoàn thành",
  archived: "Lưu trữ campaign",
};

/** Label when moving back from review to in_progress. */
export function transitionLabel(from: CampaignStatus, to: CampaignStatus): string {
  if (from === "review" && to === "in_progress") return "Trở lại thực hiện";
  return TRANSITION_LABELS[to];
}

/** New AI work is allowed unless completed/archived (same rule as the backend). */
export function acceptsGeneration(status: CampaignStatus): boolean {
  return status !== "completed" && status !== "archived";
}

export interface Actor {
  id: ID;
  roles: Role[];
  permissions: string[];
}

/** Mirrors domain.CanEdit: campaign.manage and (Lead/Admin or creator/member). */
export function canEditCampaign(c: Pick<Campaign, "createdBy" | "memberIds" | "status">, actor: Actor | null): boolean {
  if (!actor || !actor.permissions.includes("campaign.manage")) return false;
  if (c.status === "archived") return false;
  const leadOrAdmin = actor.roles.includes("admin") || actor.roles.includes("marketing_lead");
  return leadOrAdmin || c.createdBy === actor.id || c.memberIds.includes(actor.id);
}

export function emptyBrief(): CampaignBrief {
  return { productIds: [], targetAudience: "", keyMessage: "", verifiedOffer: "", channels: [], formats: [], deadline: "", notes: "" };
}

export function emptyDraft(brandId = ""): CampaignDraft {
  return { brandId, name: "", goal: "", startDate: "", endDate: "", brief: emptyBrief() };
}

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
const SLUG_RE = /^[a-z0-9_]{1,40}$/;

export function isDate(s: string): boolean {
  return DATE_RE.test(s) && !Number.isNaN(Date.parse(`${s}T00:00:00Z`));
}

export type FieldErrors = Record<string, string>;

/** Step 1 of the wizard (header). Keys follow server field paths. */
export function validateHeader(d: CampaignDraft): FieldErrors {
  const e: FieldErrors = {};
  if (!d.brandId) e.brand_id = "required";
  if (!d.name.trim()) e.name = "required";
  else if (d.name.trim().length > 200) e.name = "too_long";
  if (!d.goal) e.goal = "required";
  if (d.startDate && !isDate(d.startDate)) e.start_date = "invalid_date";
  if (d.endDate && !isDate(d.endDate)) e.end_date = "invalid_date";
  if (!e.start_date && !e.end_date && d.startDate && d.endDate && d.endDate < d.startDate) e.end_date = "before_start_date";
  return e;
}

/** Step 2 (brief) — same rules as domain.Brief.Validate (FR-CMP-01). */
export function validateBrief(b: CampaignBrief): FieldErrors {
  const e: FieldErrors = {};
  if (!b.targetAudience.trim()) e["brief.target_audience"] = "required";
  if (!b.keyMessage.trim()) e["brief.key_message"] = "required";
  if (b.channels.length === 0) e["brief.channels"] = "required";
  if (b.formats.length === 0) e["brief.formats"] = "required";
  const seen = new Set<string>();
  b.formats.forEach((f, i) => {
    if (!SLUG_RE.test(f.type)) e[`brief.formats[${i}].type`] = f.type ? "invalid" : "required";
    else if (seen.has(f.type)) e[`brief.formats[${i}].type`] = "duplicate";
    seen.add(f.type);
    if (!Number.isInteger(f.count) || f.count < 1 || f.count > 50) e[`brief.formats[${i}].count`] = "out_of_range";
  });
  if (!b.deadline) e["brief.deadline"] = "required";
  else if (!isDate(b.deadline)) e["brief.deadline"] = "invalid_date";
  return e;
}

/** Plan approval checks (same as domain.Plan.Validate, the subset a user can hit in the editor). */
export function validatePlan(p: ContentPlan): FieldErrors {
  const e: FieldErrors = {};
  if (p.items.length === 0) e["plan.items"] = "required";
  p.items.forEach((it, i) => {
    if (!it.topic.trim()) e[`plan.items[${i}].topic`] = "required";
    if (!it.format.trim()) e[`plan.items[${i}].format`] = "required";
    if (it.publishDate && !isDate(it.publishDate)) e[`plan.items[${i}].publish_date`] = "invalid_date";
  });
  return e;
}

export function newPlanItem(existing: PlanItem[]): PlanItem {
  let n = existing.length + 1;
  const ids = new Set(existing.map((i) => i.id));
  while (ids.has(`item-${n}`)) n++;
  return { id: `item-${n}`, topic: "", angle: "", channel: "", format: "", publishDate: "", notes: "", done: false };
}

/** Progress of the approved plan checklist. */
export function planProgress(p: ContentPlan | null): { done: number; total: number } {
  if (!p) return { done: 0, total: 0 };
  return { done: p.items.filter((i) => i.done).length, total: p.items.length };
}

/** Asset type a plan item / format produces (unknown formats → content). */
export function assetTypeOfFormat(format: string): CampaignAssetType {
  return FORMATS.find((f) => f.value === format)?.assetType ?? "content";
}

export const ISSUE_MESSAGES: Record<string, string> = {
  required: "Bắt buộc nhập.",
  too_long: "Quá dài.",
  too_many: "Quá nhiều mục.",
  duplicate: "Bị trùng.",
  invalid: "Không hợp lệ.",
  invalid_date: "Ngày không hợp lệ.",
  before_start_date: "Ngày kết thúc phải sau ngày bắt đầu.",
  out_of_range: "Số lượng từ 1 đến 50.",
  not_found: "Không tồn tại.",
};

export function issueMessage(issue: string | undefined | null): string | null {
  if (!issue) return null;
  return ISSUE_MESSAGES[issue] ?? "Không hợp lệ.";
}

/** Fields belonging to wizard step 1; anything else is on step 2. */
export function isHeaderField(field: string): boolean {
  return ["brand_id", "name", "goal", "start_date", "end_date", "member_ids"].some((f) => field === f || field.startsWith(`${f}[`));
}
