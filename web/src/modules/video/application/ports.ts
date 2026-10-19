import type { ID, Page } from "@/shared/kernel";
import type {
  BrandOption,
  CreateVideoInput,
  MusicSettings,
  MusicTrack,
  SceneInput,
  SubtitleSettings,
  VideoCreated,
  VideoEstimate,
  VideoListQuery,
  VideoProject,
  VoiceoverSettings,
} from "../domain/video";

export interface JobAccepted {
  jobId: ID;
  status: string;
  estimatedCost: number;
}

export interface RenderOptions {
  subtitles?: SubtitleSettings | null;
  music?: MusicSettings | null;
  voiceover?: VoiceoverSettings | null;
}

export interface VideoRepository {
  create(input: CreateVideoInput, idempotencyKey: string): Promise<VideoCreated>;
  list(q: VideoListQuery): Promise<Page<VideoProject>>;
  get(assetId: ID): Promise<VideoProject>;
  saveScenes(assetId: ID, scenes: SceneInput[]): Promise<VideoProject>;
  estimate(assetId: ID): Promise<VideoEstimate>;
  produce(assetId: ID, budgetOverride: boolean, idempotencyKey: string): Promise<JobAccepted>;
  regenerateScene(assetId: ID, sceneId: ID, visualPrompt: string | null, idempotencyKey: string): Promise<JobAccepted>;
  render(assetId: ID, opts: RenderOptions, idempotencyKey: string): Promise<JobAccepted>;
  submitForReview(assetId: ID): Promise<void>;
  listBrands(): Promise<BrandOption[]>;
  listMusicTracks(brandId: ID): Promise<MusicTrack[]>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    video: VideoRepository;
  }
}
