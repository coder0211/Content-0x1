import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useRepository } from "@/shared/lib/di";
import type { UsageSummaryQuery } from "../domain/budget";
import { budgetKeys } from "./keys";

export function useUsageSummary(q: UsageSummaryQuery, opts: { enabled?: boolean } = {}) {
  const repo = useRepository("budget");
  return useQuery({ queryKey: budgetKeys.summary(q), queryFn: () => repo.usageSummary(q), placeholderData: keepPreviousData, enabled: opts.enabled ?? true });
}
