import { useQuery } from "@tanstack/react-query";
import { useRepository } from "@/shared/lib/di";

export const dashboardKeys = {
  all: ["dashboard"] as const,
  summary: () => [...dashboardKeys.all, "summary"] as const,
};

/** Dashboard widgets; refreshed every minute while the page is open. */
export function useDashboardSummary() {
  const repo = useRepository("dashboard");
  return useQuery({ queryKey: dashboardKeys.summary(), queryFn: () => repo.summary(), refetchInterval: 60_000 });
}
