import type { ID } from "@/shared/kernel";
import type {
  BrandOption,
  CampaignSummary,
  ContentAsset,
  ContentFormat,
  ContentVersion,
  GenerateInput,
  JobAccepted,
  JobSnapshot,
  ProductOption,
  RewriteInput,
  SaveVersionInput,
} from "../domain/content";

export interface ContentRepository {
  generate(input: GenerateInput, idempotencyKey: string): Promise<JobAccepted>;
  rewrite(
    assetId: ID,
    input: RewriteInput,
    idempotencyKey: string,
  ): Promise<JobAccepted>;
  transform(
    assetId: ID,
    target: ContentFormat,
    idempotencyKey: string,
  ): Promise<JobAccepted>;
  /** GET /jobs/{id} (polled until terminal). */
  getJob(jobId: ID): Promise<JobSnapshot>;
  cancelJob(jobId: ID): Promise<void>;
  getAsset(assetId: ID): Promise<ContentAsset>;
  listVersions(assetId: ID): Promise<ContentVersion[]>;
  saveVersion(assetId: ID, input: SaveVersionInput): Promise<ContentVersion>;
  restoreVersion(assetId: ID, versionNo: number): Promise<ContentVersion>;
  listBrands(): Promise<BrandOption[]>;
  listProducts(brandId: ID): Promise<ProductOption[]>;
  getCampaign(campaignId: ID): Promise<CampaignSummary>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    content: ContentRepository;
  }
}
