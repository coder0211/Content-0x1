import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router";
import { useSession } from "@/shared/lib/session";
import { ErrorState } from "@/shared/ui/patterns";
import { useAuthInternals } from "../../application/AuthProvider";

/** Route guard: anonymous → /login?next=…; session expiry adds `reason=expired`. */
export function RequireAuth({ children }: { children: ReactNode }) {
  const session = useSession();
  const { endReason, meError, retryMe } = useAuthInternals();
  const location = useLocation();

  if (meError) {
    return (
      <div className="mx-auto max-w-lg p-6">
        <ErrorState error={meError} onRetry={retryMe} title="Không tải được phiên đăng nhập." />
      </div>
    );
  }
  if (session.status === "loading") {
    return <p className="p-6 text-small text-neutral-500">Đang tải phiên đăng nhập…</p>;
  }
  if (session.status === "anonymous") {
    if (endReason === "logout") return <Navigate to="/login" replace />;
    const params = new URLSearchParams({ next: location.pathname + location.search });
    if (endReason === "expired") params.set("reason", "expired");
    return <Navigate to={`/login?${params.toString()}`} replace />;
  }
  return <>{children}</>;
}
