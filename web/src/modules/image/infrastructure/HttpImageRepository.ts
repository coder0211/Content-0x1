import { http } from "@/shared/api/http";
import { ApiError, type ID, type Page } from "@/shared/kernel";
import type { GenerationOptions, ImageBatch, ImageEstimate, Preset, ReferenceImage, StudioTarget } from "../domain/image";
import type {
  BatchQuery,
  EditInput,
  EstimateQuery,
  ImageRepository,
  JobAccepted,
  TargetOption,
  VariationInput,
} from "../application/ports";
import {
  toBatch,
  toEstimate,
  toReference,
  type BrandDto,
  type CampaignDto,
  type GenerationRequestDto,
  type ImageBatchDto,
  type ImageEstimateDto,
  type ImageReferenceDto,
  type JobAcceptedDto,
  type UploadTicketDto,
} from "./dto";

const accepted = (d: JobAcceptedDto): JobAccepted => ({
  jobId: d.job_id,
  status: d.status,
  estimatedCost: d.estimated_cost
    ? { amount: d.estimated_cost.amount, currency: d.estimated_cost.currency, isEstimate: true }
    : null,
});

const targetQuery = (t: StudioTarget) => (t.campaignId ? { campaign_id: t.campaignId } : { brand_id: t.brandId });

/** Campaigns that can no longer receive generated assets. */
const CLOSED_CAMPAIGN = new Set(["completed", "archived"]);

export class HttpImageRepository implements ImageRepository {
  async listBatches(q: BatchQuery): Promise<Page<ImageBatch>> {
    const page = await http.page<ImageBatchDto>("/images/generations", {
      query: { campaign_id: q.campaignId, asset_id: q.assetId, cursor: q.cursor, limit: 20 },
    });
    return { items: page.items.map(toBatch), nextCursor: page.nextCursor };
  }

  async estimate(q: EstimateQuery): Promise<ImageEstimate> {
    const d = await http.post<ImageEstimateDto>("/images/estimate", {
      campaign_id: q.campaignId ?? null,
      operation: q.operation,
      aspect_ratio: q.aspectRatio,
      quality: q.quality,
      count: q.count,
    });
    return toEstimate(d);
  }

  async generate(t: StudioTarget, o: GenerationOptions, idempotencyKey: string): Promise<JobAccepted> {
    const body: GenerationRequestDto = {
      campaign_id: t.campaignId ?? null,
      brand_id: t.campaignId ? null : (t.brandId ?? null),
      prompt: o.prompt.trim(),
      preset: o.preset,
      reference_file_ids: o.referenceFileIds,
      aspect_ratio: o.aspectRatio,
      quality: o.quality,
      count: o.count,
      model_code: null,
      budget_override: o.budgetOverride,
    };
    return accepted(await http.post<JobAcceptedDto>("/images/generations", body, { idempotencyKey }));
  }

  async edit(assetId: ID, input: EditInput, idempotencyKey: string): Promise<JobAccepted> {
    const body = { file_id: input.fileId, prompt: input.prompt.trim(), mask_file_id: null, budget_override: input.budgetOverride ?? false };
    return accepted(await http.post<JobAcceptedDto>(`/images/${assetId}/edits`, body, { idempotencyKey }));
  }

  async vary(assetId: ID, input: VariationInput, idempotencyKey: string): Promise<JobAccepted> {
    const body = { file_id: input.fileId, count: input.count, budget_override: input.budgetOverride ?? false };
    return accepted(await http.post<JobAcceptedDto>(`/images/${assetId}/variations`, body, { idempotencyKey }));
  }

  async listReferences(t: StudioTarget): Promise<ReferenceImage[]> {
    const page = await http.page<ImageReferenceDto>("/images/references", { query: targetQuery(t) });
    return page.items.map(toReference);
  }

  async uploadReference(file: File): Promise<ID> {
    const ticket = await http.post<UploadTicketDto>("/files/upload-url", {
      filename: file.name,
      mime_type: file.type,
      size_bytes: file.size,
      purpose: "reference_image",
    });
    // The presigned URL points at object storage, not /api/v1: plain fetch, no cookies.
    let res: Response;
    try {
      res = await fetch(ticket.url, { method: ticket.method ?? "PUT", headers: ticket.headers, body: file, credentials: "omit" });
    } catch {
      throw new ApiError({ code: "NETWORK_ERROR", message: "Không tải được ảnh lên", status: 0 });
    }
    if (!res.ok) throw new ApiError({ code: "UNKNOWN", message: "Không tải được ảnh lên", status: res.status });
    const done = await http.post<{ id: string }>(`/files/${ticket.file_id}/complete`);
    return done.id ?? ticket.file_id;
  }

  async requestPromptSuggestions(input: { campaignId?: ID; brandId?: ID; preset: Preset }, idempotencyKey: string): Promise<ID> {
    // ContentImagePromptRequest needs campaign_id or brand_id (brand-only studio target has no campaign).
    const body = input.campaignId ? { campaign_id: input.campaignId, preset: input.preset } : { brand_id: input.brandId ?? null, preset: input.preset };
    const d = await http.post<JobAcceptedDto>("/images/prompt-suggestions", body, { idempotencyKey });
    return d.job_id;
  }

  async retryJob(jobId: ID): Promise<void> {
    await http.post(`/jobs/${jobId}/retry`, {});
  }

  async submitForReview(assetId: ID, checklist: { typographyChecked: boolean }): Promise<void> {
    await http.post(`/assets/${assetId}/submit-review`, { checklist: { typography_checked: checklist.typographyChecked } });
  }

  async listCampaigns(): Promise<TargetOption[]> {
    const page = await http.page<CampaignDto>("/campaigns", { query: { limit: 100 } });
    return page.items.filter((c) => !CLOSED_CAMPAIGN.has(c.status)).map((c) => ({ id: c.id, name: c.name, brandId: c.brand_id }));
  }

  async listBrands(): Promise<TargetOption[]> {
    const page = await http.page<BrandDto>("/brands", { query: { limit: 100 } });
    return page.items.map((b) => ({ id: b.id, name: b.name, brandId: b.id }));
  }
}
