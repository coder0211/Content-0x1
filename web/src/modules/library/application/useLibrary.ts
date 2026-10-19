import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRepository } from "@/shared/lib/di";
import type { AssetDetail, AssetFilters, AssetUpdate, ReviewQueueFilters, SubmitReviewInput } from "../domain/asset";
import { libraryKeys } from "./keys";

export function useAssets(filters: AssetFilters) {
  const repo = useRepository("library");
  return useInfiniteQuery({
    queryKey: libraryKeys.list(filters),
    queryFn: ({ pageParam }) => repo.list(filters, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.nextCursor,
  });
}

export function useAsset(id: string | null) {
  const repo = useRepository("library");
  return useQuery({ queryKey: libraryKeys.detail(id ?? ""), queryFn: () => repo.get(id!), enabled: Boolean(id) });
}

export function useAssetVersions(id: string | null) {
  const repo = useRepository("library");
  return useQuery({ queryKey: libraryKeys.versions(id ?? ""), queryFn: () => repo.listVersions(id!), enabled: Boolean(id) });
}

export function useCampaignOptions() {
  const repo = useRepository("library");
  return useQuery({ queryKey: libraryKeys.campaigns(), queryFn: () => repo.campaignOptions(), staleTime: 5 * 60_000 });
}

function useInvalidateAsset(id: string) {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: libraryKeys.detail(id) }),
      qc.invalidateQueries({ queryKey: libraryKeys.versions(id) }),
      qc.invalidateQueries({ queryKey: libraryKeys.lists() }),
    ]);
}

export function useUpdateAsset(id: string) {
  const repo = useRepository("library");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: AssetUpdate) => repo.update(id, patch),
    onSuccess: (detail) => {
      qc.setQueryData(libraryKeys.detail(id), detail);
      void qc.invalidateQueries({ queryKey: libraryKeys.lists() });
    },
  });
}

export function useRestoreVersion(id: string) {
  const repo = useRepository("library");
  const invalidate = useInvalidateAsset(id);
  return useMutation({ mutationFn: (versionNo: number) => repo.restoreVersion(id, versionNo), onSuccess: invalidate });
}

export function useSubmitReview(id: string) {
  const repo = useRepository("library");
  const invalidate = useInvalidateAsset(id);
  return useMutation({ mutationFn: (input: SubmitReviewInput) => repo.submitReview(id, input), onSuccess: invalidate });
}

/** Download errors are rendered inline by the caller (403 for non-approved assets). */
export function useDownloadAsset(id: string) {
  const repo = useRepository("library");
  return useMutation({ mutationFn: (fileId?: string) => repo.downloadUrl(id, fileId), meta: { silentError: true } });
}

export function useDeleteAsset(id: string) {
  const repo = useRepository("library");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => repo.remove(id),
    onSuccess: () => {
      qc.removeQueries({ queryKey: libraryKeys.detail(id) });
      void qc.invalidateQueries({ queryKey: libraryKeys.lists() });
    },
  });
}

/** Review queue (pending_review, oldest submission first). */
export function useReviewQueue(filters: ReviewQueueFilters) {
  const repo = useRepository("library");
  return useInfiniteQuery({
    queryKey: libraryKeys.queue(filters),
    queryFn: ({ pageParam }) => repo.reviewQueue(filters, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.nextCursor,
  });
}

export type ReviewDecision = "approve" | "request_changes";

/**
 * Approve / request changes. Errors (403 SELF_APPROVAL_NOT_ALLOWED, 409 when another reviewer
 * decided first) are rendered inline by the caller.
 */
export function useReviewDecision() {
  const repo = useRepository("library");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, decision, comment }: { id: string; decision: ReviewDecision; comment: string }): Promise<AssetDetail> =>
      decision === "approve" ? repo.approve(id, comment) : repo.requestChanges(id, comment),
    meta: { silentError: true },
    onSuccess: (detail) => {
      qc.setQueryData(libraryKeys.detail(detail.id), detail);
      void qc.invalidateQueries({ queryKey: libraryKeys.queues() });
      void qc.invalidateQueries({ queryKey: libraryKeys.lists() });
    },
    // Another reviewer may have decided first (409): refresh what is on screen.
    onError: (_e, { id }) => {
      void qc.invalidateQueries({ queryKey: libraryKeys.detail(id) });
      void qc.invalidateQueries({ queryKey: libraryKeys.queues() });
    },
  });
}
