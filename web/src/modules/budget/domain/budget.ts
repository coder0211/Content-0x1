/** Budget & usage domain types and pure rules (SPEC-02 §6, SPEC-04 §2.9). */

export type EstimateKind = "text" | "image" | "video";

/** Input of the generic estimator (POST /cost-estimates). Empty provider/model → default model. */
export interface EstimateRequest {
  kind: EstimateKind;
  provider?: string;
  model?: string;
  campaignId?: string | null;
  /** text */
  promptChars?: number;
  maxOutputTokens?: number;
  /** image */
  count?: number;
  size?: string;
  quality?: "standard" | "high";
  /** video */
  clipSeconds?: number;
  resolution?: "720p" | "1080p";
  voiceoverChars?: number;
}

export interface EstimateLine {
  item: string;
  provider: string;
  model: string;
  unit: string;
  units: number;
  unitPrice: number;
  amount: number;
}

export interface BudgetLimits {
  perJobLimit: number;
  campaignRemaining: number | null;
  globalRemaining: number;
}

export interface Estimate {
  lines: EstimateLine[];
  total: number;
  currency: string;
  pricingVersion: string;
  limits: BudgetLimits;
  /** Estimate exceeds the per-job limit (Lead/Admin may override). */
  requiresOverride: boolean;
  /** A hard-stop budget would refuse the job. */
  blocked: boolean;
}

export type BudgetScope = "global" | "campaign";

export interface Budget {
  id: string;
  scope: BudgetScope;
  campaignId: string | null;
  campaignName: string | null;
  periodStart: string;
  periodEnd: string;
  limitAmount: number;
  reservedAmount: number;
  spentAmount: number;
  remainingAmount: number;
  percentUsed: number;
  currency: string;
  alertThresholds: number[];
  alertsSent: number[];
  hardStop: boolean;
  updatedAt: string;
}

export interface PerJobLimits {
  text: number;
  image: number;
  video: number;
}

export interface BudgetSettings {
  currency: string;
  perJobLimits: PerJobLimits;
  globalTemplate: { monthlyLimit: number; alertThresholds: number[]; hardStop: boolean };
  global: Budget;
  campaigns: Budget[];
  maxConcurrentVideoJobs: number;
  updatedAt: string;
}

export interface BudgetSettingsPatch {
  perJobLimits?: Partial<PerJobLimits>;
  global?: { limitAmount?: number; alertThresholds?: number[]; hardStop?: boolean };
  maxConcurrentVideoJobs?: number;
}

export interface CampaignBudgetInput {
  limitAmount: number;
  periodStart?: string;
  periodEnd?: string;
  alertThresholds?: number[];
  hardStop?: boolean;
}

export const GROUP_BYS = ["day", "campaign", "provider", "model", "user", "job_type"] as const;
export type GroupBy = (typeof GROUP_BYS)[number];

export const GROUP_BY_LABELS: Record<GroupBy, string> = {
  day: "Theo ngày",
  campaign: "Campaign",
  provider: "Nhà cung cấp",
  model: "Model",
  user: "Người dùng",
  job_type: "Loại job",
};

export const COST_SOURCES = ["provider_reported", "calculated", "estimated"] as const;
export type CostSource = (typeof COST_SOURCES)[number];

/** UI labels for cost_source (SPEC-02 §6.3: the label must be explicit). */
export const COST_SOURCE_LABELS: Record<CostSource, string> = {
  provider_reported: "Nhà cung cấp báo",
  calculated: "Tính theo bảng giá",
  estimated: "Ước tính",
};

export interface UsageTotals {
  providerReported: number;
  calculated: number;
  estimated: number;
  total: number;
  count: number;
}

export interface UsageRow extends UsageTotals {
  /** null = none (e.g. usage without campaign). */
  key: string | null;
  label: string;
}

export interface UsageSummaryQuery {
  from: string;
  to: string;
  groupBy: GroupBy;
}

export interface UsageSummary {
  from: string;
  to: string;
  groupBy: GroupBy;
  currency: string;
  rows: UsageRow[];
  totals: UsageTotals;
}

export function sourceAmount(t: UsageTotals, s: CostSource): number {
  return s === "provider_reported" ? t.providerReported : s === "calculated" ? t.calculated : t.estimated;
}

/** Sum of rows, rounded to micro-dollars like the backend's NUMERIC(14,6). */
export function sumRows(rows: UsageTotals[]): UsageTotals {
  const r6 = (n: number) => Math.round(n * 1e6) / 1e6;
  const acc = rows.reduce(
    (a, r) => ({
      providerReported: a.providerReported + r.providerReported,
      calculated: a.calculated + r.calculated,
      estimated: a.estimated + r.estimated,
      total: a.total + r.total,
      count: a.count + r.count,
    }),
    { providerReported: 0, calculated: 0, estimated: 0, total: 0, count: 0 },
  );
  return { ...acc, providerReported: r6(acc.providerReported), calculated: r6(acc.calculated), estimated: r6(acc.estimated), total: r6(acc.total) };
}

/** `true` when the API totals equal the sum of rows (to the micro-dollar). */
export function totalsMatchRows(summary: Pick<UsageSummary, "rows" | "totals">): boolean {
  const s = sumRows(summary.rows);
  return Math.abs(s.total - summary.totals.total) < 1e-6 && s.count === summary.totals.count;
}

/**
 * Parse "80, 100" into sorted unique thresholds 1..100 (max 5).
 * Returns an error message (Vietnamese) instead when invalid.
 */
export function parseThresholds(text: string): { ok: true; value: number[] } | { ok: false; error: string } {
  const parts = text
    .split(/[,;\s]+/)
    .map((p) => p.trim())
    .filter(Boolean);
  if (parts.length === 0) return { ok: false, error: "Nhập ít nhất một ngưỡng." };
  if (parts.length > 5) return { ok: false, error: "Tối đa 5 ngưỡng." };
  const nums = parts.map(Number);
  if (nums.some((n) => !Number.isInteger(n) || n < 1 || n > 100)) return { ok: false, error: "Ngưỡng là số nguyên từ 1 đến 100." };
  if (new Set(nums).size !== nums.length) return { ok: false, error: "Các ngưỡng không được trùng nhau." };
  return { ok: true, value: [...nums].sort((a, b) => a - b) };
}

/** Parse a money input ("12.5" or "12,5"); null when invalid or negative. */
export function parseAmount(text: string): number | null {
  const t = text.trim().replace(",", ".");
  if (!/^\d+(\.\d{1,6})?$/.test(t)) return null;
  return Number(t);
}

/** Usage tone for a budget meter: ok < 80% ≤ warning < 100% ≤ danger. */
export function budgetTone(percentUsed: number): "ok" | "warning" | "danger" {
  if (percentUsed >= 100) return "danger";
  if (percentUsed >= 80) return "warning";
  return "ok";
}

/** Default range for the usage page: first day of the month → today (UTC dates, YYYY-MM-DD). */
export function currentMonthRange(now: Date = new Date()): { from: string; to: string } {
  const iso = (d: Date) => d.toISOString().slice(0, 10);
  const first = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), 1));
  return { from: iso(first), to: iso(now) };
}
