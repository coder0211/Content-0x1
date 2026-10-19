/** snake_case DTOs mirroring api/openapi/budget.yaml + mappers. */
import type {
  Budget,
  BudgetSettings,
  BudgetSettingsPatch,
  CampaignBudgetInput,
  Estimate,
  EstimateRequest,
  GroupBy,
  UsageRow,
  UsageSummary,
  UsageTotals,
} from "../domain/budget";

export interface EstimateRequestDto {
  kind: string;
  provider?: string;
  model?: string;
  campaign_id?: string | null;
  prompt_chars?: number;
  max_output_tokens?: number;
  count?: number;
  size?: string;
  quality?: string;
  clip_seconds?: number;
  resolution?: string;
  voiceover_chars?: number;
}

export interface EstimateDto {
  breakdown: { item: string; provider: string; model: string; unit: string; units: number; unit_price: number; amount: number }[];
  total: { amount: number; currency: string; is_estimate?: boolean };
  pricing_version: string;
  limits: { per_job_limit: number; campaign_remaining: number | null; global_remaining: number };
  requires_override: boolean;
  blocked: boolean;
}

export interface BudgetDto {
  id: string;
  scope: "global" | "campaign";
  campaign_id: string | null;
  campaign_name?: string | null;
  period_start: string;
  period_end: string;
  limit_amount: number;
  reserved_amount: number;
  spent_amount: number;
  remaining_amount: number;
  percent_used: number;
  currency: string;
  alert_thresholds: number[];
  alerts_sent: number[];
  hard_stop: boolean;
  updated_at: string;
}

export interface BudgetSettingsDto {
  currency: string;
  per_job_limits: { text: number; image: number; video: number };
  global_template: { monthly_limit: number; alert_thresholds: number[]; hard_stop: boolean };
  global: BudgetDto;
  campaigns: BudgetDto[];
  max_concurrent_video_jobs_per_user: number;
  updated_by: string | null;
  updated_at: string;
}

export interface UsageTotalsDto {
  provider_reported: number;
  calculated: number;
  estimated: number;
  total: number;
  count: number;
}

export interface UsageSummaryDto {
  from: string;
  to: string;
  group_by: GroupBy;
  currency: string;
  rows: (UsageTotalsDto & { key: string | null; label: string })[];
  totals: UsageTotalsDto;
}

export function toEstimateRequestDto(r: EstimateRequest): EstimateRequestDto {
  const dto: EstimateRequestDto = { kind: r.kind };
  if (r.provider) dto.provider = r.provider;
  if (r.model) dto.model = r.model;
  if (r.campaignId) dto.campaign_id = r.campaignId;
  if (r.promptChars !== undefined) dto.prompt_chars = r.promptChars;
  if (r.maxOutputTokens !== undefined) dto.max_output_tokens = r.maxOutputTokens;
  if (r.count !== undefined) dto.count = r.count;
  if (r.size) dto.size = r.size;
  if (r.quality) dto.quality = r.quality;
  if (r.clipSeconds !== undefined) dto.clip_seconds = r.clipSeconds;
  if (r.resolution) dto.resolution = r.resolution;
  if (r.voiceoverChars !== undefined) dto.voiceover_chars = r.voiceoverChars;
  return dto;
}

export function toEstimate(d: EstimateDto): Estimate {
  return {
    lines: d.breakdown.map((l) => ({ item: l.item, provider: l.provider, model: l.model, unit: l.unit, units: l.units, unitPrice: l.unit_price, amount: l.amount })),
    total: d.total.amount,
    currency: d.total.currency,
    pricingVersion: d.pricing_version,
    limits: { perJobLimit: d.limits.per_job_limit, campaignRemaining: d.limits.campaign_remaining, globalRemaining: d.limits.global_remaining },
    requiresOverride: d.requires_override,
    blocked: d.blocked,
  };
}

export function toBudget(d: BudgetDto): Budget {
  return {
    id: d.id,
    scope: d.scope,
    campaignId: d.campaign_id,
    campaignName: d.campaign_name ?? null,
    periodStart: d.period_start,
    periodEnd: d.period_end,
    limitAmount: d.limit_amount,
    reservedAmount: d.reserved_amount,
    spentAmount: d.spent_amount,
    remainingAmount: d.remaining_amount,
    percentUsed: d.percent_used,
    currency: d.currency,
    alertThresholds: d.alert_thresholds,
    alertsSent: d.alerts_sent,
    hardStop: d.hard_stop,
    updatedAt: d.updated_at,
  };
}

export function toSettings(d: BudgetSettingsDto): BudgetSettings {
  return {
    currency: d.currency,
    perJobLimits: { ...d.per_job_limits },
    globalTemplate: { monthlyLimit: d.global_template.monthly_limit, alertThresholds: d.global_template.alert_thresholds, hardStop: d.global_template.hard_stop },
    global: toBudget(d.global),
    campaigns: d.campaigns.map(toBudget),
    maxConcurrentVideoJobs: d.max_concurrent_video_jobs_per_user,
    updatedAt: d.updated_at,
  };
}

export function toSettingsPatchDto(p: BudgetSettingsPatch) {
  const dto: Record<string, unknown> = {};
  if (p.perJobLimits) dto.per_job_limits = { ...p.perJobLimits };
  if (p.global) {
    const g: Record<string, unknown> = {};
    if (p.global.limitAmount !== undefined) g.limit_amount = p.global.limitAmount;
    if (p.global.alertThresholds !== undefined) g.alert_thresholds = p.global.alertThresholds;
    if (p.global.hardStop !== undefined) g.hard_stop = p.global.hardStop;
    dto.global = g;
  }
  if (p.maxConcurrentVideoJobs !== undefined) dto.max_concurrent_video_jobs_per_user = p.maxConcurrentVideoJobs;
  return dto;
}

export function toCampaignBudgetDto(i: CampaignBudgetInput) {
  const dto: Record<string, unknown> = { limit_amount: i.limitAmount };
  if (i.periodStart) dto.period_start = i.periodStart;
  if (i.periodEnd) dto.period_end = i.periodEnd;
  if (i.alertThresholds) dto.alert_thresholds = i.alertThresholds;
  if (i.hardStop !== undefined) dto.hard_stop = i.hardStop;
  return dto;
}

function toTotals(t: UsageTotalsDto): UsageTotals {
  return { providerReported: t.provider_reported, calculated: t.calculated, estimated: t.estimated, total: t.total, count: t.count };
}

export function toSummary(d: UsageSummaryDto): UsageSummary {
  return {
    from: d.from,
    to: d.to,
    groupBy: d.group_by,
    currency: d.currency,
    rows: d.rows.map((r): UsageRow => ({ key: r.key, label: r.label, ...toTotals(r) })),
    totals: toTotals(d.totals),
  };
}
