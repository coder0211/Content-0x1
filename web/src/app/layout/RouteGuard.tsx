import type { ReactNode } from "react";
import { useMatches } from "react-router";
import type { RouteHandle } from "@/shared/lib/module";
import { useSession } from "@/shared/lib/session";
import { NoPermission } from "@/shared/ui/patterns";

/** Enforces `handle.permission` of every matched route; shows NoPermission instead of hiding silently. */
export function RouteGuard({ children }: { children: ReactNode }) {
  const { can } = useSession();
  const allowed = useMatches().every((m) => can((m.handle as RouteHandle | undefined)?.permission));
  return allowed ? <>{children}</> : <NoPermission />;
}
