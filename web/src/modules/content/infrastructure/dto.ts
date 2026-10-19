/** snake_case DTOs (SPEC-04 §2.4, §2.7, §2.8) and mappers. */
import type { AssetStatus, JobStatus } from "@/shared/kernel";
import type {
  ContentAsset,
  ContentBody,
  ContentVersion,
  GenerateInput,
  JobAccepted,
  Provenance,
  RewriteInput,
  WarningType,
} from "../domain/content";

export interface MoneyDto {
  amount: number | string;
  currency: string;
  is_estimate?: boolean;
}

export interface JobAcceptedDto {
  job_id: string;
  status: JobStatus;
  estimated_cost?: MoneyDto | null;
}

export interface GenerateRequestDto {
  campaign_id: string | null;
  brand_id: string | null;
  platform: string;
  content_type: string;
  objective?: string;
  framework: string;
  length: string;
  language: string;
  tone: string[];
  brief: {
    product_id: string | null;
    target_audience: string;
    key_message: string;
    cta: string;
    reference_text: string;
  };
  variants: number;
}

export interface RewriteRequestDto {
  instruction: string;
  tone?: string[];
  custom_prompt?: string;
}

export interface ContentJsonDto {
  title?: string;
  hook?: string;
  body_html?: string;
  body_text?: string;
  cta?: string;
  hashtags?: string[] | null;
  platform?: string;
  format?: string;
  language?: string;
  scenes?:
    | { narration?: string; visual_hint?: string; duration_sec?: number }[]
    | null;
  warnings?: { type: string; text: string }[] | null;
  provenance?: {
    job_id?: string | null;
    provider_code?: string;
    model_code?: string;
    prompt_template_code?: string;
    prompt_template_version?: number | null;
    brand_version_no?: number | null;
  } | null;
}

export interface VersionDto {
  version_no: number;
  change_note?: string | null;
  created_at: string;
  generated_by_job_id?: string | null;
  content_json?: ContentJsonDto | null;
}

export interface AssetDto {
  id: string;
  asset_type?: string;
  subtype?: string | null;
  title: string;
  status: AssetStatus;
  campaign_id?: string | null;
  brand_id?: string | null;
  source_asset_id?: string | null;
  current_version?: VersionDto | null;
}

export interface BrandDto {
  id: string;
  name: string;
}
export interface ProductDto {
  id: string;
  name: string;
}
export interface CampaignDto {
  id: string;
  name: string;
  brand_id: string;
  goal?: string | null;
  status: string;
}

export function toGenerateDto(i: GenerateInput): GenerateRequestDto {
  return {
    campaign_id: i.campaignId,
    brand_id: i.brandId,
    platform: i.platform,
    content_type: i.contentType,
    ...(i.objective ? { objective: i.objective } : {}),
    framework: i.framework === "none" ? "" : i.framework,
    length: i.length,
    language: i.language,
    tone: i.tone,
    brief: {
      product_id: i.brief.productId,
      target_audience: i.brief.targetAudience,
      key_message: i.brief.keyMessage,
      cta: i.brief.cta,
      reference_text: i.brief.referenceText,
    },
    variants: i.variants,
  };
}

export function toRewriteDto(i: RewriteInput): RewriteRequestDto {
  return {
    instruction: i.instruction,
    ...(i.tone?.length ? { tone: i.tone } : {}),
    ...(i.customPrompt ? { custom_prompt: i.customPrompt } : {}),
  };
}

export function fromJobAccepted(d: JobAcceptedDto): JobAccepted {
  const c = d.estimated_cost;
  return {
    jobId: d.job_id,
    status: d.status,
    estimatedCost: c
      ? {
          amount: Number(c.amount),
          currency: c.currency === "VND" ? "VND" : "USD",
        }
      : null,
  };
}

function fromProvenance(p: ContentJsonDto["provenance"]): Provenance | null {
  if (!p) return null;
  return {
    jobId: p.job_id ?? null,
    providerCode: p.provider_code ?? "",
    modelCode: p.model_code ?? "",
    promptTemplateCode: p.prompt_template_code ?? "",
    promptTemplateVersion: p.prompt_template_version ?? null,
    brandVersionNo: p.brand_version_no ?? null,
  };
}

export function fromContentJson(
  c: ContentJsonDto | null | undefined,
): ContentBody | null {
  if (!c) return null;
  return {
    title: c.title ?? "",
    hook: c.hook ?? "",
    bodyText: c.body_text ?? "",
    bodyHtml: c.body_html ?? "",
    cta: c.cta ?? "",
    hashtags: c.hashtags ?? [],
    platform: c.platform ?? "",
    format: c.format ?? "",
    language: c.language ?? "vi",
    scenes: (c.scenes ?? []).map((s) => ({
      narration: s.narration ?? "",
      visualHint: s.visual_hint ?? "",
      durationSec: s.duration_sec ?? 0,
    })),
    warnings: (c.warnings ?? [])
      .filter(
        (w) => w.type === "forbidden_word" || w.type === "unverified_claim",
      )
      .map((w) => ({ type: w.type as WarningType, text: w.text })),
    provenance: fromProvenance(c.provenance),
  };
}

export function toContentJson(c: ContentBody): ContentJsonDto {
  return {
    title: c.title,
    hook: c.hook,
    body_html: c.bodyHtml,
    body_text: c.bodyText,
    cta: c.cta,
    hashtags: c.hashtags,
    platform: c.platform,
    format: c.format,
    language: c.language,
    scenes: c.scenes.map((s) => ({
      narration: s.narration,
      visual_hint: s.visualHint,
      duration_sec: s.durationSec,
    })),
    warnings: c.warnings,
    provenance: c.provenance
      ? {
          job_id: c.provenance.jobId,
          provider_code: c.provenance.providerCode,
          model_code: c.provenance.modelCode,
          prompt_template_code: c.provenance.promptTemplateCode,
          prompt_template_version: c.provenance.promptTemplateVersion,
          brand_version_no: c.provenance.brandVersionNo,
        }
      : null,
  };
}

export function fromVersion(v: VersionDto): ContentVersion {
  return {
    versionNo: v.version_no,
    changeNote: v.change_note ?? "",
    createdAt: v.created_at,
    generatedByJobId: v.generated_by_job_id ?? null,
    content: fromContentJson(v.content_json),
  };
}

export function fromAsset(a: AssetDto): ContentAsset {
  return {
    id: a.id,
    title: a.title,
    status: a.status,
    subtype: a.subtype ?? "",
    campaignId: a.campaign_id ?? null,
    brandId: a.brand_id ?? null,
    sourceAssetId: a.source_asset_id ?? null,
    currentVersion: a.current_version ? fromVersion(a.current_version) : null,
  };
}
