import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ID } from "@/shared/kernel";
import { useRepository } from "@/shared/lib/di";
import { applyStreamEvent, isTerminal, type Job } from "../domain/job";
import { jobKeys } from "./keys";
import type { JobStreamMode } from "./ports";

export interface UseJobOptions {
  /** Called once when the job reaches succeeded | failed | cancelled while mounted. */
  onFinished?: (job: Job) => void;
}

export interface UseJobResult {
  job: Job | undefined;
  isLoading: boolean;
  error: unknown;
  refetch: () => void;
  /** "sse" while streaming, "polling" after fallback (every 3s), "closed" when finished. */
  mode: JobStreamMode;
  cancel: () => Promise<unknown>;
  retry: (opts?: { force?: boolean }) => Promise<unknown>;
  isCancelling: boolean;
  isRetrying: boolean;
}

/**
 * Live job state (ACS-019): GET /jobs/{id} + SSE `/jobs/{id}/events`, falling back to polling every 3s
 * when SSE fails. Safe to call with `null` (no-op) so callers can render before a job exists.
 */
export function useJob(jobId: ID | null | undefined, opts: UseJobOptions = {}): UseJobResult {
  const repo = useRepository("job");
  const qc = useQueryClient();
  const [mode, setMode] = useState<JobStreamMode>("closed");
  const key = jobKeys.detail(jobId ?? "");

  const query = useQuery({
    queryKey: key,
    queryFn: () => repo.get(jobId as ID),
    enabled: !!jobId,
  });
  const job = query.data;
  const terminal = job ? isTerminal(job.status) : false;

  // Stream while the job is active; re-subscribe after a retry re-opens it.
  const loaded = !!job;
  useEffect(() => {
    if (!jobId || !loaded || terminal) return;
    const close = repo.subscribe(jobId, {
      onEvent: (ev) => {
        qc.setQueryData<Job>(jobKeys.detail(jobId), (prev) => (prev ? applyStreamEvent(prev, ev) : prev));
        // Step details (attempts, errors, outputs) and the final result come from the API.
        if (ev.type === "done" || ev.type === "step") void qc.invalidateQueries({ queryKey: jobKeys.detail(jobId) });
      },
      onPoll: () => void qc.invalidateQueries({ queryKey: jobKeys.detail(jobId) }),
      onModeChange: setMode,
    });
    return () => {
      close();
      setMode("closed");
    };
  }, [jobId, loaded, terminal, repo, qc]);

  // Fire onFinished on the transition to a terminal status (not for jobs already finished on mount).
  const onFinished = useRef(opts.onFinished);
  onFinished.current = opts.onFinished;
  const prevStatus = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!job) return;
    const prev = prevStatus.current;
    prevStatus.current = job.status;
    if (prev && !isTerminal(prev as Job["status"]) && isTerminal(job.status)) {
      onFinished.current?.(job);
      void qc.invalidateQueries({ queryKey: jobKeys.lists() });
    }
  }, [job, qc]);

  const cancelM = useMutation({
    mutationFn: () => repo.cancel(jobId as ID),
    onSettled: () => qc.invalidateQueries({ queryKey: jobKeys.all }),
  });
  const retryM = useMutation({
    mutationFn: (force: boolean) => repo.retry(jobId as ID, force),
    onSettled: () => qc.invalidateQueries({ queryKey: jobKeys.all }),
  });

  return {
    job,
    isLoading: query.isLoading,
    error: query.error,
    refetch: () => void query.refetch(),
    mode,
    cancel: () => cancelM.mutateAsync(),
    retry: (o) => retryM.mutateAsync(o?.force ?? false),
    isCancelling: cancelM.isPending,
    isRetrying: retryM.isPending,
  };
}
