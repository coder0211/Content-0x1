import type { CampaignStatus, ID, Page } from "@/shared/kernel";
import type { Campaign, CampaignAsset, CampaignAssetType, CampaignBrief, CampaignGoal, CampaignMember, ContentPlan } from "../domain/types";

export interface CampaignQuery {
  brandId?: ID;
  status?: CampaignStatus;
  q?: string;
  mine?: boolean;
  cursor?: string | null;
  limit?: number;
}

export interface CreateCampaignInput {
  brandId: ID;
  name: string;
  goal: CampaignGoal;
  brief: CampaignBrief;
  startDate: string | null;
  endDate: string | null;
  memberIds: ID[];
}

export interface UpdateCampaignInput {
  name?: string;
  goal?: CampaignGoal;
  brief?: CampaignBrief;
  /** "" clears the date. */
  startDate?: string;
  endDate?: string;
  status?: CampaignStatus;
  memberIds?: ID[];
}

export interface CampaignRepository {
  list(q: CampaignQuery): Promise<Page<Campaign>>;
  get(id: ID): Promise<Campaign>;
  create(input: CreateCampaignInput): Promise<Campaign>;
  update(id: ID, input: UpdateCampaignInput): Promise<Campaign>;
  memberCandidates(): Promise<CampaignMember[]>;
  /** PUT /campaigns/{id}/plan — approve (and edit) the plan (ACS-037). */
  savePlan(id: ID, plan: ContentPlan): Promise<Campaign>;
  discardProposal(id: ID): Promise<void>;
  /** POST /campaigns/{id}/plan-generations (served by the content module) → job id. */
  generatePlan(id: ID, idempotencyKey: string): Promise<{ jobId: ID }>;
  /** GET /assets?campaign_id&type (Library search). */
  assets(campaignId: ID, type: CampaignAssetType, cursor?: string | null): Promise<Page<CampaignAsset>>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    campaign: CampaignRepository;
  }
}
