import { applyStreamEvent, canCancel, canRetry, failedStep, jobErrorKind, progressLabel, stepLabel, type Job } from "./job";

const base: Job = {
  id: "j1",
  type: "video_production",
  status: "running",
  progress: 10,
  createdBy: "u1",
  createdByName: "",
  campaignId: null,
  assetId: null,
  providerCode: "veo",
  modelCode: "veo-3",
  cost: { estimate: 3.2, actual: null, source: null, currency: "USD" },
  error: null,
  cancelRequested: false,
  needsReconciliation: false,
  createdAt: "2026-10-19T09:00:00Z",
  startedAt: "2026-10-19T09:00:01Z",
  finishedAt: null,
  request: {},
  result: null,
  steps: [
    { id: "s1", key: "script_generation", type: "script_generation", status: "succeeded", attemptCount: 1, errorKind: null, errorMessage: null, startedAt: null, finishedAt: null },
    { id: "s2", key: "scene_generation:1", type: "scene_generation", status: "waiting_provider", attemptCount: 1, errorKind: null, errorMessage: null, startedAt: null, finishedAt: null },
    { id: "s3", key: "video_render", type: "video_render", status: "pending", attemptCount: 0, errorKind: null, errorMessage: null, startedAt: null, finishedAt: null },
  ],
};

describe("job domain", () => {
  it("labels steps with their index", () => {
    expect(stepLabel({ key: "scene_generation:3", type: "scene_generation" })).toBe("Sinh cảnh 3");
    expect(stepLabel({ key: "custom_step", type: "custom_step" })).toBe("custom step");
  });

  it("builds concrete progress text", () => {
    expect(progressLabel(base)).toBe("Đang sinh cảnh 1 (chờ nhà cung cấp) · bước 2/3");
    expect(progressLabel({ ...base, cancelRequested: true })).toBe("Đang hủy sau bước hiện tại");
    expect(progressLabel({ ...base, status: "succeeded" })).toBeUndefined();
  });

  it("decides cancel/retry availability", () => {
    expect(canCancel(base)).toBe(true);
    expect(canCancel({ ...base, cancelRequested: true })).toBe(false);
    expect(canCancel({ ...base, status: "succeeded" })).toBe(false);
    expect(canRetry({ status: "failed" })).toBe(true);
    expect(canRetry({ status: "cancelled" })).toBe(false);
  });

  it("applies SSE events", () => {
    let j = applyStreamEvent(base, { type: "progress", data: { progress: 45 } });
    expect(j.progress).toBe(45);
    j = applyStreamEvent(j, { type: "status", data: { status: "running", progress: 30 } });
    expect(j.progress).toBe(45); // never goes backwards
    j = applyStreamEvent(j, { type: "step", data: { step_key: "scene_generation:1", status: "failed", error_kind: "ContentPolicy" } });
    expect(failedStep(j)?.key).toBe("scene_generation:1");
    j = applyStreamEvent(j, { type: "done", data: { status: "failed", error: { code: "PROVIDER_CONTENT_POLICY", message: "x" } } });
    expect(j.status).toBe("failed");
    expect(jobErrorKind(j)).toBe("ContentPolicy");
    expect(base.status).toBe("running"); // immutable
  });

  it("falls back to the job error code", () => {
    expect(jobErrorKind({ ...base, steps: [], error: { code: "LEASE_EXPIRED", message: "" } })).toBe("LEASE_EXPIRED");
  });

  it("sets progress to 100 on success", () => {
    expect(applyStreamEvent(base, { type: "done", data: { status: "succeeded", result: { asset_ids: ["a"] } } })).toMatchObject({
      progress: 100,
      result: { asset_ids: ["a"] },
    });
  });
});
