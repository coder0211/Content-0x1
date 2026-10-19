import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createQueryClient } from "@/app/providers";
import { RepositoryProvider } from "@/shared/lib/di";
import type { Page } from "@/shared/kernel";
import type { Job, JobSummary } from "../domain/job";
import type { JobRepository, JobStreamHandlers } from "./ports";
import { useJob } from "./useJob";

function makeJob(over: Partial<Job> = {}): Job {
  return {
    id: "j1",
    type: "content_generation",
    status: "running",
    progress: 0,
    createdBy: "u1",
    campaignId: null,
    assetId: null,
    providerCode: "fake",
    modelCode: "m",
    cost: { estimate: 0.1, actual: null, source: null, currency: "USD" },
    error: null,
    cancelRequested: false,
    needsReconciliation: false,
    createdAt: "2026-01-01T00:00:00Z",
    startedAt: null,
    finishedAt: null,
    steps: [{ id: "s1", key: "generate", type: "text_generation", status: "running", attemptCount: 1, errorKind: null, errorMessage: null, startedAt: null, finishedAt: null }],
    request: null,
    result: null,
    ...over,
  };
}

class FakeJobRepository implements JobRepository {
  job = makeJob();
  handlers: JobStreamHandlers | null = null;
  closed = 0;
  list = async (): Promise<Page<JobSummary>> => ({ items: [], nextCursor: null });
  get = vi.fn(async () => structuredClone(this.job));
  history = async () => [];
  cancel = vi.fn(async () => ({ jobId: "j1", status: "running" as const, cancelRequested: true }));
  retry = vi.fn(async () => ({ jobId: "j1", status: "queued" as const }));
  subscribe(_id: string, h: JobStreamHandlers) {
    this.handlers = h;
    h.onModeChange?.("sse");
    return () => {
      this.closed++;
    };
  }
}

function wrapper(repo: FakeJobRepository) {
  const qc = createQueryClient();
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <RepositoryProvider value={{ job: repo }}>{children}</RepositoryProvider>
    </QueryClientProvider>
  );
}

describe("useJob", () => {
  it("applies SSE events, closes the stream and fires onFinished once", async () => {
    const repo = new FakeJobRepository();
    const onFinished = vi.fn();
    const { result } = renderHook(() => useJob("j1", { onFinished }), { wrapper: wrapper(repo) });
    await waitFor(() => expect(result.current.job?.status).toBe("running"));
    await waitFor(() => expect(result.current.mode).toBe("sse"));

    act(() => repo.handlers?.onEvent({ type: "progress", data: { progress: 40 } }));
    await waitFor(() => expect(result.current.job?.progress).toBe(40));

    repo.job = makeJob({ status: "succeeded", progress: 100, result: { asset_ids: ["a1"] } });
    act(() => repo.handlers?.onEvent({ type: "done", data: { status: "succeeded" } }));
    await waitFor(() => expect(result.current.job?.status).toBe("succeeded"));
    await waitFor(() => expect(onFinished).toHaveBeenCalledTimes(1));
    expect(repo.closed).toBe(1);
    expect(result.current.mode).toBe("closed");
  });

  it("refetches on polling ticks (SSE fallback)", async () => {
    const repo = new FakeJobRepository();
    const { result } = renderHook(() => useJob("j1"), { wrapper: wrapper(repo) });
    await waitFor(() => expect(result.current.job).toBeDefined());
    const calls = repo.get.mock.calls.length;
    repo.job = makeJob({ progress: 70 });
    act(() => {
      repo.handlers?.onModeChange?.("polling");
      repo.handlers?.onPoll();
    });
    await waitFor(() => expect(result.current.job?.progress).toBe(70));
    expect(repo.get.mock.calls.length).toBeGreaterThan(calls);
    expect(result.current.mode).toBe("polling");
  });

  it("does not subscribe for finished jobs and exposes cancel/retry", async () => {
    const repo = new FakeJobRepository();
    repo.job = makeJob({ status: "failed" });
    const { result } = renderHook(() => useJob("j1"), { wrapper: wrapper(repo) });
    await waitFor(() => expect(result.current.job?.status).toBe("failed"));
    expect(repo.handlers).toBeNull();
    await act(() => result.current.retry());
    expect(repo.retry).toHaveBeenCalledWith("j1", false);
  });

  it("is a no-op without an id", () => {
    const repo = new FakeJobRepository();
    const { result } = renderHook(() => useJob(null), { wrapper: wrapper(repo) });
    expect(result.current.job).toBeUndefined();
    expect(repo.get).not.toHaveBeenCalled();
  });
});
