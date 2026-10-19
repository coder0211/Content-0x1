import { http } from "@/shared/api/http";
import type { ID, Page } from "@/shared/kernel";
import type { JobAccepted, RenderOptions, VideoRepository } from "../application/ports";
import { PRESETS, type CreateVideoInput, type SceneInput, type VideoListQuery, type VideoProject } from "../domain/video";
import {
  fromBrandAssetDto,
  fromBrandDto,
  fromCreatedDto,
  fromEstimateDto,
  fromProjectDto,
  musicToDto,
  subsToDto,
  toCreateDto,
  toSceneInputDto,
  voiceToDto,
  type BrandAssetDto,
  type BrandDto,
  type EstimateDto,
  type JobAcceptedDto,
  type VideoCreatedDto,
  type VideoProjectDto,
} from "./dto";

const accepted = (d: JobAcceptedDto): JobAccepted => ({ jobId: d.job_id, status: d.status, estimatedCost: d.estimated_cost.amount });

export class HttpVideoRepository implements VideoRepository {
  async create(input: CreateVideoInput, idempotencyKey: string) {
    const dto = toCreateDto(input, PRESETS[input.preset].aspectRatio);
    return fromCreatedDto(await http.post<VideoCreatedDto>("/videos", dto, { idempotencyKey }));
  }

  async list(q: VideoListQuery): Promise<Page<VideoProject>> {
    const res = await http.page<VideoProjectDto>("/videos", {
      query: { limit: 20, campaign_id: q.campaignId ?? undefined, cursor: q.cursor ?? undefined },
    });
    return { items: res.items.map(fromProjectDto), nextCursor: res.nextCursor };
  }

  async get(assetId: ID) {
    return fromProjectDto(await http.get<VideoProjectDto>(`/videos/${assetId}`));
  }

  async saveScenes(assetId: ID, scenes: SceneInput[]) {
    return fromProjectDto(await http.put<VideoProjectDto>(`/videos/${assetId}/scenes`, { scenes: scenes.map(toSceneInputDto) }));
  }

  async estimate(assetId: ID) {
    return fromEstimateDto(await http.post<EstimateDto>(`/videos/${assetId}/estimate`, {}));
  }

  async produce(assetId: ID, budgetOverride: boolean, idempotencyKey: string) {
    return accepted(
      await http.post<JobAcceptedDto>(`/videos/${assetId}/productions`, { budget_override: budgetOverride }, { idempotencyKey }),
    );
  }

  async regenerateScene(assetId: ID, sceneId: ID, visualPrompt: string | null, idempotencyKey: string) {
    return accepted(
      await http.post<JobAcceptedDto>(
        `/videos/${assetId}/scenes/${sceneId}/regenerations`,
        { visual_prompt: visualPrompt },
        { idempotencyKey },
      ),
    );
  }

  async render(assetId: ID, opts: RenderOptions, idempotencyKey: string) {
    const body = {
      subtitles: opts.subtitles ? subsToDto(opts.subtitles) : null,
      music: opts.music ? musicToDto(opts.music) : null,
      voiceover: opts.voiceover ? voiceToDto(opts.voiceover) : null,
    };
    return accepted(await http.post<JobAcceptedDto>(`/videos/${assetId}/renders`, body, { idempotencyKey }));
  }

  async submitForReview(assetId: ID) {
    await http.post<unknown>(`/assets/${assetId}/submit-review`, {});
  }

  async listBrands() {
    const page = await http.page<BrandDto>("/brands", { query: { limit: 100 } });
    return page.items.map(fromBrandDto);
  }

  async getCampaignBrandId(campaignId: ID) {
    const c = await http.get<{ brand_id: ID }>(`/campaigns/${encodeURIComponent(campaignId)}`);
    return c.brand_id;
  }

  async listMusicTracks(brandId: ID) {
    const page = await http.page<BrandAssetDto>(`/brands/${brandId}/assets`, { query: { asset_type: "music" } });
    return page.items.filter((a) => a.asset_type === "music").map(fromBrandAssetDto);
  }
}
