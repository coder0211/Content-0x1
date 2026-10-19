import type { ID, Page } from "@/shared/kernel";
import type {
  GenerationOptions,
  ImageBatch,
  ImageEstimate,
  Operation,
  Preset,
  Quality,
  ReferenceImage,
  StudioTarget,
} from "../domain/image";

export interface BatchQuery {
  campaignId?: ID;
  assetId?: ID;
  cursor?: string | null;
}

export interface EstimateQuery {
  campaignId?: ID;
  operation: Operation;
  aspectRatio: string;
  quality: Quality;
  count: number;
}

export interface JobAccepted {
  jobId: ID;
  status: string;
  estimatedCost: { amount: number; currency: string; isEstimate: boolean } | null;
}

export interface EditInput {
  fileId: ID;
  prompt: string;
  budgetOverride?: boolean;
}

export interface VariationInput {
  fileId: ID;
  count: number;
  budgetOverride?: boolean;
}

export interface PromptSuggestionResult {
  status: "queued" | "running" | "succeeded" | "failed" | "cancelled";
  prompts: string[];
  errorKind: string | null;
}

/** Campaign or brand the user can generate for (the Studio target picker). */
export interface TargetOption {
  id: ID;
  name: string;
  brandId: ID | null;
}

export interface ImageRepository {
  listBatches(q: BatchQuery): Promise<Page<ImageBatch>>;
  estimate(q: EstimateQuery): Promise<ImageEstimate>;
  generate(target: StudioTarget, o: GenerationOptions, idempotencyKey: string): Promise<JobAccepted>;
  edit(assetId: ID, input: EditInput, idempotencyKey: string): Promise<JobAccepted>;
  vary(assetId: ID, input: VariationInput, idempotencyKey: string): Promise<JobAccepted>;
  listReferences(target: StudioTarget): Promise<ReferenceImage[]>;
  /** Presigned upload: upload-url → PUT → complete. Returns the ready file id. */
  uploadReference(file: File): Promise<ID>;
  /** POST /images/prompt-suggestions (202) → job id. */
  requestPromptSuggestions(input: { campaignId?: ID; preset: Preset }, idempotencyKey: string): Promise<ID>;
  /** GET /jobs/{id} → `result.prompts[]`. */
  getPromptSuggestions(jobId: ID): Promise<PromptSuggestionResult>;
  /** POST /jobs/{id}/retry (job module; only failed jobs with a retryable error). */
  retryJob(jobId: ID): Promise<void>;
  /** POST /assets/{id}/submit-review with the FR-IMG-06 checklist. */
  submitForReview(assetId: ID, checklist: { typographyChecked: boolean }): Promise<void>;
  listCampaigns(): Promise<TargetOption[]>;
  listBrands(): Promise<TargetOption[]>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    image: ImageRepository;
  }
}
