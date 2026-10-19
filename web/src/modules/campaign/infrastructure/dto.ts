import type { AssetStatus, CampaignStatus, JobStatus } from "@/shared/kernel";
import type { Campaign, CampaignAsset, CampaignAssetType, CampaignBrief, CampaignGoal, CampaignMember, ContentPlan, PlanJob } from "../domain/types";

export interface CampaignBriefDto {
  product_ids: string[];
  target_audience: string;
  key_message: string;
  verified_offer: string;
  channels: string[];
  formats: { type: string; count: number }[];
  deadline: string;
  notes: string;
}

export interface PlanItemDto {
  id?: string;
  topic: string;
  angle?: string;
  channel?: string;
  format: string;
  publish_date?: string;
  notes?: string;
  done?: boolean;
}

export interface ContentPlanDto {
  summary?: string;
  themes?: string[];
  items: PlanItemDto[];
}

export interface CampaignMemberDto {
  id: string;
  name: string;
  email: string;
}

export interface CampaignDto {
  id: string;
  brand_id: string;
  name: string;
  goal: CampaignGoal;
  status: CampaignStatus;
  next_statuses: CampaignStatus[];
  brief: CampaignBriefDto;
  start_date: string | null;
  end_date: string | null;
  content_plan: ContentPlanDto | null;
  content_plan_approved_by: string | null;
  content_plan_approved_at: string | null;
  content_plan_proposal: ContentPlanDto | null;
  content_plan_proposed_at: string | null;
  member_ids: string[];
  members?: CampaignMemberDto[];
  created_by: string;
  created_at: string;
  updated_at: string;
}

/** Minimal Library list row (SPEC-04 §2.8); other fields are ignored. */
export interface AssetRowDto {
  id: string;
  asset_type: CampaignAssetType;
  title?: string | null;
  status: AssetStatus;
  created_at: string;
  updated_at?: string;
}

export interface JobDto {
  id: string;
  status: JobStatus;
  progress?: number | null;
  error?: { kind?: string; error_kind?: string; message?: string; detail?: string } | null;
}

export function toBrief(d: Partial<CampaignBriefDto> | null | undefined): CampaignBrief {
  return {
    productIds: d?.product_ids ?? [],
    targetAudience: d?.target_audience ?? "",
    keyMessage: d?.key_message ?? "",
    verifiedOffer: d?.verified_offer ?? "",
    channels: d?.channels ?? [],
    formats: (d?.formats ?? []).map((f) => ({ type: f.type, count: f.count })),
    deadline: d?.deadline ?? "",
    notes: d?.notes ?? "",
  };
}

export function fromBrief(b: CampaignBrief): CampaignBriefDto {
  return {
    product_ids: b.productIds,
    target_audience: b.targetAudience.trim(),
    key_message: b.keyMessage.trim(),
    verified_offer: b.verifiedOffer.trim(),
    channels: b.channels,
    formats: b.formats.map((f) => ({ type: f.type, count: f.count })),
    deadline: b.deadline,
    notes: b.notes.trim(),
  };
}

export function toPlan(d: ContentPlanDto | null | undefined): ContentPlan | null {
  if (!d) return null;
  return {
    summary: d.summary ?? "",
    themes: d.themes ?? [],
    items: (d.items ?? []).map((it, i) => ({
      id: it.id || `item-${i + 1}`,
      topic: it.topic ?? "",
      angle: it.angle ?? "",
      channel: it.channel ?? "",
      format: it.format ?? "",
      publishDate: it.publish_date ?? "",
      notes: it.notes ?? "",
      done: !!it.done,
    })),
  };
}

export function fromPlan(p: ContentPlan): ContentPlanDto {
  return {
    summary: p.summary.trim(),
    themes: p.themes.map((t) => t.trim()).filter(Boolean),
    items: p.items.map((it) => ({
      id: it.id,
      topic: it.topic.trim(),
      angle: it.angle.trim(),
      channel: it.channel.trim(),
      format: it.format.trim(),
      publish_date: it.publishDate,
      notes: it.notes.trim(),
      done: it.done,
    })),
  };
}

export function toMember(d: CampaignMemberDto): CampaignMember {
  return { id: d.id, name: d.name, email: d.email };
}

export function toCampaign(d: CampaignDto): Campaign {
  return {
    id: d.id,
    brandId: d.brand_id,
    name: d.name,
    goal: d.goal,
    status: d.status,
    nextStatuses: d.next_statuses ?? [],
    brief: toBrief(d.brief),
    startDate: d.start_date,
    endDate: d.end_date,
    contentPlan: toPlan(d.content_plan),
    contentPlanApprovedBy: d.content_plan_approved_by,
    contentPlanApprovedAt: d.content_plan_approved_at,
    planProposal: toPlan(d.content_plan_proposal),
    planProposedAt: d.content_plan_proposed_at,
    memberIds: d.member_ids ?? [],
    members: (d.members ?? []).map(toMember),
    createdBy: d.created_by,
    createdAt: d.created_at,
    updatedAt: d.updated_at,
  };
}

export function toAsset(d: AssetRowDto): CampaignAsset {
  return { id: d.id, type: d.asset_type, title: d.title ?? "", status: d.status, createdAt: d.created_at, updatedAt: d.updated_at ?? d.created_at };
}

export function toJob(d: JobDto): PlanJob {
  return {
    id: d.id,
    status: d.status,
    progress: d.progress ?? 0,
    errorKind: d.error?.kind ?? d.error?.error_kind ?? null,
    errorDetail: d.error?.detail ?? d.error?.message ?? null,
  };
}
