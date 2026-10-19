import type { ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { AppProviders, createQueryClient } from "@/app/providers";
import { fromProjectDto, type SceneDto, type VideoProjectDto } from "../infrastructure/dto";
import { useStoryboardDrafts } from "./useStoryboardDrafts";

const scene = (n: number): SceneDto => ({
  id: `5ce00000-0000-4000-8000-00000000000${n}`,
  sequence_no: n,
  duration_ms: 5000,
  visual_prompt: `Cảnh ${n}`,
  narration: `Lời thoại ${n}`,
  subtitle_text: `Phụ đề ${n}`,
  source_file_id: null,
  clip_file_id: null,
  status: "draft",
  attempt_count: 0,
  last_error: null,
  preview_url: null,
  thumbnail_url: null,
});

const dto = (scenes: SceneDto[]): VideoProjectDto => ({
  id: "a1d00000-0000-4000-8000-000000000001",
  title: "Video",
  status: "draft",
  stage: scenes.length ? "editing" : "storyboarding",
  campaign_id: null,
  brand_id: "10000000-0000-4000-8000-000000000001",
  created_by: "00000000-0000-4000-8000-000000000003",
  prompt: "Video",
  script: null,
  settings: {
    preset: "social_vertical",
    aspect_ratio: "9:16",
    width: 1080,
    height: 1920,
    fps: 30,
    target_duration_seconds: 20,
    resolution: "1080p",
    voiceover: { enabled: true, language: "vi", voice_id: null, mode: "per_scene" },
    subtitles: { enabled: true, language: "vi", style: "burn_in" },
    music: { enabled: false, track_brand_asset_id: null },
    source: { type: "prompt", file_ids: [] },
  },
  scenes,
  total_duration_ms: scenes.length * 5000,
  limits: { max_total_seconds: 60, max_scenes: 10, min_scene_ms: 2000, max_scene_ms: 8000 },
  output: null,
  current_job: null,
  version_no: 1,
  created_at: "2026-10-19T08:00:00Z",
  updated_at: "2026-10-19T08:00:00Z",
});

const wrapper = ({ children }: { children: ReactNode }) => <AppProviders queryClient={createQueryClient()}>{children}</AppProviders>;

describe("useStoryboardDrafts", () => {
  it("adopts the first storyboard when the storyboard job finishes (empty → scenes)", () => {
    const { result, rerender } = renderHook(({ p }) => useStoryboardDrafts(p), { wrapper, initialProps: { p: fromProjectDto(dto([])) } });
    expect(result.current.drafts).toHaveLength(0);
    rerender({ p: fromProjectDto(dto([scene(1), scene(2)])) });
    expect(result.current.drafts.map((d) => d.visualPrompt)).toEqual(["Cảnh 1", "Cảnh 2"]);
    expect(result.current.dirty).toBe(false);
  });

  it("keeps the user's edits when the server scenes change", () => {
    const { result, rerender } = renderHook(({ p }) => useStoryboardDrafts(p), { wrapper, initialProps: { p: fromProjectDto(dto([scene(1)])) } });
    act(() => result.current.update(result.current.drafts[0]!.key, { visualPrompt: "Đã sửa" }));
    rerender({ p: fromProjectDto(dto([scene(1), scene(2)])) });
    expect(result.current.drafts.map((d) => d.visualPrompt)).toEqual(["Đã sửa"]);
    expect(result.current.dirty).toBe(true);
  });
});
