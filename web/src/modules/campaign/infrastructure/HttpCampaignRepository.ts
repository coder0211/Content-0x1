import { http } from "@/shared/api/http";
import type { ID, Page } from "@/shared/kernel";
import type { CampaignQuery, CampaignRepository, CreateCampaignInput, UpdateCampaignInput } from "../application/ports";
import type { Campaign, CampaignAsset, CampaignAssetType, CampaignMember, ContentPlan, PlanJob } from "../domain/types";
import { fromBrief, fromPlan, toAsset, toCampaign, toJob, toMember, type AssetRowDto, type CampaignDto, type CampaignMemberDto, type JobDto } from "./dto";

export class HttpCampaignRepository implements CampaignRepository {
  async list(q: CampaignQuery): Promise<Page<Campaign>> {
    const p = await http.page<CampaignDto>("/campaigns", {
      query: { brand_id: q.brandId, status: q.status, q: q.q, mine: q.mine ? "true" : undefined, cursor: q.cursor, limit: q.limit ?? 50 },
    });
    return { items: p.items.map(toCampaign), nextCursor: p.nextCursor };
  }

  async get(id: ID): Promise<Campaign> {
    return toCampaign(await http.get<CampaignDto>(`/campaigns/${id}`));
  }

  async create(input: CreateCampaignInput): Promise<Campaign> {
    const dto = await http.post<CampaignDto>("/campaigns", {
      brand_id: input.brandId,
      name: input.name.trim(),
      goal: input.goal,
      brief: fromBrief(input.brief),
      start_date: input.startDate || null,
      end_date: input.endDate || null,
      member_ids: input.memberIds,
    });
    return toCampaign(dto);
  }

  async update(id: ID, input: UpdateCampaignInput): Promise<Campaign> {
    const body: Record<string, unknown> = {};
    if (input.name !== undefined) body.name = input.name.trim();
    if (input.goal !== undefined) body.goal = input.goal;
    if (input.brief !== undefined) body.brief = fromBrief(input.brief);
    if (input.startDate !== undefined) body.start_date = input.startDate;
    if (input.endDate !== undefined) body.end_date = input.endDate;
    if (input.status !== undefined) body.status = input.status;
    if (input.memberIds !== undefined) body.member_ids = input.memberIds;
    return toCampaign(await http.patch<CampaignDto>(`/campaigns/${id}`, body));
  }

  async memberCandidates(): Promise<CampaignMember[]> {
    const p = await http.page<CampaignMemberDto>("/campaigns/member-candidates");
    return p.items.map(toMember);
  }

  async savePlan(id: ID, plan: ContentPlan): Promise<Campaign> {
    return toCampaign(await http.put<CampaignDto>(`/campaigns/${id}/plan`, { plan: fromPlan(plan) }));
  }

  async discardProposal(id: ID): Promise<void> {
    await http.delete(`/campaigns/${id}/plan-proposal`);
  }

  async generatePlan(id: ID, idempotencyKey: string): Promise<{ jobId: ID }> {
    const r = await http.post<{ job_id: string }>(`/campaigns/${id}/plan-generations`, {}, { idempotencyKey });
    return { jobId: r.job_id };
  }

  async job(jobId: ID): Promise<PlanJob> {
    return toJob(await http.get<JobDto>(`/jobs/${jobId}`));
  }

  async assets(campaignId: ID, type: CampaignAssetType, cursor?: string | null): Promise<Page<CampaignAsset>> {
    const p = await http.page<AssetRowDto>("/assets", { query: { campaign_id: campaignId, type, cursor, limit: 50 } });
    return { items: p.items.map(toAsset), nextCursor: p.nextCursor };
  }
}
