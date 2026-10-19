import { useInfiniteQuery } from "@tanstack/react-query";
import { useRepository } from "@/shared/lib/di";
import type { AuditEntry, AuditQuery } from "../domain/audit";
import { settingsKeys } from "./keys";

export function useAuditLogs(q: AuditQuery) {
  const repo = useRepository("settings");
  const query = useInfiniteQuery({
    queryKey: settingsKeys.audit(q),
    queryFn: ({ pageParam }) => repo.listAuditLogs(q, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.nextCursor,
  });
  const entries: AuditEntry[] | undefined = query.data?.pages.flatMap((p) => p.items);
  return { ...query, entries };
}
