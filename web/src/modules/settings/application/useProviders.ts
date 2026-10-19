import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRepository } from "@/shared/lib/di";
import type { CreateModelInput, NewPricingInput, UpdateModelInput, UpdateProviderInput } from "../domain/provider";
import { settingsKeys } from "./keys";

export function useProviders() {
  const repo = useRepository("settings");
  return useQuery({ queryKey: settingsKeys.providers(), queryFn: () => repo.listProviders() });
}

export function useUpdateProvider() {
  const repo = useRepository("settings");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateProviderInput }) => repo.updateProvider(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsKeys.providers() }),
  });
}

export function useModels(providerId: string | undefined) {
  const repo = useRepository("settings");
  return useQuery({
    queryKey: settingsKeys.models(providerId),
    queryFn: () => repo.listModels(providerId),
    enabled: Boolean(providerId),
  });
}

export function useCreateModel() {
  const repo = useRepository("settings");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateModelInput) => repo.createModel(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsKeys.modelsAll() }),
  });
}

export function useUpdateModel() {
  const repo = useRepository("settings");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateModelInput }) => repo.updateModel(id, input),
    // Setting a default may unset another model's default → refetch all lists.
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsKeys.modelsAll() }),
  });
}

export function usePricing(modelId: string | null) {
  const repo = useRepository("settings");
  return useQuery({
    queryKey: settingsKeys.pricing(modelId ?? ""),
    queryFn: () => repo.listPricing(modelId!),
    enabled: Boolean(modelId),
  });
}

export function useAddPricing(modelId: string) {
  const repo = useRepository("settings");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: NewPricingInput) => repo.addPricing(modelId, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsKeys.pricing(modelId) }),
  });
}
