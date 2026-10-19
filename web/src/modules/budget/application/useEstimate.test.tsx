import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RepositoryProvider } from "@/shared/lib/di";
import type { BudgetRepository } from "./ports";
import { useEstimate } from "./useEstimate";

function wrapper(repo: Partial<BudgetRepository>) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <RepositoryProvider value={{ budget: repo as BudgetRepository }}>{children}</RepositoryProvider>
    </QueryClientProvider>
  );
}

describe("useEstimate", () => {
  it("does not call the API while the request is null, then estimates", async () => {
    const estimate = vi.fn().mockResolvedValue({ total: 0.42, lines: [], currency: "USD", pricingVersion: "v", limits: { perJobLimit: 2, campaignRemaining: null, globalRemaining: 100 }, requiresOverride: false, blocked: false });
    const { result, rerender } = renderHook(({ req }) => useEstimate(req), {
      wrapper: wrapper({ estimate }),
      initialProps: { req: null as Parameters<typeof useEstimate>[0] },
    });
    expect(estimate).not.toHaveBeenCalled();
    rerender({ req: { kind: "image", count: 2 } });
    await waitFor(() => expect(result.current.data?.total).toBe(0.42));
    expect(estimate).toHaveBeenCalledWith({ kind: "image", count: 2 });
  });
});
