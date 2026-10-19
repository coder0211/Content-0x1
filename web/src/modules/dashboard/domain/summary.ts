import type { CampaignStatus, ID, ISODateTime, JobStatus } from "@/shared/kernel";

export interface DashboardCampaign {
  id: ID;
  name: string;
  status: CampaignStatus;
  updatedAt: ISODateTime;
}

export interface DashboardJob {
  id: ID;
  jobType: string;
  status: JobStatus;
  progress: number;
  errorCode: string | null;
  createdAt: ISODateTime;
}

/** GET /dashboard/summary. A widget the user may not see is `null` (backend decides by permission). */
export interface DashboardSummary {
  campaigns: { activeCount: number; recent: DashboardCampaign[] } | null;
  jobs: { scope: "own" | "all"; queued: number; running: number; failed24h: number; recent: DashboardJob[] };
  reviews: { pendingReview: number | null; myPendingReview: number; myChangesRequested: number; myDrafts: number };
  cost: { currency: string; today: number; month: number; monthlyLimit: number | null; monthUsedPct: number | null } | null;
  generatedAt: ISODateTime;
}

export const JOB_TYPE_LABELS: Record<string, string> = {
  content_generation: "Sinh nội dung",
  content_rewrite: "Viết lại nội dung",
  content_transform: "Chuyển đổi nội dung",
  content_plan: "Lập kế hoạch nội dung",
  image_generation: "Sinh ảnh",
  image_edit: "Chỉnh sửa ảnh",
  image_prompt_suggest: "Gợi ý mô tả ảnh",
  video_storyboard: "Kịch bản & storyboard",
  video_production: "Sản xuất video",
  video_scene_regenerate: "Sinh lại cảnh",
  video_render: "Render video",
  brand_extraction: "Trích xuất brand",
};

export function jobTypeLabel(t: string): string {
  return JOB_TYPE_LABELS[t] ?? t;
}

export type BudgetTone = "neutral" | "warning" | "danger";

/** Budget usage tone: ≥100% danger, ≥80% warning (same thresholds as budget alerts). */
export function budgetTone(pct: number | null): BudgetTone {
  if (pct === null) return "neutral";
  if (pct >= 100) return "danger";
  if (pct >= 80) return "warning";
  return "neutral";
}

export interface TodoItem {
  label: string;
  count: number;
  to: string;
}

/** Actionable counters for the current user; zero counts are hidden. */
export function todoItems({ reviews }: DashboardSummary): TodoItem[] {
  const items: TodoItem[] = [
    { label: "Asset chờ bạn duyệt", count: reviews.pendingReview ?? 0, to: "/review" },
    { label: "Asset bị yêu cầu sửa", count: reviews.myChangesRequested, to: "/library?status=changes_requested" },
    { label: "Bản nháp chưa gửi duyệt", count: reviews.myDrafts, to: "/library?status=draft" },
    { label: "Asset của bạn đang chờ duyệt", count: reviews.myPendingReview, to: "/library?status=pending_review" },
  ];
  return items.filter((i) => i.count > 0);
}
