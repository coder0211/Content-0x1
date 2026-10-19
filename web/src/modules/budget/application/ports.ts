import type {
  BudgetSettings,
  BudgetSettingsPatch,
  Budget,
  CampaignBudgetInput,
  Estimate,
  EstimateRequest,
  UsageSummary,
  UsageSummaryQuery,
} from "../domain/budget";

export interface BudgetRepository {
  estimate(req: EstimateRequest): Promise<Estimate>;
  usageSummary(q: UsageSummaryQuery): Promise<UsageSummary>;
  getSettings(): Promise<BudgetSettings>;
  updateSettings(patch: BudgetSettingsPatch): Promise<BudgetSettings>;
  putCampaignBudget(campaignId: string, input: CampaignBudgetInput): Promise<Budget>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    budget: BudgetRepository;
  }
}
