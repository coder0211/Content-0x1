import { http as mswHttp, HttpResponse } from "msw";
import { server } from "@/app/mocks/server";
import { api, mockSession } from "@/shared/api/mock";
import { isApiError } from "@/shared/kernel";
import type { GenerateInput } from "../domain/content";
import { HttpContentRepository } from "./HttpContentRepository";
import { MOCK_BRANDS, resetContentMocks, setContentMockPolls } from "./mocks";

const input: GenerateInput = {
  campaignId: null,
  brandId: MOCK_BRANDS[0]!.id,
  platform: "facebook",
  contentType: "promotional_post",
  objective: "lead",
  framework: "none",
  length: "medium",
  language: "vi",
  tone: ["thân thiện"],
  brief: {
    productId: null,
    targetAudience: "Sinh viên",
    keyMessage: "Tự tin giao tiếp",
    cta: "Đăng ký",
    referenceText: "",
  },
  variants: 3,
};

afterEach(() => resetContentMocks());

describe("HttpContentRepository", () => {
  it("sends the SPEC-04 body with an Idempotency-Key and maps the 202", async () => {
    mockSession.signInAs("creator@acs.local");
    let body: unknown = null;
    let key: string | null = null;
    server.events.on("request:start", ({ request }) => {
      if (request.url.endsWith("/content/generations")) {
        key = request.headers.get("Idempotency-Key");
        void request
          .clone()
          .json()
          .then((b) => (body = b));
      }
    });
    const repo = new HttpContentRepository();
    const acc = await repo.generate(
      input,
      "11111111-1111-4111-8111-111111111111",
    );
    server.events.removeAllListeners();
    expect(key).toBe("11111111-1111-4111-8111-111111111111");
    expect(body).toMatchObject({
      brand_id: input.brandId,
      platform: "facebook",
      framework: "",
      variants: 3,
      brief: { target_audience: "Sinh viên" },
    });
    expect(acc.status).toBe("queued");
    expect(acc.estimatedCost).toEqual({ amount: 0.02, currency: "USD" });
  });

  it("polls the job to success and loads the created assets and versions", async () => {
    mockSession.signInAs("creator@acs.local");
    setContentMockPolls(1);
    const repo = new HttpContentRepository();
    const acc = await repo.generate(
      input,
      "22222222-2222-4222-8222-222222222222",
    );
    const job = await repo.getJob(acc.jobId);
    expect(job.status).toBe("succeeded");
    expect(job.assetIds).toHaveLength(3);
    const asset = await repo.getAsset(job.assetIds[1]!);
    expect(asset.currentVersion?.versionNo).toBe(1);
    expect(asset.currentVersion?.content?.warnings).toEqual([
      { type: "unverified_claim", text: "giảm 50%" },
    ]);
    const saved = await repo.saveVersion(asset.id, {
      content: asset.currentVersion!.content!,
      changeNote: "Sửa",
      baseVersionNo: 1,
    });
    expect(saved.versionNo).toBe(2);
    const restored = await repo.restoreVersion(asset.id, 1);
    expect(restored.versionNo).toBe(3);
    expect((await repo.listVersions(asset.id)).map((v) => v.versionNo)).toEqual(
      [3, 2, 1],
    );
    await expect(
      repo.saveVersion(asset.id, {
        content: asset.currentVersion!.content!,
        changeNote: "",
        baseVersionNo: 1,
      }),
    ).rejects.toSatisfy((e) => isApiError(e) && e.code === "VERSION_CONFLICT");
  });

  it("maps job errors", async () => {
    mockSession.signInAs("creator@acs.local");
    server.use(
      mswHttp.get(api("/jobs/:id"), () =>
        HttpResponse.json({
          data: {
            id: "j1",
            job_type: "content_generation",
            status: "failed",
            progress: 30,
            result: null,
            error: { kind: "ContentPolicy", message: "x" },
          },
          meta: { request_id: "r" },
        }),
      ),
    );
    const job = await new HttpContentRepository().getJob("j1");
    expect(job).toMatchObject({
      status: "failed",
      errorKind: "ContentPolicy",
      assetIds: [],
    });
  });

  it("rejects reviewers (403)", async () => {
    mockSession.signInAs("reviewer@acs.local");
    await expect(
      new HttpContentRepository().generate(
        input,
        "33333333-3333-4333-8333-333333333333",
      ),
    ).rejects.toSatisfy((e) => isApiError(e) && e.code === "FORBIDDEN");
  });
});
