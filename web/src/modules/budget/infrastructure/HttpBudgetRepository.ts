import { http } from "@/shared/api/http";
import type { BudgetRepository } from "../application/ports";
import type { BudgetSettingsPatch, CampaignBudgetInput, EstimateRequest, UsageSummaryQuery } from "../domain/budget";
import {
  toBudget,
  toCampaignBudgetDto,
  toEstimate,
  toEstimateRequestDto,
  toSettings,
  toSettingsPatchDto,
  toSummary,
  type BudgetDto,
  type BudgetSettingsDto,
  type EstimateDto,
  type UsageSummaryDto,
} from "./dto";

export class HttpBudgetRepository implements BudgetRepository {
  async estimate(req: EstimateRequest) {
    return toEstimate(await http.post<EstimateDto>("/cost-estimates", toEstimateRequestDto(req)));
  }

  async usageSummary(q: UsageSummaryQuery) {
    return toSummary(await http.get<UsageSummaryDto>("/usage/summary", { query: { from: q.from, to: q.to, group_by: q.groupBy } }));
  }

  async getSettings() {
    return toSettings(await http.get<BudgetSettingsDto>("/settings/budgets"));
  }

  async updateSettings(patch: BudgetSettingsPatch) {
    return toSettings(await http.patch<BudgetSettingsDto>("/settings/budgets", toSettingsPatchDto(patch)));
  }

  async putCampaignBudget(campaignId: string, input: CampaignBudgetInput) {
    return toBudget(await http.put<BudgetDto>(`/campaigns/${encodeURIComponent(campaignId)}/budget`, toCampaignBudgetDto(input)));
  }
}
