import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ID } from "@/shared/kernel";
import { useRepository } from "@/shared/lib/di";
import { isJobActive, type CreateVideoInput, type SceneInput } from "../domain/video";
import { videoKeys } from "./keys";
import type { RenderOptions } from "./ports";

/** Poll interval while a job runs (FR-VID-08: ≤ 5s). */
export const POLL_MS = 3000;

export function useVideoList(campaignId: ID | null) {
  const repo = useRepository("video");
  return useInfiniteQuery({
    queryKey: videoKeys.list(campaignId),
    queryFn: ({ pageParam }) => repo.list({ campaignId, cursor: pageParam }),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.nextCursor,
  });
}

export function useVideoProject(assetId: ID) {
  const repo = useRepository("video");
  return useQuery({
    queryKey: videoKeys.project(assetId),
    queryFn: () => repo.get(assetId),
    refetchInterval: (q) => (isJobActive(q.state.data?.currentJob) ? POLL_MS : false),
  });
}

export function useVideoEstimate(assetId: ID, enabled: boolean) {
  const repo = useRepository("video");
  return useQuery({ queryKey: videoKeys.estimate(assetId), queryFn: () => repo.estimate(assetId), enabled });
}

export function useBrandOptions(enabled: boolean) {
  const repo = useRepository("video");
  return useQuery({ queryKey: videoKeys.brands, queryFn: () => repo.listBrands(), enabled });
}

export function useMusicTracks(brandId: ID | null) {
  const repo = useRepository("video");
  return useQuery({
    queryKey: videoKeys.music(brandId ?? ""),
    queryFn: () => repo.listMusicTracks(brandId as ID),
    enabled: !!brandId,
  });
}

export function useCreateVideo() {
  const repo = useRepository("video");
  return useMutation({
    mutationFn: ({ input, key }: { input: CreateVideoInput; key: string }) => repo.create(input, key),
    meta: { silentError: true },
  });
}

function useInvalidate(assetId: ID) {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: videoKeys.project(assetId) }),
      qc.invalidateQueries({ queryKey: videoKeys.estimate(assetId) }),
    ]);
}

export function useSaveScenes(assetId: ID) {
  const repo = useRepository("video");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (scenes: SceneInput[]) => repo.saveScenes(assetId, scenes),
    onSuccess: async (project) => {
      qc.setQueryData(videoKeys.project(assetId), project);
      await qc.invalidateQueries({ queryKey: videoKeys.estimate(assetId) });
    },
    meta: { silentError: true },
  });
}

export function useProduce(assetId: ID) {
  const repo = useRepository("video");
  const invalidate = useInvalidate(assetId);
  return useMutation({
    mutationFn: ({ budgetOverride, key }: { budgetOverride: boolean; key: string }) => repo.produce(assetId, budgetOverride, key),
    onSuccess: invalidate,
    meta: { silentError: true },
  });
}

export function useRegenerateScene(assetId: ID) {
  const repo = useRepository("video");
  const invalidate = useInvalidate(assetId);
  return useMutation({
    mutationFn: ({ sceneId, visualPrompt, key }: { sceneId: ID; visualPrompt: string | null; key: string }) =>
      repo.regenerateScene(assetId, sceneId, visualPrompt, key),
    onSuccess: invalidate,
    meta: { silentError: true },
  });
}

export function useRerender(assetId: ID) {
  const repo = useRepository("video");
  const invalidate = useInvalidate(assetId);
  return useMutation({
    mutationFn: ({ opts, key }: { opts: RenderOptions; key: string }) => repo.render(assetId, opts, key),
    onSuccess: invalidate,
    meta: { silentError: true },
  });
}

export function useSubmitForReview(assetId: ID) {
  const repo = useRepository("video");
  const invalidate = useInvalidate(assetId);
  return useMutation({ mutationFn: () => repo.submitForReview(assetId), onSuccess: invalidate });
}
