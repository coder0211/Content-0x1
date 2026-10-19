import { ApiError } from "@/shared/kernel";
import { mockSession } from "@/shared/api/mock";
import { HttpJobRepository } from "./HttpJobRepository";
import { createMockJob, resetJobMocks } from "./mocks";

const repo = new HttpJobRepository();

describe("HttpJobRepository ↔ MSW", () => {
  beforeEach(() => resetJobMocks());
  afterEach(() => resetJobMocks({ seedData: true }));

  it("lists only own jobs without provider.view and filters by status", async () => {
    const creator = mockSession.signInAs("creator@acs.local");
    createMockJob({ status: "succeeded", createdBy: creator.id });
    createMockJob({ status: "failed", createdBy: creator.id });
    createMockJob({ status: "failed", createdBy: "someone-else" });

    const all = await repo.list({});
    expect(all.items).toHaveLength(2);
    expect(all.items.every((j) => j.createdBy === creator.id)).toBe(true);

    const failed = await repo.list({ status: "failed" });
    expect(failed.items.map((j) => j.status)).toEqual(["failed"]);
  });

  it("maps the job detail with steps", async () => {
    mockSession.signInAs("creator@acs.local");
    const dto = createMockJob({ status: "failed", steps: [["a", "text_generation"], ["b", "image_generation"]] });
    const job = await repo.get(dto.id);
    expect(job.steps.map((s) => [s.key, s.status])).toEqual([
      ["a", "succeeded"],
      ["b", "failed"],
    ]);
    expect(job.steps[1]?.errorKind).toBe("Transient");
    expect(job.error?.code).toBe("STEP_FAILED");
  });

  it("cancel → 409 INVALID_STATE_TRANSITION once the job finished", async () => {
    mockSession.signInAs("creator@acs.local");
    const running = createMockJob({ status: "queued" });
    await expect(repo.cancel(running.id)).resolves.toMatchObject({ status: "cancelled", cancelRequested: true });
    const err = await repo.cancel(running.id).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(409);
    expect((err as ApiError).code).toBe("INVALID_STATE_TRANSITION");
  });

  it("retry re-opens a failed job; history records it", async () => {
    mockSession.signInAs("creator@acs.local");
    const failed = createMockJob({ status: "failed" });
    await expect(repo.retry(failed.id)).resolves.toMatchObject({ status: "queued" });
    const events = await repo.history(failed.id);
    expect(events.map((e) => e.type)).toContain("retried");
  });

  it("hides other users' jobs (404)", async () => {
    mockSession.signInAs("creator@acs.local");
    const other = createMockJob({ createdBy: "someone-else" });
    await expect(repo.get(other.id)).rejects.toMatchObject({ code: "NOT_FOUND" });
  });
});
