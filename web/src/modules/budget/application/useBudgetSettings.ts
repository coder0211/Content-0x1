import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRepository } from "@/shared/lib/di";
import type { BudgetSettingsPatch, CampaignBudgetInput } from "../domain/budget";
import { budgetKeys } from "./keys";

export function useBudgetSettings() {
  const repo = useRepository("budget");
  return useQuery({ queryKey: budgetKeys.settings(), queryFn: () => repo.getSettings() });
}

/** The campaign's own budget, or `null` when none is set. */
export function useCampaignBudget(campaignId: string | undefined) {
  const repo = useRepository("budget");
  return useQuery({
    queryKey: budgetKeys.campaign(campaignId ?? ""),
    queryFn: () => repo.getCampaignBudget(campaignId!),
    enabled: Boolean(campaignId),
  });
}

export function useUpdateBudgetSettings() {
  const repo = useRepository("budget");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: BudgetSettingsPatch) => repo.updateSettings(patch),
    onSuccess: (data) => {
      qc.setQueryData(budgetKeys.settings(), data);
      void qc.invalidateQueries({ queryKey: ["budget", "estimate"] });
    },
  });
}

export function usePutCampaignBudget() {
  const repo = useRepository("budget");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ campaignId, input }: { campaignId: string; input: CampaignBudgetInput }) => repo.putCampaignBudget(campaignId, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: budgetKeys.all }),
  });
}
