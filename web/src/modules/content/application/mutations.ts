import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { ID } from "@/shared/kernel";
import { useRepository } from "@/shared/lib/di";
import { newIdempotencyKey } from "@/shared/lib/id";
import type {
  ContentFormat,
  GenerateInput,
  RewriteInput,
  SaveVersionInput,
} from "../domain/content";
import { contentKeys } from "./keys";

/**
 * AI job mutations take a stable idempotency key per user intent: the caller
 * creates one key per click and reuses it if the same request is retried.
 */
export function useGenerate() {
  const repo = useRepository("content");
  return useMutation({
    mutationFn: ({ input, key }: { input: GenerateInput; key: string }) =>
      repo.generate(input, key),
    meta: { silentError: true },
  });
}

export function useRewrite(assetId: ID) {
  const repo = useRepository("content");
  return useMutation({
    mutationFn: (input: RewriteInput) =>
      repo.rewrite(assetId, input, newIdempotencyKey()),
  });
}

export function useTransform(assetId: ID) {
  const repo = useRepository("content");
  return useMutation({
    mutationFn: (target: ContentFormat) =>
      repo.transform(assetId, target, newIdempotencyKey()),
  });
}

export function useCancelJob() {
  const repo = useRepository("content");
  return useMutation({ mutationFn: (jobId: ID) => repo.cancelJob(jobId) });
}

export function useSaveVersion(assetId: ID) {
  const repo = useRepository("content");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: SaveVersionInput) => repo.saveVersion(assetId, input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: contentKeys.asset(assetId) });
    },
  });
}

export function useRestoreVersion(assetId: ID) {
  const repo = useRepository("content");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (versionNo: number) => repo.restoreVersion(assetId, versionNo),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: contentKeys.asset(assetId) });
    },
  });
}
