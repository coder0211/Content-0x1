import { http } from "@/shared/api/http";
import { mockSession } from "@/shared/api/mock";
import { ApiError } from "@/shared/kernel";
import { defaultOptions } from "../domain/image";
import { HttpImageRepository } from "./HttpImageRepository";
import { MOCK_CAMPAIGN_ID, completeImageMockJobs, resetImageMocks, setImageMockCapabilities } from "./mocks";

const KEY = "11111111-2222-4333-8444-555555555555";

describe("HttpImageRepository ↔ mocks (SPEC-04 §2.5)", () => {
  const repo = new HttpImageRepository();
  beforeEach(() => mockSession.signInAs("creator@acs.local"));
  afterEach(() => resetImageMocks());

  it("generates 4 images into one asset and lists the batch", async () => {
    const res = await repo.generate({ campaignId: MOCK_CAMPAIGN_ID }, { ...defaultOptions("poster"), prompt: "Poster IELTS", count: 4 }, KEY);
    expect(res.status).toBe("queued");
    expect(res.estimatedCost?.amount).toBeCloseTo(0.24);
    completeImageMockJobs();
    const page = await repo.listBatches({ campaignId: MOCK_CAMPAIGN_ID });
    const batch = page.items.find((b) => b.job.id === res.jobId)!;
    expect(batch.job.status).toBe("succeeded");
    expect(batch.asset?.images).toHaveLength(4);
    expect(batch.asset?.requiresTypographyCheck).toBe(true);
  });

  it("maps 422 CAPABILITY_NOT_SUPPORTED with supported values", async () => {
    setImageMockCapabilities({ aspectRatios: ["1:1", "16:9"] });
    const err = await repo
      .generate({ campaignId: MOCK_CAMPAIGN_ID }, { ...defaultOptions("poster"), prompt: "x" }, KEY)
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).code).toBe("CAPABILITY_NOT_SUPPORTED");
    expect((err as ApiError).details[0]).toMatchObject({ field: "aspect_ratio", supported: ["1:1", "16:9"] });
  });

  it("prices an estimate and returns reference candidates", async () => {
    const est = await repo.estimate({ campaignId: MOCK_CAMPAIGN_ID, operation: "generate", aspectRatio: "1:1", quality: "high", count: 2 });
    expect(est.total).toEqual({ amount: 0.34, currency: "USD", isEstimate: true });
    expect(est.requiresOverride).toBe(false);
    const refs = await repo.listReferences({ campaignId: MOCK_CAMPAIGN_ID });
    expect(refs.some((r) => r.source === "brand")).toBe(true);
    expect(refs.some((r) => r.source === "library")).toBe(true);
  });

  it("creates variations with parent_file_id and returns prompt suggestions", async () => {
    const seed = (await repo.listBatches({ campaignId: MOCK_CAMPAIGN_ID })).items[0]!;
    const src = seed.asset!.images[0]!;
    await repo.vary(seed.asset!.id, { fileId: src.fileId, count: 2 }, KEY);
    completeImageMockJobs();
    const page = await repo.listBatches({ assetId: seed.asset!.id });
    const variation = page.items.find((b) => b.operation === "variation")!;
    expect(variation.asset?.images.every((i) => i.parentFileId === src.fileId)).toBe(true);
    expect(variation.asset?.versionNo).toBe(2);

    const jobId = await repo.requestPromptSuggestions({ campaignId: MOCK_CAMPAIGN_ID, preset: "poster" }, KEY);
    // The job is in the shared store, served by the job module's mocks.
    const job = await http.get<{ status: string; result: { prompts: string[] } }>(`/jobs/${jobId}`);
    expect(job.status).toBe("succeeded");
    expect(job.result.prompts).toHaveLength(3);
  });

  it("hides another creator's asset (IDOR → 404)", async () => {
    mockSession.signInAs("admin@acs.local");
    const res = await repo.generate({ campaignId: MOCK_CAMPAIGN_ID }, { ...defaultOptions(), prompt: "x" }, KEY);
    completeImageMockJobs();
    const batch = (await repo.listBatches({ campaignId: MOCK_CAMPAIGN_ID })).items.find((b) => b.job.id === res.jobId)!;
    mockSession.signInAs("creator@acs.local");
    const err = await repo.vary(batch.asset!.id, { fileId: batch.asset!.images[0]!.fileId, count: 1 }, KEY).catch((e: unknown) => e);
    expect((err as ApiError).code).toBe("NOT_FOUND");
  });
});
