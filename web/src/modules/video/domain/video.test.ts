import {
  blankScene,
  isDirty,
  jobProgressLabel,
  moveItem,
  produceBlockers,
  stepLabel,
  toDrafts,
  validateStoryboard,
  type Scene,
  type SceneDraft,
  type VideoEstimate,
} from "./video";

const draft = (durationMs: number, visualPrompt = "Cảnh"): SceneDraft => ({ ...blankScene(), durationMs, visualPrompt });

const scene = (n: number, durationMs = 4000): Scene => ({
  id: `s${n}`,
  sequenceNo: n,
  durationMs,
  visualPrompt: `Cảnh ${n}`,
  narration: null,
  subtitleText: null,
  sourceFileId: null,
  clipFileId: null,
  status: "draft",
  attemptCount: 0,
  lastError: null,
  previewUrl: null,
  thumbnailUrl: null,
});

const estimate = (over: Partial<VideoEstimate> = {}): VideoEstimate => ({
  sceneCount: 5,
  totalClipSeconds: 20,
  breakdown: [],
  total: 3,
  currency: "USD",
  perJobLimit: 8,
  campaignRemaining: null,
  globalRemaining: 900,
  requiresOverride: false,
  blocked: false,
  violations: [],
  ...over,
});

describe("validateStoryboard", () => {
  it("accepts a valid 20s storyboard", () => {
    expect(validateStoryboard([draft(4000), draft(4000), draft(4000), draft(4000), draft(4000)])).toEqual([]);
  });

  it("rejects more than 10 scenes", () => {
    const issues = validateStoryboard(Array.from({ length: 11 }, () => draft(2000)));
    expect(issues.map((i) => i.message)).toContain("Tối đa 10 cảnh (đang có 11). Xóa bớt cảnh.");
  });

  it("rejects total duration above 60s", () => {
    const issues = validateStoryboard(Array.from({ length: 8 }, () => draft(8000)));
    expect(issues[0]?.message).toMatch(/Tổng thời lượng 64 giây vượt giới hạn 60 giây/);
  });

  it("rejects scenes outside 2–8s and empty prompts", () => {
    const issues = validateStoryboard([draft(1500), draft(9000), draft(4000, " ")]);
    expect(issues.map((i) => i.field)).toEqual(["scenes[0].duration_ms", "scenes[1].duration_ms", "scenes[2].visual_prompt"]);
  });

  it("rejects an empty storyboard", () => {
    expect(validateStoryboard([])).toHaveLength(1);
  });
});

describe("helpers", () => {
  it("moveItem reorders", () => {
    expect(moveItem(["a", "b", "c"], 0, 2)).toEqual(["b", "c", "a"]);
    expect(moveItem(["a", "b"], 0, 5)).toEqual(["a", "b"]);
  });

  it("isDirty detects reorder and edits", () => {
    const saved = [scene(1), scene(2)];
    const drafts = toDrafts(saved);
    expect(isDirty(drafts, saved)).toBe(false);
    expect(isDirty(moveItem(drafts, 0, 1), saved)).toBe(true);
    expect(isDirty([{ ...drafts[0]!, narration: "Mới" }, drafts[1]!], saved)).toBe(true);
  });

  it("labels steps and progress", () => {
    expect(stepLabel("scene_generation:3")).toBe("Sinh cảnh 3");
    expect(stepLabel("video_render")).toBe("Render video");
    const label = jobProgressLabel({
      id: "j",
      jobType: "video_production",
      status: "running",
      progress: 30,
      errorCode: null,
      errorMessage: null,
      steps: [
        { stepKey: "scene_generation:1", status: "succeeded", attemptCount: 1, errorKind: null, errorMessage: null },
        { stepKey: "scene_generation:2", status: "waiting_provider", attemptCount: 1, errorKind: null, errorMessage: null },
        { stepKey: "video_render", status: "pending", attemptCount: 0, errorKind: null, errorMessage: null },
      ],
    });
    expect(label).toBe("Đang sinh cảnh 2/2");
  });
});

describe("produceBlockers", () => {
  const base = { issues: [], dirty: false, estimate: estimate(), canOverride: false, override: false, jobActive: false };

  it("is empty when producible", () => {
    expect(produceBlockers(base)).toEqual([]);
  });

  it("blocks over-limit storyboard and unsaved changes", () => {
    const out = produceBlockers({ ...base, issues: [{ field: "scenes", message: "x" }], dirty: true });
    expect(out).toHaveLength(2);
  });

  it("blocks creators over per-job limit; lead must tick override", () => {
    const over = estimate({ requiresOverride: true, total: 9 });
    expect(produceBlockers({ ...base, estimate: over })[0]).toMatch(/Cần Marketing Lead duyệt/);
    expect(produceBlockers({ ...base, estimate: over, canOverride: true })[0]).toMatch(/Lead duyệt/);
    expect(produceBlockers({ ...base, estimate: over, canOverride: true, override: true })).toEqual([]);
  });
});
