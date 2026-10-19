import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ID } from "@/shared/kernel";
import { useRepository } from "@/shared/lib/di";
import type { JobFilter } from "../domain/job";
import { jobKeys } from "./keys";

/** GET /jobs with cursor pagination ("Tải thêm"). Active rows refresh every 5s. */
export function useJobs(filter: JobFilter) {
  const repo = useRepository("job");
  return useInfiniteQuery({
    queryKey: jobKeys.list(filter),
    queryFn: ({ pageParam }) => repo.list(filter, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.nextCursor,
    refetchInterval: (q) =>
      q.state.data?.pages.some((p) => p.items.some((j) => j.status === "queued" || j.status === "running")) ? 5000 : false,
  });
}

export function useJobHistory(jobId: ID | undefined) {
  const repo = useRepository("job");
  return useQuery({ queryKey: jobKeys.history(jobId ?? ""), queryFn: () => repo.history(jobId as ID), enabled: !!jobId });
}

/** Cancel/retry from a list row (the detail page uses `useJob`). */
export function useJobActions() {
  const repo = useRepository("job");
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: jobKeys.all });
  const cancel = useMutation({ mutationFn: (id: ID) => repo.cancel(id), onSettled: invalidate });
  const retry = useMutation({ mutationFn: (v: { id: ID; force?: boolean }) => repo.retry(v.id, v.force), onSettled: invalidate });
  return { cancel, retry };
}
