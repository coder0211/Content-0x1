import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ID } from "@/shared/kernel";
import { useRepository } from "@/shared/lib/di";
import type { AddAssetInput, BrandProfile, ProductInput, ProductStatus } from "../domain/types";
import { brandKeys } from "./keys";
import type { BrandQuery, UpdateBrandInput } from "./ports";

export function useBrands(q: BrandQuery = {}) {
  const repo = useRepository("brand");
  return useQuery({ queryKey: brandKeys.list(q), queryFn: () => repo.list(q), placeholderData: keepPreviousData });
}

/** All brands as select options (first 100), for other modules (campaign wizard, filters). */
export function useBrandOptions() {
  const q = useBrands({ limit: 100 });
  return { ...q, options: (q.data?.items ?? []).map((b) => ({ value: b.id, label: b.name })) };
}

export function useBrand(id: ID | undefined) {
  const repo = useRepository("brand");
  return useQuery({ queryKey: brandKeys.detail(id ?? ""), queryFn: () => repo.get(id!), enabled: !!id });
}

export function useBrandVersions(id: ID) {
  const repo = useRepository("brand");
  return useQuery({ queryKey: brandKeys.versions(id), queryFn: () => repo.versions(id) });
}

export function useCreateBrand() {
  const repo = useRepository("brand");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; profile: BrandProfile }) => repo.create(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["brand", "list"] }),
    meta: { silentError: true },
  });
}

export function useUpdateBrand(id: ID) {
  const repo = useRepository("brand");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateBrandInput) => repo.update(id, input),
    onSuccess: (b) => {
      qc.setQueryData(brandKeys.detail(id), b);
      void qc.invalidateQueries({ queryKey: brandKeys.versions(id) });
      void qc.invalidateQueries({ queryKey: ["brand", "list"] });
    },
    meta: { silentError: true },
  });
}

export function useProducts(brandId: ID | undefined, status?: ProductStatus) {
  const repo = useRepository("brand");
  return useQuery({
    queryKey: brandKeys.products(brandId ?? "", status),
    queryFn: () => repo.products(brandId!, status),
    enabled: !!brandId,
  });
}

export function useSaveProduct(brandId: ID) {
  const repo = useRepository("brand");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id?: ID; input: ProductInput }) => (id ? repo.updateProduct(id, input) : repo.createProduct(brandId, input)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["brand", "products", brandId] }),
    meta: { silentError: true },
  });
}

export function useBrandAssets(brandId: ID) {
  const repo = useRepository("brand");
  return useQuery({ queryKey: brandKeys.assets(brandId), queryFn: () => repo.assets(brandId) });
}

export function useAddBrandAsset(brandId: ID) {
  const repo = useRepository("brand");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AddAssetInput) => repo.addAsset(brandId, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: brandKeys.assets(brandId) }),
    meta: { silentError: true },
  });
}

export function useDeleteBrandAsset(brandId: ID) {
  const repo = useRepository("brand");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (assetId: ID) => repo.deleteAsset(brandId, assetId),
    onSuccess: () => qc.invalidateQueries({ queryKey: brandKeys.assets(brandId) }),
  });
}
