import { mockSession } from "@/shared/api/mock";
import { HttpVideoRepository } from "./HttpVideoRepository";
import { resetVideoMocks } from "./mocks";

afterEach(() => resetVideoMocks());

describe("HttpVideoRepository ↔ MSW contract", () => {
  const repo = new HttpVideoRepository();

  it("creates, lists, edits, estimates and produces a video", async () => {
    mockSession.signInAs("creator@acs.local");
    const created = await repo.create(
      {
        campaignId: null,
        brandId: "b0000000-0000-4000-8000-000000000001",
        title: null,
        source: { type: "prompt", fileIds: [] },
        prompt: "Video giới thiệu khóa học",
        scriptFromAssetId: null,
        preset: "square",
        targetDurationSeconds: 12,
        voiceover: { enabled: true, language: "vi", voiceId: null, mode: "per_scene" },
        subtitles: { enabled: true, language: "vi", style: "soft" },
        music: { enabled: false, trackBrandAssetId: null },
      },
      crypto.randomUUID(),
    );
    expect(created.status).toBe("queued");

    let p = await repo.get(created.assetId); // tick 1: script done
    p = await repo.get(created.assetId); // tick 2: storyboard ready
    expect(p.stage).toBe("editing");
    expect(p.settings.aspectRatio).toBe("1:1");
    expect(p.scenes.length).toBeGreaterThan(0);
    expect(p.scenes[0]!.thumbnailUrl).toBeNull();

    const list = await repo.list({});
    expect(list.items.map((v) => v.id)).toContain(created.assetId);

    const saved = await repo.saveScenes(created.assetId, [
      { id: p.scenes[0]!.id, durationMs: 4000, visualPrompt: "Cảnh mở đầu", narration: "Xin chào", subtitleText: null, sourceFileId: null },
      { id: null, durationMs: 3000, visualPrompt: "Logo", narration: null, subtitleText: "Đăng ký ngay", sourceFileId: null },
    ]);
    expect(saved.scenes.map((s) => s.visualPrompt)).toEqual(["Cảnh mở đầu", "Logo"]);

    const est = await repo.estimate(created.assetId);
    expect(est.total).toBeGreaterThan(0);
    expect(est.requiresOverride).toBe(false);
    expect(est.violations).toEqual([]);

    const job = await repo.produce(created.assetId, false, crypto.randomUUID());
    expect(job.status).toBe("queued");
    await expect(repo.produce(created.assetId, false, crypto.randomUUID())).rejects.toMatchObject({ code: "VIDEO_JOB_IN_PROGRESS", status: 409 });
  });

  it("refuses unlicensed music with 422 MUSIC_NOT_LICENSED", async () => {
    mockSession.signInAs("creator@acs.local");
    await expect(
      repo.create(
        {
          campaignId: null,
          brandId: "b0000000-0000-4000-8000-000000000001",
          title: null,
          source: { type: "prompt", fileIds: [] },
          prompt: "x",
          scriptFromAssetId: null,
          preset: "landscape",
          targetDurationSeconds: 10,
          voiceover: { enabled: false, language: "vi", voiceId: null, mode: null },
          subtitles: { enabled: false, language: "vi", style: "burn_in" },
          music: { enabled: true, trackBrandAssetId: "ba000000-0000-4000-8000-000000000002" },
        },
        crypto.randomUUID(),
      ),
    ).rejects.toMatchObject({ code: "MUSIC_NOT_LICENSED", status: 422 });
  });

  it("returns 404 for another user's project", async () => {
    mockSession.signInAs("creator@acs.local");
    await expect(repo.get("a1d00000-0000-4000-8000-00000000ffff")).rejects.toMatchObject({ code: "NOT_FOUND" });
  });
});
