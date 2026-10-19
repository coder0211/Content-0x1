import type { EstimateRequest, UsageSummaryQuery } from "../domain/budget";

export const budgetKeys = {
  all: ["budget"] as const,
  estimate: (req: EstimateRequest) => ["budget", "estimate", req] as const,
  summary: (q: UsageSummaryQuery) => ["budget", "summary", q] as const,
  settings: () => ["budget", "settings"] as const,
  campaign: (campaignId: string) => ["budget", "campaign", campaignId] as const,
};
