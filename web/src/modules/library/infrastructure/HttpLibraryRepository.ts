import { http } from "@/shared/api/http";
import type { Page } from "@/shared/kernel";
import type { LibraryRepository } from "../application/ports";
import {
  filtersToParams,
  type AssetDetail,
  type AssetFilters,
  type AssetSummary,
  type AssetUpdate,
  type AssetVersion,
  type CampaignOption,
  type DownloadLink,
  type ReviewQueueFilters,
  type SubmitReviewInput,
} from "../domain/asset";
import {
  toDetail,
  toDownload,
  toSummary,
  toVersion,
  type AssetDetailDto,
  type AssetDownloadUrlDto,
  type AssetSummaryDto,
  type AssetVersionDto,
  type CampaignOptionDto,
} from "./dto";

const PAGE_SIZE = 24;

export class HttpLibraryRepository implements LibraryRepository {
  async list(filters: AssetFilters, cursor: string | null): Promise<Page<AssetSummary>> {
    const query: Record<string, string | number> = { limit: PAGE_SIZE };
    filtersToParams(filters).forEach((v, k) => (query[k] = v));
    if (cursor) query.cursor = cursor;
    const page = await http.page<AssetSummaryDto>("/assets", { query });
    return { items: page.items.map(toSummary), nextCursor: page.nextCursor };
  }

  async get(id: string): Promise<AssetDetail> {
    return toDetail(await http.get<AssetDetailDto>(`/assets/${id}`));
  }

  async listVersions(id: string): Promise<AssetVersion[]> {
    const page = await http.page<AssetVersionDto>(`/assets/${id}/versions`);
    return page.items.map(toVersion);
  }

  async update(id: string, patch: AssetUpdate): Promise<AssetDetail> {
    const body: Record<string, unknown> = {};
    if (patch.title !== undefined) body.title = patch.title;
    if (patch.tags !== undefined) body.tags = patch.tags;
    if (patch.usageRights !== undefined) body.usage_rights = patch.usageRights;
    return toDetail(await http.patch<AssetDetailDto>(`/assets/${id}`, body));
  }

  async restoreVersion(id: string, versionNo: number): Promise<AssetVersion> {
    return toVersion(await http.post<AssetVersionDto>(`/assets/${id}/versions/${versionNo}/restore`));
  }

  async submitReview(id: string, input: SubmitReviewInput): Promise<AssetDetail> {
    const body: Record<string, unknown> = {};
    if (input.comment?.trim()) body.comment = input.comment.trim();
    if (input.typographyChecked !== undefined) body.checklist = { typography_checked: input.typographyChecked };
    return toDetail(await http.post<AssetDetailDto>(`/assets/${id}/submit-review`, body));
  }

  async downloadUrl(id: string, fileId?: string): Promise<DownloadLink> {
    return toDownload(await http.post<AssetDownloadUrlDto>(`/assets/${id}/download-url`, fileId ? { file_id: fileId } : {}));
  }

  async remove(id: string): Promise<void> {
    await http.delete(`/assets/${id}`);
  }

  /** Filter options; the campaign module owns /campaigns (only id + name are read here). */
  async campaignOptions(): Promise<CampaignOption[]> {
    const page = await http.page<CampaignOptionDto>("/campaigns", { query: { limit: 100 } });
    return page.items.map((c) => ({ id: c.id, name: c.name }));
  }

  async reviewQueue(filters: ReviewQueueFilters, cursor: string | null): Promise<Page<AssetSummary>> {
    const query: Record<string, string | number> = { limit: PAGE_SIZE };
    if (filters.type) query.type = filters.type;
    if (filters.campaignId) query.campaign_id = filters.campaignId;
    if (cursor) query.cursor = cursor;
    const page = await http.page<AssetSummaryDto>("/reviews/queue", { query });
    return { items: page.items.map(toSummary), nextCursor: page.nextCursor };
  }

  async approve(id: string, comment: string): Promise<AssetDetail> {
    const body = comment.trim() ? { comment: comment.trim() } : {};
    return toDetail(await http.post<AssetDetailDto>(`/assets/${id}/approve`, body));
  }

  async requestChanges(id: string, comment: string): Promise<AssetDetail> {
    return toDetail(await http.post<AssetDetailDto>(`/assets/${id}/request-changes`, { comment: comment.trim() }));
  }
}
