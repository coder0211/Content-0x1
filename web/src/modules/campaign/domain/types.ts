import type { AssetStatus, CampaignStatus, ID, ISODateTime, JobStatus } from "@/shared/kernel";

export type CampaignGoal = "awareness" | "engagement" | "lead" | "conversion";
export type CampaignAssetType = "content" | "image" | "video";

export interface CampaignFormat {
  type: string;
  count: number;
}

export interface CampaignBrief {
  productIds: ID[];
  targetAudience: string;
  keyMessage: string;
  verifiedOffer: string;
  channels: string[];
  formats: CampaignFormat[];
  /** YYYY-MM-DD */
  deadline: string;
  notes: string;
}

export interface PlanItem {
  id: string;
  topic: string;
  angle: string;
  channel: string;
  format: string;
  publishDate: string;
  notes: string;
  done: boolean;
}

export interface ContentPlan {
  summary: string;
  themes: string[];
  items: PlanItem[];
}

export interface CampaignMember {
  id: ID;
  name: string;
  email: string;
}

export interface Campaign {
  id: ID;
  brandId: ID;
  name: string;
  goal: CampaignGoal;
  status: CampaignStatus;
  nextStatuses: CampaignStatus[];
  brief: CampaignBrief;
  startDate: string | null;
  endDate: string | null;
  contentPlan: ContentPlan | null;
  contentPlanApprovedBy: ID | null;
  contentPlanApprovedAt: ISODateTime | null;
  planProposal: ContentPlan | null;
  planProposedAt: ISODateTime | null;
  memberIds: ID[];
  /** Only on the detail response. */
  members: CampaignMember[];
  createdBy: ID;
  createdAt: ISODateTime;
  updatedAt: ISODateTime;
}

/** Header + brief as edited in the wizard / brief sheet. */
export interface CampaignDraft {
  brandId: ID;
  name: string;
  goal: CampaignGoal | "";
  startDate: string;
  endDate: string;
  brief: CampaignBrief;
}

/** A content/image/video asset of the campaign (Library list row, minimal fields). */
export interface CampaignAsset {
  id: ID;
  type: CampaignAssetType;
  title: string;
  status: AssetStatus;
  createdAt: ISODateTime;
  updatedAt: ISODateTime;
}

export interface PlanJob {
  id: ID;
  status: JobStatus;
  progress: number;
  errorKind: string | null;
  errorDetail: string | null;
}
