import type { ID, Page } from "@/shared/kernel";
import type {
  AssetDetail,
  AssetFilters,
  AssetSummary,
  AssetUpdate,
  AssetVersion,
  CampaignOption,
  DownloadLink,
  ReviewQueueFilters,
  SubmitReviewInput,
} from "../domain/asset";

export interface LibraryRepository {
  list(filters: AssetFilters, cursor: string | null): Promise<Page<AssetSummary>>;
  get(id: ID): Promise<AssetDetail>;
  listVersions(id: ID): Promise<AssetVersion[]>;
  update(id: ID, patch: AssetUpdate): Promise<AssetDetail>;
  restoreVersion(id: ID, versionNo: number): Promise<AssetVersion>;
  submitReview(id: ID, input: SubmitReviewInput): Promise<AssetDetail>;
  downloadUrl(id: ID, fileId?: ID): Promise<DownloadLink>;
  remove(id: ID): Promise<void>;
  campaignOptions(): Promise<CampaignOption[]>;
  // Review (ACS-080/081): the asset API client also serves the review module.
  reviewQueue(filters: ReviewQueueFilters, cursor: string | null): Promise<Page<AssetSummary>>;
  approve(id: ID, comment: string): Promise<AssetDetail>;
  requestChanges(id: ID, comment: string): Promise<AssetDetail>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    library: LibraryRepository;
  }
}
