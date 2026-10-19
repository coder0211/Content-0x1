import { useRouteError } from "react-router";
import { ErrorState } from "@/shared/ui/patterns";

/** Fallback for render/lazy-chunk errors. */
export function RouteErrorPage() {
  const error = useRouteError();
  return (
    <div className="mx-auto max-w-xl p-6">
      <ErrorState error={error} title="Không hiển thị được trang." onRetry={() => window.location.reload()} />
    </div>
  );
}
