import { useQuery } from "@tanstack/react-query";
import { useRepository } from "@/shared/lib/di";
import type { EstimateRequest } from "../domain/budget";
import { budgetKeys } from "./keys";

/**
 * Live cost estimate for a studio's generate area (cost chip + over-budget banner).
 * Pass `null` while the form is incomplete. Errors keep their `ApiError.code`
 * (e.g. `PRICING_MISSING` → disable the generate button).
 */
export function useEstimate(req: EstimateRequest | null) {
  const repo = useRepository("budget");
  return useQuery({
    queryKey: budgetKeys.estimate(req ?? { kind: "text" }),
    queryFn: () => repo.estimate(req as EstimateRequest),
    enabled: req !== null,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
    retry: false,
  });
}
