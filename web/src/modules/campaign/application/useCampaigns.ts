import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { useJob } from "@/modules/job";
import type { ID } from "@/shared/kernel";
import { useRepository } from "@/shared/lib/di";
import { newIdempotencyKey } from "@/shared/lib/id";
import type { Campaign, CampaignAssetType, ContentPlan, PlanJob } from "../domain/types";
import { campaignKeys } from "./keys";
import type { CampaignQuery, CreateCampaignInput, UpdateCampaignInput } from "./ports";

export function useCampaigns(q: CampaignQuery = {}) {
  const repo = useRepository("campaign");
  return useQuery({ queryKey: campaignKeys.list(q), queryFn: () => repo.list(q), placeholderData: keepPreviousData });
}

export function useCampaign(id: ID | undefined) {
  const repo = useRepository("campaign");
  return useQuery({ queryKey: campaignKeys.detail(id ?? ""), queryFn: () => repo.get(id!), enabled: !!id });
}

function useCampaignWriter() {
  const qc = useQueryClient();
  return (c: Campaign) => {
    qc.setQueryData(campaignKeys.detail(c.id), c);
    void qc.invalidateQueries({ queryKey: campaignKeys.lists });
  };
}

export function useCreateCampaign() {
  const repo = useRepository("campaign");
  const write = useCampaignWriter();
  return useMutation({ mutationFn: (input: CreateCampaignInput) => repo.create(input), onSuccess: write, meta: { silentError: true } });
}

export function useUpdateCampaign(id: ID) {
  const repo = useRepository("campaign");
  const write = useCampaignWriter();
  return useMutation({ mutationFn: (input: UpdateCampaignInput) => repo.update(id, input), onSuccess: write, meta: { silentError: true } });
}

export function useMemberCandidates(enabled = true) {
  const repo = useRepository("campaign");
  return useQuery({ queryKey: campaignKeys.candidates, queryFn: () => repo.memberCandidates(), enabled, staleTime: 5 * 60_000 });
}

export function useSavePlan(id: ID) {
  const repo = useRepository("campaign");
  const write = useCampaignWriter();
  return useMutation({ mutationFn: (plan: ContentPlan) => repo.savePlan(id, plan), onSuccess: write, meta: { silentError: true } });
}

export function useDiscardProposal(id: ID) {
  const repo = useRepository("campaign");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => repo.discardProposal(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: campaignKeys.detail(id) }),
  });
}

/** "Tạo kế hoạch": one idempotency key per click (FE-CONVENTIONS §5). */
export function useGeneratePlan(id: ID) {
  const repo = useRepository("campaign");
  return useMutation({ mutationFn: (key: string = newIdempotencyKey()) => repo.generatePlan(id, key) });
}

/**
 * Live content_plan job through the job module (`useJob`: SSE, 3 s polling fallback). The job writes
 * campaigns.content_plan_proposal, so the campaign is reloaded when it succeeds.
 */
export function usePlanJob(campaignId: ID, jobId: ID | null): { data: PlanJob | undefined } {
  const qc = useQueryClient();
  const { job } = useJob(jobId);
  const succeeded = job?.status === "succeeded";
  useEffect(() => {
    if (succeeded) void qc.invalidateQueries({ queryKey: campaignKeys.detail(campaignId) });
  }, [succeeded, campaignId, qc]);
  if (!job) return { data: undefined };
  const failed = job.steps.find((s) => s.status === "failed");
  return {
    data: {
      id: job.id,
      status: job.status,
      progress: job.progress,
      errorKind: failed?.errorKind ?? job.error?.code ?? null,
      errorDetail: job.error?.message ?? null,
    },
  };
}

export function useCampaignAssets(campaignId: ID, type: CampaignAssetType) {
  const repo = useRepository("campaign");
  return useQuery({ queryKey: campaignKeys.assets(campaignId, type), queryFn: () => repo.assets(campaignId, type) });
}
