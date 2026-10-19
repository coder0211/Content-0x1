import type { ID } from "@/shared/kernel";
import type {
  BrandOption,
  CampaignSummary,
  ContentAsset,
  ContentFormat,
  ContentVersion,
  GenerateInput,
  JobAccepted,
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
  getAsset(assetId: ID): Promise<ContentAsset>;
  listVersions(assetId: ID): Promise<ContentVersion[]>;
  saveVersion(assetId: ID, input: SaveVersionInput): Promise<ContentVersion>;
  restoreVersion(assetId: ID, versionNo: number): Promise<ContentVersion>;
  /** POST /assets/{id}/submit-review (asset module, ACS-080). */
  submitReview(assetId: ID): Promise<void>;
  listBrands(): Promise<BrandOption[]>;
  listProducts(brandId: ID): Promise<ProductOption[]>;
  getCampaign(campaignId: ID): Promise<CampaignSummary>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    content: ContentRepository;
  }
}
