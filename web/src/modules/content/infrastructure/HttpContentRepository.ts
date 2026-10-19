import { http } from "@/shared/api/http";
import type { ID } from "@/shared/kernel";
import type { ContentRepository } from "../application/ports";
import type {
  ContentFormat,
  GenerateInput,
  RewriteInput,
  SaveVersionInput,
} from "../domain/content";
import {
  fromAsset,
  fromJobAccepted,
  fromVersion,
  toContentJson,
  toGenerateDto,
  toRewriteDto,
  type AssetDto,
  type BrandDto,
  type CampaignDto,
  type JobAcceptedDto,
  type ProductDto,
  type VersionDto,
} from "./dto";

const enc = encodeURIComponent;

export class HttpContentRepository implements ContentRepository {
  async generate(input: GenerateInput, idempotencyKey: string) {
    return fromJobAccepted(
      await http.post<JobAcceptedDto>(
        "/content/generations",
        toGenerateDto(input),
        { idempotencyKey },
      ),
    );
  }

  async rewrite(assetId: ID, input: RewriteInput, idempotencyKey: string) {
    return fromJobAccepted(
      await http.post<JobAcceptedDto>(
        `/content/${enc(assetId)}/rewrites`,
        toRewriteDto(input),
        { idempotencyKey },
      ),
    );
  }

  async transform(assetId: ID, target: ContentFormat, idempotencyKey: string) {
    return fromJobAccepted(
      await http.post<JobAcceptedDto>(
        `/content/${enc(assetId)}/transforms`,
        { target_type: target },
        { idempotencyKey },
      ),
    );
  }

  async submitReview(assetId: ID) {
    await http.post(`/assets/${enc(assetId)}/submit-review`, {});
  }

  async getAsset(assetId: ID) {
    return fromAsset(await http.get<AssetDto>(`/assets/${enc(assetId)}`));
  }

  async listVersions(assetId: ID) {
    const res = await http.page<VersionDto>(
      `/assets/${enc(assetId)}/versions`,
      { query: { limit: 50 } },
    );
    return res.items.map(fromVersion).sort((a, b) => b.versionNo - a.versionNo);
  }

  async saveVersion(assetId: ID, input: SaveVersionInput) {
    const v = await http.post<VersionDto>(`/assets/${enc(assetId)}/versions`, {
      content_json: toContentJson(input.content),
      change_note: input.changeNote,
      base_version_no: input.baseVersionNo,
    });
    return fromVersion(v);
  }

  async restoreVersion(assetId: ID, versionNo: number) {
    return fromVersion(
      await http.post<VersionDto>(
        `/assets/${enc(assetId)}/versions/${versionNo}/restore`,
      ),
    );
  }

  async listBrands() {
    const res = await http.page<BrandDto>("/brands", { query: { limit: 100 } });
    return res.items.map((b) => ({ id: b.id, name: b.name }));
  }

  async listProducts(brandId: ID) {
    const res = await http.page<ProductDto>(
      `/brands/${enc(brandId)}/products`,
      { query: { limit: 100 } },
    );
    return res.items.map((p) => ({ id: p.id, name: p.name }));
  }

  async getCampaign(campaignId: ID) {
    const c = await http.get<CampaignDto>(`/campaigns/${enc(campaignId)}`);
    return {
      id: c.id,
      name: c.name,
      brandId: c.brand_id,
      goal: c.goal ?? null,
      status: c.status,
    };
  }
}
