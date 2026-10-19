import type { AssetStatus, ID, ISODateTime, JobStatus, JobStepStatus, SceneStatus } from "@/shared/kernel";

/** OQ-07 limits; the server echoes them in `limits` and is the authority. */
export interface StoryboardLimits {
  maxTotalSeconds: number;
  maxScenes: number;
  minSceneMs: number;
  maxSceneMs: number;
}

export const DEFAULT_LIMITS: StoryboardLimits = { maxTotalSeconds: 60, maxScenes: 10, minSceneMs: 2000, maxSceneMs: 8000 };

export type VideoPreset = "social_vertical" | "square" | "landscape";
export type AspectRatio = "9:16" | "1:1" | "16:9";

export const PRESETS: Record<VideoPreset, { label: string; aspectRatio: AspectRatio; width: number; height: number }> = {
  social_vertical: { label: "Dọc 9:16 (1080×1920)", aspectRatio: "9:16", width: 1080, height: 1920 },
  square: { label: "Vuông 1:1 (1080×1080)", aspectRatio: "1:1", width: 1080, height: 1080 },
  landscape: { label: "Ngang 16:9 (1920×1080)", aspectRatio: "16:9", width: 1920, height: 1080 },
};

export const MIN_TARGET_SECONDS = 4;

export type SourceType = "prompt" | "image" | "script";
export type VoiceMode = "per_scene" | "whole";
export type SubtitleStyle = "burn_in" | "soft";
export type VideoStage = "storyboarding" | "editing" | "producing" | "ready" | "failed";
export type VideoJobType = "video_storyboard" | "video_production" | "video_scene_regenerate" | "video_render";

export interface VoiceoverSettings {
  enabled: boolean;
  language: string;
  voiceId: string | null;
  mode: VoiceMode | null;
}
export interface SubtitleSettings {
  enabled: boolean;
  language: string;
  style: SubtitleStyle;
}
export interface MusicSettings {
  enabled: boolean;
  trackBrandAssetId: ID | null;
}

export interface CreateVideoInput {
  campaignId: ID | null;
  brandId: ID | null;
  title: string | null;
  source: { type: SourceType; fileIds: ID[] };
  prompt: string | null;
  scriptFromAssetId: ID | null;
  preset: VideoPreset;
  targetDurationSeconds: number;
  voiceover: VoiceoverSettings;
  subtitles: SubtitleSettings;
  music: MusicSettings;
}

export interface VideoCreated {
  assetId: ID;
  jobId: ID;
  status: JobStatus;
  estimatedCost: number;
}

export interface Scene {
  id: ID;
  sequenceNo: number;
  durationMs: number;
  visualPrompt: string;
  narration: string | null;
  subtitleText: string | null;
  sourceFileId: ID | null;
  clipFileId: ID | null;
  status: SceneStatus;
  attemptCount: number;
  lastError: string | null;
  previewUrl: string | null;
  thumbnailUrl: string | null;
}

export interface JobStep {
  stepKey: string;
  status: JobStepStatus;
  attemptCount: number;
  errorKind: string | null;
  errorMessage: string | null;
}

export interface VideoJob {
  id: ID;
  jobType: VideoJobType;
  status: JobStatus;
  progress: number;
  steps: JobStep[];
  errorCode: string | null;
  errorMessage: string | null;
}

export interface VideoOutput {
  fileId: ID;
  url: string;
  thumbnailUrl: string | null;
  durationMs: number;
  versionNo: number;
  subtitleFileId: ID | null;
  subtitleUrl: string | null;
}

export interface VideoSettings {
  preset: VideoPreset;
  aspectRatio: AspectRatio;
  width: number;
  height: number;
  fps: number;
  targetDurationSeconds: number;
  resolution: string;
  voiceover: VoiceoverSettings;
  subtitles: SubtitleSettings;
  music: MusicSettings;
  source: { type: SourceType; fileIds: ID[] };
}

export interface VideoProject {
  id: ID;
  title: string;
  status: AssetStatus;
  stage: VideoStage;
  campaignId: ID | null;
  brandId: ID;
  createdBy: ID;
  prompt: string | null;
  script: string | null;
  settings: VideoSettings;
  scenes: Scene[];
  totalDurationMs: number;
  limits: StoryboardLimits;
  output: VideoOutput | null;
  currentJob: VideoJob | null;
  versionNo: number;
  createdAt: ISODateTime;
  updatedAt: ISODateTime;
}

/** Editable scene in the storyboard editor. `key` is stable for drag & drop; `id` null = new scene. */
export interface SceneDraft {
  key: string;
  id: ID | null;
  durationMs: number;
  visualPrompt: string;
  narration: string;
  subtitleText: string;
  sourceFileId: ID | null;
}

export interface SceneInput {
  id: ID | null;
  durationMs: number;
  visualPrompt: string;
  narration: string | null;
  subtitleText: string | null;
  sourceFileId: ID | null;
}

export interface EstimateLine {
  item: string;
  provider: string | null;
  model: string | null;
  units: number;
  unit: string;
  amount: number;
}

export interface VideoEstimate {
  sceneCount: number;
  totalClipSeconds: number;
  breakdown: EstimateLine[];
  total: number;
  currency: string;
  perJobLimit: number;
  campaignRemaining: number | null;
  globalRemaining: number;
  requiresOverride: boolean;
  blocked: boolean;
  violations: { field: string; issue: string }[];
}

export interface VideoListQuery {
  campaignId?: ID | null;
  cursor?: string | null;
}

export interface MusicTrack {
  id: ID;
  label: string;
  licensed: boolean;
}

export interface BrandOption {
  id: ID;
  name: string;
}

export interface StoryboardIssue {
  field: string;
  message: string;
}

export function totalDurationMs(scenes: readonly { durationMs: number }[]): number {
  return scenes.reduce((sum, s) => sum + s.durationMs, 0);
}

/** Client-side mirror of the server rules (OQ-07). Empty list = producible. */
export function validateStoryboard(scenes: readonly SceneDraft[], limits: StoryboardLimits = DEFAULT_LIMITS): StoryboardIssue[] {
  const issues: StoryboardIssue[] = [];
  if (scenes.length === 0) issues.push({ field: "scenes", message: "Storyboard cần ít nhất 1 cảnh." });
  if (scenes.length > limits.maxScenes) {
    issues.push({ field: "scenes", message: `Tối đa ${limits.maxScenes} cảnh (đang có ${scenes.length}). Xóa bớt cảnh.` });
  }
  const total = totalDurationMs(scenes);
  if (total > limits.maxTotalSeconds * 1000) {
    issues.push({
      field: "scenes",
      message: `Tổng thời lượng ${formatSeconds(total)} vượt giới hạn ${limits.maxTotalSeconds} giây. Giảm số cảnh hoặc thời lượng.`,
    });
  }
  scenes.forEach((s, i) => {
    if (s.durationMs < limits.minSceneMs || s.durationMs > limits.maxSceneMs) {
      issues.push({
        field: `scenes[${i}].duration_ms`,
        message: `Cảnh ${i + 1}: thời lượng phải từ ${limits.minSceneMs / 1000} đến ${limits.maxSceneMs / 1000} giây.`,
      });
    }
    if (!s.visualPrompt.trim()) {
      issues.push({ field: `scenes[${i}].visual_prompt`, message: `Cảnh ${i + 1}: cần mô tả hình ảnh.` });
    }
  });
  return issues;
}

function formatSeconds(ms: number): string {
  const s = ms / 1000;
  return `${Number.isInteger(s) ? s : s.toFixed(1)} giây`;
}

let draftSeq = 0;
export function newDraftKey(): string {
  draftSeq += 1;
  return `draft-${draftSeq}`;
}

export function toDrafts(scenes: readonly Scene[]): SceneDraft[] {
  return [...scenes]
    .sort((a, b) => a.sequenceNo - b.sequenceNo)
    .map((s) => ({
      key: s.id,
      id: s.id,
      durationMs: s.durationMs,
      visualPrompt: s.visualPrompt,
      narration: s.narration ?? "",
      subtitleText: s.subtitleText ?? "",
      sourceFileId: s.sourceFileId,
    }));
}

export function toSceneInputs(drafts: readonly SceneDraft[]): SceneInput[] {
  return drafts.map((d) => ({
    id: d.id,
    durationMs: d.durationMs,
    visualPrompt: d.visualPrompt.trim(),
    narration: d.narration.trim() || null,
    subtitleText: d.subtitleText.trim() || null,
    sourceFileId: d.sourceFileId,
  }));
}

export function blankScene(): SceneDraft {
  return { key: newDraftKey(), id: null, durationMs: 4000, visualPrompt: "", narration: "", subtitleText: "", sourceFileId: null };
}

/** Moves an item (drag & drop / keyboard reorder). */
export function moveItem<T>(list: readonly T[], from: number, to: number): T[] {
  if (from === to || from < 0 || to < 0 || from >= list.length || to >= list.length) return [...list];
  const out = [...list];
  const [item] = out.splice(from, 1);
  out.splice(to, 0, item as T);
  return out;
}

/** True when the drafts differ from the saved scenes (order, content, additions, removals). */
export function isDirty(drafts: readonly SceneDraft[], saved: readonly Scene[]): boolean {
  const a = toSceneInputs(drafts);
  const b = toSceneInputs(toDrafts(saved));
  return JSON.stringify(a) !== JSON.stringify(b);
}

export function isJobActive(job: VideoJob | null | undefined): boolean {
  return !!job && (job.status === "queued" || job.status === "running");
}

/** Vietnamese label for a DAG step key (SPEC-02 §5.5). */
export function stepLabel(stepKey: string): string {
  const [type, n] = stepKey.split(":");
  switch (type) {
    case "script_generation":
      return "Viết kịch bản";
    case "storyboard_generation":
      return "Tạo storyboard";
    case "scene_generation":
      return `Sinh cảnh ${n ?? ""}`.trim();
    case "voiceover_generation":
      return "Tạo giọng đọc";
    case "subtitle_generation":
      return "Tạo phụ đề";
    case "video_render":
      return "Render video";
    case "quality_check":
      return "Kiểm tra chất lượng";
    case "asset_publish_ready":
      return "Lưu phiên bản";
    default:
      return stepKey;
  }
}

/** Concrete progress line, e.g. "Đang sinh cảnh 3/5". */
export function jobProgressLabel(job: VideoJob): string {
  if (job.status === "queued") return "Đang chờ xử lý";
  const scenes = job.steps.filter((s) => s.stepKey.startsWith("scene_generation"));
  const active = job.steps.find((s) => s.status === "running" || s.status === "waiting_provider" || s.status === "queued");
  if (active?.stepKey.startsWith("scene_generation") && scenes.length > 0) {
    const done = scenes.filter((s) => s.status === "succeeded").length;
    return `Đang sinh cảnh ${Math.min(done + 1, scenes.length)}/${scenes.length}`;
  }
  if (active) return `Đang ${stepLabel(active.stepKey).toLowerCase()}`;
  if (job.status === "succeeded") return "Hoàn thành";
  return "Đang xử lý";
}

export function jobTypeLabel(t: VideoJobType): string {
  switch (t) {
    case "video_storyboard":
      return "Tạo storyboard";
    case "video_production":
      return "Sản xuất video";
    case "video_scene_regenerate":
      return "Tạo lại cảnh";
    case "video_render":
      return "Render lại";
  }
}

export function canSubmitForReview(p: VideoProject): boolean {
  return p.stage === "ready" && !!p.output && (p.status === "draft" || p.status === "changes_requested") && !isJobActive(p.currentJob);
}

/** Reasons the "Sản xuất video" button is disabled; empty = enabled. */
export function produceBlockers(args: {
  issues: readonly StoryboardIssue[];
  dirty: boolean;
  estimate: VideoEstimate | null | undefined;
  canOverride: boolean;
  override: boolean;
  jobActive: boolean;
}): string[] {
  const out: string[] = [];
  if (args.jobActive) out.push("Đang có job chạy cho video này.");
  if (args.issues.length > 0) out.push("Storyboard chưa đạt giới hạn: sửa các lỗi bên trên.");
  if (args.dirty) out.push("Lưu storyboard trước khi sản xuất.");
  if (!args.estimate) out.push("Chưa có dự toán.");
  else {
    if (args.estimate.blocked) out.push("Ngân sách còn lại không đủ cho dự toán này.");
    if (args.estimate.requiresOverride && !(args.canOverride && args.override)) {
      out.push(
        args.canOverride
          ? "Chọn “Vượt hạn mức (Lead duyệt)” để tiếp tục."
          : "Cần Marketing Lead duyệt vượt hạn mức.",
      );
    }
  }
  return out;
}

/** Timeline slot of each scene on the rendered video (cross-fade 300 ms between scenes, as the renderer does). */
export const TRANSITION_MS = 300;

export interface TimelineSlot {
  startMs: number;
  endMs: number;
}

export function timeline(durationsMs: readonly number[], transitionMs = TRANSITION_MS): { slots: TimelineSlot[]; totalMs: number } {
  const t = durationsMs.length > 1 ? transitionMs : 0;
  const slots: TimelineSlot[] = [];
  let start = 0;
  for (const d of durationsMs) {
    slots.push({ startMs: start, endMs: start + d });
    start += d - t;
  }
  return { slots, totalMs: slots.length ? slots[slots.length - 1]!.endMs : 0 };
}

export function stageLabel(stage: VideoStage): string {
  switch (stage) {
    case "storyboarding":
      return "Đang tạo storyboard";
    case "editing":
      return "Sửa storyboard";
    case "producing":
      return "Đang sản xuất";
    case "ready":
      return "Đã có video";
    case "failed":
      return "Sản xuất lỗi";
  }
}

/** Scenes whose generation failed in the current job (partial failure). */
export function failedScenes(p: VideoProject): Scene[] {
  return p.scenes.filter((s) => s.status === "failed");
}

/** Re-render is possible when every scene has a clip and no job runs. */
export function canRerender(p: VideoProject): boolean {
  return p.scenes.length > 0 && p.scenes.every((s) => s.status === "ready") && !isJobActive(p.currentJob);
}
