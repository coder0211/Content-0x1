import { useEffect, useRef, useState } from "react";
import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { newIdempotencyKey } from "@/shared/lib/id";
import { useJob } from "@/modules/job";
import { useRepository } from "@/shared/lib/di";
import type { ID } from "@/shared/kernel";
import { isBatchActive, type GenerationOptions, type ImageBatch, type Preset, type StudioTarget } from "../domain/image";
import { imageKeys } from "./keys";
import type { EditInput, EstimateQuery, VariationInput } from "./ports";

/** Poll interval while a batch job is queued/running (SPEC-01 FR-VID-08: ≤ 5s). */
export const ACTIVE_POLL_MS = 2000;

/** Gallery of batches, newest first; polls while any job is active. */
export function useImageBatches(campaignId: ID | undefined) {
  const repo = useRepository("image");
  const q = { campaignId };
  return useInfiniteQuery({
    queryKey: imageKeys.batches(q),
    queryFn: ({ pageParam }) => repo.listBatches({ ...q, cursor: pageParam }),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.nextCursor,
    refetchInterval: (query) =>
      query.state.data?.pages.some((p) => p.items.some(isBatchActive)) ? ACTIVE_POLL_MS : false,
  });
}

/** Debounces a value (estimate is re-priced when options settle). */
export function useDebounced<T>(value: T, ms = 300): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

export function useImageEstimate(q: EstimateQuery, enabled: boolean) {
  const repo = useRepository("image");
  const debounced = useDebounced(q);
  return useQuery({
    queryKey: imageKeys.estimate(debounced),
    queryFn: () => repo.estimate(debounced),
    enabled,
    placeholderData: keepPreviousData,
    retry: false,
  });
}

/**
 * Wraps a job-creating mutation with ONE idempotency key per user intent: the key
 * is reused when the same intent is retried after a failure and renewed after success.
 */
function useIntentKey() {
  const key = useRef<string | null>(null);
  return {
    get: () => (key.current ??= newIdempotencyKey()),
    reset: () => {
      key.current = null;
    },
  };
}

export function useGenerateImages() {
  const repo = useRepository("image");
  const qc = useQueryClient();
  const intent = useIntentKey();
  return useMutation({
    mutationFn: ({ target, options }: { target: StudioTarget; options: GenerationOptions }) =>
      repo.generate(target, options, intent.get()),
    onSuccess: () => {
      intent.reset();
      return qc.invalidateQueries({ queryKey: ["image", "batches"] });
    },
    // The form renders 400/422 next to the fields; other errors toast.
    meta: { silentError: true },
  });
}

export function useEditImage() {
  const repo = useRepository("image");
  const qc = useQueryClient();
  const intent = useIntentKey();
  return useMutation({
    mutationFn: ({ assetId, input }: { assetId: ID; input: EditInput }) => repo.edit(assetId, input, intent.get()),
    onSuccess: () => {
      intent.reset();
      return qc.invalidateQueries({ queryKey: ["image", "batches"] });
    },
  });
}

export function useVaryImage() {
  const repo = useRepository("image");
  const qc = useQueryClient();
  const intent = useIntentKey();
  return useMutation({
    mutationFn: ({ assetId, input }: { assetId: ID; input: VariationInput }) => repo.vary(assetId, input, intent.get()),
    onSuccess: () => {
      intent.reset();
      return qc.invalidateQueries({ queryKey: ["image", "batches"] });
    },
  });
}

export function useImageReferences(target: StudioTarget | null) {
  const repo = useRepository("image");
  return useQuery({
    queryKey: imageKeys.references(target),
    queryFn: () => repo.listReferences(target!),
    enabled: target != null,
  });
}

export function useUploadReference() {
  const repo = useRepository("image");
  return useMutation({ mutationFn: (file: File) => repo.uploadReference(file) });
}

/** "AI đề xuất prompt" (FR-IMG-03): starts the suggestion job, then follows it until it has prompts. */
export function usePromptSuggestions() {
  const repo = useRepository("image");
  const intent = useIntentKey();
  const [jobId, setJobId] = useState<ID | null>(null);
  const start = useMutation({
    mutationFn: (input: { campaignId?: ID; brandId?: ID; preset: Preset }) => repo.requestPromptSuggestions(input, intent.get()),
    onSuccess: (id) => {
      intent.reset();
      setJobId(id);
    },
  });
  // Job state through the job module (SSE with 3s polling fallback); prompts are in `result.prompts`.
  const { job } = useJob(jobId);
  const prompts = job?.status === "succeeded" && Array.isArray(job.result?.prompts) ? (job.result.prompts as unknown[]).filter((p): p is string => typeof p === "string") : [];
  return {
    start: start.mutate,
    starting: start.isPending,
    status: jobId == null ? null : (job?.status ?? "queued"),
    prompts,
    errorKind: job?.status === "failed" ? (job.steps.find((s) => s.status === "failed")?.errorKind ?? job.error?.code ?? null) : null,
    dismiss: () => setJobId(null),
  };
}

export function useRetryImageJob() {
  const repo = useRepository("image");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (jobId: ID) => repo.retryJob(jobId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["image", "batches"] }),
  });
}

export function useSubmitImageReview() {
  const repo = useRepository("image");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ assetId, typographyChecked }: { assetId: ID; typographyChecked: boolean }) =>
      repo.submitForReview(assetId, { typographyChecked }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["image", "batches"] }),
  });
}

export function useStudioTargets() {
  const repo = useRepository("image");
  const campaigns = useQuery({ queryKey: imageKeys.campaigns(), queryFn: () => repo.listCampaigns() });
  const brands = useQuery({ queryKey: imageKeys.brands(), queryFn: () => repo.listBrands() });
  return { campaigns, brands };
}

/** Flattened gallery items. */
export const flattenBatches = (pages: { items: ImageBatch[] }[] | undefined): ImageBatch[] =>
  pages?.flatMap((p) => p.items) ?? [];
