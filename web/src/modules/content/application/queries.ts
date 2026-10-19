import { useQuery } from "@tanstack/react-query";
import type { ID } from "@/shared/kernel";
import { useRepository } from "@/shared/lib/di";
import { contentKeys } from "./keys";

export function useContentAsset(assetId: ID | null) {
  const repo = useRepository("content");
  return useQuery({
    queryKey: contentKeys.asset(assetId ?? "none"),
    queryFn: () => repo.getAsset(assetId!),
    enabled: !!assetId,
  });
}

export function useAssetVersions(assetId: ID | null) {
  const repo = useRepository("content");
  return useQuery({
    queryKey: contentKeys.versions(assetId ?? "none"),
    queryFn: () => repo.listVersions(assetId!),
    enabled: !!assetId,
  });
}

export function useBrands(enabled = true) {
  const repo = useRepository("content");
  return useQuery({
    queryKey: contentKeys.brands(),
    queryFn: () => repo.listBrands(),
    enabled,
  });
}

export function useProducts(brandId: ID | null) {
  const repo = useRepository("content");
  return useQuery({
    queryKey: contentKeys.products(brandId ?? "none"),
    queryFn: () => repo.listProducts(brandId!),
    enabled: !!brandId,
  });
}

export function useCampaign(campaignId: ID | null) {
  const repo = useRepository("content");
  return useQuery({
    queryKey: contentKeys.campaign(campaignId ?? "none"),
    queryFn: () => repo.getCampaign(campaignId!),
    enabled: !!campaignId,
  });
}
