import type { AssetStatus, JobStatus, JobStepStatus, SceneStatus } from "@/shared/kernel";
import type {
  AspectRatio,
  BrandOption,
  CreateVideoInput,
  MusicSettings,
  MusicTrack,
  SceneInput,
  SourceType,
  SubtitleSettings,
  SubtitleStyle,
  VideoCreated,
  VideoEstimate,
  VideoJobType,
  VideoPreset,
  VideoProject,
  VideoStage,
  VoiceMode,
  VoiceoverSettings,
} from "../domain/video";

/** Snake_case wire types — SPEC-04 §2.6 + Agent 9 contract. */
export interface VoiceoverDto {
  enabled: boolean;
  language: string;
  voice_id: string | null;
  mode: VoiceMode | null;
}
export interface SubtitlesDto {
  enabled: boolean;
  language: string;
  style: SubtitleStyle;
}
export interface MusicDto {
  enabled: boolean;
  track_brand_asset_id: string | null;
}

export interface CreateVideoDto {
  campaign_id: string | null;
  brand_id: string | null;
  title: string | null;
  source: { type: SourceType; file_ids: string[] };
  prompt: string | null;
  script_from_asset_id: string | null;
  preset: VideoPreset;
  aspect_ratio: AspectRatio | null;
  target_duration_seconds: number;
  resolution: string | null;
  voiceover: VoiceoverDto;
  subtitles: SubtitlesDto;
  music: MusicDto;
}

export interface MoneyEstimateDto {
  amount: number;
  currency: string;
  is_estimate: boolean;
}

export interface VideoCreatedDto {
  asset_id: string;
  job_id: string;
  status: JobStatus;
  estimated_cost: MoneyEstimateDto;
}

export interface JobAcceptedDto {
  job_id: string;
  status: string;
  estimated_cost: MoneyEstimateDto;
  asset_id?: string;
}

export interface SceneDto {
  id: string;
  sequence_no: number;
  duration_ms: number;
  visual_prompt: string;
  narration: string | null;
  subtitle_text: string | null;
  source_file_id: string | null;
  clip_file_id: string | null;
  status: SceneStatus;
  attempt_count: number;
  last_error: string | null;
  preview_url: string | null;
  thumbnail_url?: string | null;
}

export interface JobStepDto {
  step_key: string;
  step_type?: string;
  status: JobStepStatus;
  attempt_count: number;
  error_kind: string | null;
  error_message: string | null;
}

export interface VideoJobDto {
  id: string;
  job_type: VideoJobType;
  status: JobStatus;
  progress: number;
  steps: JobStepDto[];
  error_code: string | null;
  error_message: string | null;
}

export interface VideoProjectDto {
  id: string;
  title: string;
  status: AssetStatus;
  stage: VideoStage;
  campaign_id: string | null;
  brand_id: string;
  created_by: string;
  prompt: string | null;
  script: string | null;
  settings: {
    preset: VideoPreset;
    aspect_ratio: AspectRatio;
    width: number;
    height: number;
    fps: number;
    target_duration_seconds: number;
    resolution: string;
    voiceover: VoiceoverDto;
    subtitles: SubtitlesDto;
    music: MusicDto;
    source: { type: SourceType; file_ids: string[] };
  };
  scenes: SceneDto[];
  total_duration_ms: number;
  limits: { max_total_seconds: number; max_scenes: number; min_scene_ms: number; max_scene_ms: number };
  output: {
    file_id: string;
    url: string;
    thumbnail_url: string | null;
    duration_ms: number;
    version_no: number;
    subtitle_file_id: string | null;
    subtitle_url?: string | null;
  } | null;
  current_job: VideoJobDto | null;
  version_no: number;
  created_at: string;
  updated_at: string;
}

export interface SceneInputDto {
  id: string | null;
  duration_ms: number;
  visual_prompt: string;
  narration: string | null;
  subtitle_text: string | null;
  source_file_id: string | null;
}

export interface EstimateDto {
  scene_count: number;
  total_clip_seconds: number;
  breakdown: { item: string; provider?: string | null; model?: string | null; units: number; unit: string; amount: number }[];
  total: MoneyEstimateDto;
  limits: { per_job_limit: number; campaign_remaining: number | null; global_remaining: number };
  requires_override: boolean;
  blocked?: boolean;
  violations?: { field: string; issue: string }[];
}

export interface BrandDto {
  id: string;
  name: string;
}

export interface BrandAssetDto {
  id: string;
  asset_type: string;
  label: string | null;
  license?: unknown;
  /** Complete and unexpired license (BrandAsset.license_valid) — the API's MUSIC_NOT_LICENSED rule. */
  license_valid?: boolean;
  /** @deprecated Legacy mock field; the API sends `license` + `license_valid`. */
  license_json?: unknown;
}

export const voiceToDto = (v: VoiceoverSettings): VoiceoverDto => ({
  enabled: v.enabled,
  language: v.language,
  voice_id: v.voiceId,
  mode: v.mode,
});
const voiceFromDto = (v: VoiceoverDto): VoiceoverSettings => ({
  enabled: v.enabled,
  language: v.language,
  voiceId: v.voice_id ?? null,
  mode: v.mode ?? null,
});
export const subsToDto = (s: SubtitleSettings): SubtitlesDto => ({ enabled: s.enabled, language: s.language, style: s.style });
export const musicToDto = (m: MusicSettings): MusicDto => ({ enabled: m.enabled, track_brand_asset_id: m.trackBrandAssetId });
const musicFromDto = (m: MusicDto): MusicSettings => ({ enabled: m.enabled, trackBrandAssetId: m.track_brand_asset_id ?? null });

export function toCreateDto(i: CreateVideoInput, aspectRatio: AspectRatio): CreateVideoDto {
  return {
    campaign_id: i.campaignId,
    brand_id: i.campaignId ? null : i.brandId,
    title: i.title,
    source: { type: i.source.type, file_ids: i.source.fileIds },
    prompt: i.prompt,
    script_from_asset_id: i.scriptFromAssetId,
    preset: i.preset,
    aspect_ratio: aspectRatio,
    target_duration_seconds: i.targetDurationSeconds,
    resolution: "1080p",
    voiceover: voiceToDto(i.voiceover),
    subtitles: subsToDto(i.subtitles),
    music: musicToDto(i.music),
  };
}

export function fromCreatedDto(d: VideoCreatedDto): VideoCreated {
  return { assetId: d.asset_id, jobId: d.job_id, status: d.status, estimatedCost: d.estimated_cost.amount };
}

export function fromProjectDto(d: VideoProjectDto): VideoProject {
  const job = d.current_job;
  return {
    id: d.id,
    title: d.title,
    status: d.status,
    stage: d.stage,
    campaignId: d.campaign_id,
    brandId: d.brand_id,
    createdBy: d.created_by,
    prompt: d.prompt,
    script: d.script,
    settings: {
      preset: d.settings.preset,
      aspectRatio: d.settings.aspect_ratio,
      width: d.settings.width,
      height: d.settings.height,
      fps: d.settings.fps,
      targetDurationSeconds: d.settings.target_duration_seconds,
      resolution: d.settings.resolution,
      voiceover: voiceFromDto(d.settings.voiceover),
      subtitles: { ...d.settings.subtitles },
      music: musicFromDto(d.settings.music),
      source: { type: d.settings.source.type, fileIds: d.settings.source.file_ids ?? [] },
    },
    scenes: (d.scenes ?? []).map((s) => ({
      id: s.id,
      sequenceNo: s.sequence_no,
      durationMs: s.duration_ms,
      visualPrompt: s.visual_prompt,
      narration: s.narration,
      subtitleText: s.subtitle_text,
      sourceFileId: s.source_file_id,
      clipFileId: s.clip_file_id,
      status: s.status,
      attemptCount: s.attempt_count,
      lastError: s.last_error,
      previewUrl: s.preview_url || null,
      thumbnailUrl: s.thumbnail_url || null,
    })),
    totalDurationMs: d.total_duration_ms,
    limits: {
      maxTotalSeconds: d.limits.max_total_seconds,
      maxScenes: d.limits.max_scenes,
      minSceneMs: d.limits.min_scene_ms,
      maxSceneMs: d.limits.max_scene_ms,
    },
    output: d.output
      ? {
          fileId: d.output.file_id,
          url: d.output.url,
          thumbnailUrl: d.output.thumbnail_url,
          durationMs: d.output.duration_ms,
          versionNo: d.output.version_no,
          subtitleFileId: d.output.subtitle_file_id,
          subtitleUrl: d.output.subtitle_url || null,
        }
      : null,
    currentJob: job
      ? {
          id: job.id,
          jobType: job.job_type,
          status: job.status,
          progress: job.progress,
          steps: (job.steps ?? []).map((s) => ({
            stepKey: s.step_key,
            status: s.status,
            attemptCount: s.attempt_count,
            errorKind: s.error_kind ?? null,
            errorMessage: s.error_message ?? null,
          })),
          errorCode: job.error_code,
          errorMessage: job.error_message,
        }
      : null,
    versionNo: d.version_no,
    createdAt: d.created_at,
    updatedAt: d.updated_at,
  };
}

export function toSceneInputDto(s: SceneInput): SceneInputDto {
  return {
    id: s.id,
    duration_ms: s.durationMs,
    visual_prompt: s.visualPrompt,
    narration: s.narration,
    subtitle_text: s.subtitleText,
    source_file_id: s.sourceFileId,
  };
}

export function fromEstimateDto(d: EstimateDto): VideoEstimate {
  return {
    sceneCount: d.scene_count,
    totalClipSeconds: d.total_clip_seconds,
    breakdown: d.breakdown.map((b) => ({
      item: b.item,
      provider: b.provider ?? null,
      model: b.model ?? null,
      units: b.units,
      unit: b.unit,
      amount: b.amount,
    })),
    total: d.total.amount,
    currency: d.total.currency,
    perJobLimit: d.limits.per_job_limit,
    campaignRemaining: d.limits.campaign_remaining,
    globalRemaining: d.limits.global_remaining,
    requiresOverride: d.requires_override,
    blocked: d.blocked ?? false,
    violations: d.violations ?? [],
  };
}

export function fromBrandDto(d: BrandDto): BrandOption {
  return { id: d.id, name: d.name };
}

export function fromBrandAssetDto(d: BrandAssetDto): MusicTrack {
  const license = d.license ?? d.license_json;
  const licensed =
    d.license_valid ?? (!!license && typeof license === "object" && Object.keys(license as object).length > 0);
  return { id: d.id, label: d.label || "Nhạc chưa đặt tên", licensed };
}
