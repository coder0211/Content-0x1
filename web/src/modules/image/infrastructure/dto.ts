import type { AssetStatus, JobStatus } from "@/shared/kernel";
import type { ImageBatch, ImageEstimate, Operation, Preset, ReferenceImage } from "../domain/image";

/** snake_case DTOs mirroring api/openapi/image.yaml. */
export interface MoneyDto {
  amount: number;
  currency: string;
  is_estimate?: boolean;
}

export interface ImageFileDto {
  file_id: string;
  role: "primary" | "variant";
  sort_order: number;
  mime_type: string;
  width: number;
  height: number;
  parent_file_id: string | null;
  url: string;
  thumbnail_file_id: string | null;
  thumbnail_url: string;
  url_expires_at: string;
}

export interface ImageBatchDto {
  id: string;
  operation: Operation;
  campaign_id: string | null;
  brand_id: string;
  prompt: string;
  preset: Preset;
  aspect_ratio: string;
  quality: string;
  count: number;
  model_code: string;
  source_file_id: string | null;
  created_by: string;
  created_at: string;
  job: {
    id: string;
    status: JobStatus;
    progress: number;
    error_code: string | null;
    error_kind: string | null;
    error_message: string | null;
    cost_estimate: MoneyDto | null;
    cost_actual: MoneyDto | null;
  };
  asset: {
    id: string;
    title: string;
    status: AssetStatus;
    version_no: number;
    current_version_no: number;
    requires_typography_check: boolean;
    images: ImageFileDto[];
  } | null;
}

export interface ImageEstimateDto {
  total: MoneyDto;
  lines: { item: string; provider_code: string; model_code: string; unit: string; units: number; amount: MoneyDto }[];
  pricing_version: string;
  per_job_limit: MoneyDto;
  campaign_remaining: MoneyDto | null;
  global_remaining: MoneyDto;
  requires_override: boolean;
  blocked: boolean;
}

export interface ImageReferenceDto {
  file_id: string;
  source: "brand" | "library";
  label: string;
  mime_type: string;
  width: number;
  height: number;
  url: string;
  thumbnail_url: string;
  url_expires_at: string;
}

export interface JobAcceptedDto {
  job_id: string;
  status: string;
  estimated_cost?: MoneyDto | null;
}

export interface GenerationRequestDto {
  campaign_id: string | null;
  brand_id: string | null;
  prompt: string;
  preset: Preset;
  reference_file_ids: string[];
  aspect_ratio: string;
  quality: string;
  count: number;
  model_code: null;
  budget_override: boolean;
}

export interface UploadTicketDto {
  file_id: string;
  url: string;
  method: "PUT";
  headers: Record<string, string>;
  expires_at: string;
}

/** GET /jobs/{id} (SPEC-04 §2.7) — only the fields Image Studio reads. */
export interface JobDto {
  id: string;
  status: JobStatus;
  result: { prompts?: string[] } | null;
  error: { code?: string; kind?: string; message?: string } | null;
}

export interface CampaignDto {
  id: string;
  name: string;
  brand_id: string;
  status: string;
}

export interface BrandDto {
  id: string;
  name: string;
}

const money = (m: MoneyDto, isEstimate: boolean) => ({
  amount: m.amount,
  currency: (m.currency === "VND" ? "VND" : "USD") as "USD" | "VND",
  isEstimate,
});

export function toBatch(d: ImageBatchDto): ImageBatch {
  return {
    id: d.id,
    operation: d.operation,
    campaignId: d.campaign_id,
    brandId: d.brand_id,
    prompt: d.prompt,
    preset: d.preset,
    aspectRatio: d.aspect_ratio,
    quality: d.quality,
    count: d.count,
    modelCode: d.model_code,
    sourceFileId: d.source_file_id,
    createdAt: d.created_at,
    job: {
      id: d.job.id,
      status: d.job.status,
      progress: d.job.progress,
      errorCode: d.job.error_code,
      errorKind: d.job.error_kind,
      errorMessage: d.job.error_message,
      costEstimate: d.job.cost_estimate ? money(d.job.cost_estimate, true) : null,
      costActual: d.job.cost_actual ? money(d.job.cost_actual, false) : null,
    },
    asset: d.asset
      ? {
          id: d.asset.id,
          title: d.asset.title,
          status: d.asset.status,
          versionNo: d.asset.version_no,
          currentVersionNo: d.asset.current_version_no,
          requiresTypographyCheck: d.asset.requires_typography_check,
          images: d.asset.images.map((i) => ({
            fileId: i.file_id,
            role: i.role,
            sortOrder: i.sort_order,
            mimeType: i.mime_type,
            width: i.width,
            height: i.height,
            parentFileId: i.parent_file_id,
            url: i.url,
            thumbnailUrl: i.thumbnail_url || i.url,
          })),
        }
      : null,
  };
}

export function toEstimate(d: ImageEstimateDto): ImageEstimate {
  return {
    total: money(d.total, true),
    perJobLimit: money(d.per_job_limit, false),
    campaignRemaining: d.campaign_remaining ? money(d.campaign_remaining, false) : null,
    globalRemaining: money(d.global_remaining, false),
    requiresOverride: d.requires_override,
    blocked: d.blocked,
    pricingVersion: d.pricing_version,
  };
}

export const toReference = (d: ImageReferenceDto): ReferenceImage => ({
  fileId: d.file_id,
  source: d.source,
  label: d.label,
  thumbnailUrl: d.thumbnail_url || d.url,
});
