import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ID } from "@/shared/kernel";
import { useRepository } from "@/shared/lib/di";

export const dashboardKeys = {
  all: ["dashboard"] as const,
  summary: () => [...dashboardKeys.all, "summary"] as const,
  notifications: () => [...dashboardKeys.all, "notifications"] as const,
};

/** Dashboard widgets; refreshed every minute while the page is open. */
export function useDashboardSummary() {
  const repo = useRepository("dashboard");
  return useQuery({ queryKey: dashboardKeys.summary(), queryFn: () => repo.summary(), refetchInterval: 60_000 });
}

/** Topbar bell: the latest notifications, polled every minute. */
export function useNotifications(enabled = true) {
  const repo = useRepository("dashboard");
  return useQuery({
    queryKey: dashboardKeys.notifications(),
    queryFn: () => repo.notifications(20),
    refetchInterval: 60_000,
    enabled,
  });
}

export function useMarkNotificationRead() {
  const repo = useRepository("dashboard");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: ID) => repo.markNotificationRead(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: dashboardKeys.notifications() }),
    meta: { silentError: true },
  });
}
