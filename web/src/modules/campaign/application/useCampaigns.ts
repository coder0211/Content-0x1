import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ID } from "@/shared/kernel";
import { useRepository } from "@/shared/lib/di";
import { newIdempotencyKey } from "@/shared/lib/id";
import type { Campaign, CampaignAssetType, ContentPlan } from "../domain/types";
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

const TERMINAL = new Set(["succeeded", "failed", "cancelled"]);

/** Polls the content_plan job every 3 s until it ends (SSE fallback cadence). */
export function usePlanJob(campaignId: ID, jobId: ID | null) {
  const repo = useRepository("campaign");
  const qc = useQueryClient();
  return useQuery({
    queryKey: campaignKeys.job(jobId ?? ""),
    queryFn: async () => {
      const j = await repo.job(jobId!);
      // The job writes campaigns.content_plan_proposal; reload the campaign to show it.
      if (j.status === "succeeded") await qc.invalidateQueries({ queryKey: campaignKeys.detail(campaignId) });
      return j;
    },
    enabled: !!jobId,
    refetchInterval: (q) => (q.state.data && TERMINAL.has(q.state.data.status) ? false : 3000),
  });
}

export function useCampaignAssets(campaignId: ID, type: CampaignAssetType) {
  const repo = useRepository("campaign");
  return useQuery({ queryKey: campaignKeys.assets(campaignId, type), queryFn: () => repo.assets(campaignId, type) });
}
