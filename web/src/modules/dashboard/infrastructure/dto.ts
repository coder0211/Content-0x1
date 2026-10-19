import type { CampaignStatus, JobStatus } from "@/shared/kernel";
import type { DashboardSummary } from "../domain/summary";

export interface DashboardSummaryDto {
  campaigns: { active_count: number; recent: { id: string; name: string; status: CampaignStatus; updated_at: string }[] } | null;
  jobs: {
    scope: "own" | "all";
    queued: number;
    running: number;
    failed_24h: number;
    recent: { id: string; job_type: string; status: JobStatus; progress: number; error_code: string | null; created_at: string }[];
  };
  reviews: { pending_review: number | null; my_pending_review: number; my_changes_requested: number; my_drafts: number };
  cost: { currency: string; today: number; month: number; monthly_limit: number | null; month_used_pct: number | null } | null;
  generated_at: string;
}

export function toSummary(d: DashboardSummaryDto): DashboardSummary {
  return {
    campaigns: d.campaigns
      ? { activeCount: d.campaigns.active_count, recent: d.campaigns.recent.map((c) => ({ id: c.id, name: c.name, status: c.status, updatedAt: c.updated_at })) }
      : null,
    jobs: {
      scope: d.jobs.scope,
      queued: d.jobs.queued,
      running: d.jobs.running,
      failed24h: d.jobs.failed_24h,
      recent: d.jobs.recent.map((j) => ({
        id: j.id,
        jobType: j.job_type,
        status: j.status,
        progress: j.progress,
        errorCode: j.error_code,
        createdAt: j.created_at,
      })),
    },
    reviews: {
      pendingReview: d.reviews.pending_review,
      myPendingReview: d.reviews.my_pending_review,
      myChangesRequested: d.reviews.my_changes_requested,
      myDrafts: d.reviews.my_drafts,
    },
    cost: d.cost
      ? { currency: d.cost.currency, today: d.cost.today, month: d.cost.month, monthlyLimit: d.cost.monthly_limit, monthUsedPct: d.cost.month_used_pct }
      : null,
    generatedAt: d.generated_at,
  };
}
