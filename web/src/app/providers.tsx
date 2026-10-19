import { useState, type ReactNode } from "react";
import { MutationCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider } from "@/modules/auth";
import { isApiError } from "@/shared/kernel";
import { buildRepositories, RepositoryProvider, type Repositories } from "@/shared/lib/di";
import { shouldRetry } from "@/shared/lib/query";
import { toast, Toaster } from "@/shared/ui/toast";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { appModules } from "./modules";

/**
 * DI container: every module's `repositories` factories are instantiated once here.
 * Tests/Storybook override any entry: <AppProviders repositories={{ content: new FakeContentRepository() }}>.
 */
export function createRepositories(overrides: Repositories = {}): Repositories {
  return { ...buildRepositories(appModules.map((m) => m.repositories)), ...overrides };
}

export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: shouldRetry, staleTime: 30_000, refetchOnWindowFocus: false },
      mutations: { retry: false },
    },
    // Mutation errors → toast by error.code. Query errors render inline (<ErrorState/>).
    mutationCache: new MutationCache({
      onError: (error, _vars, _ctx, mutation) => {
        if (mutation.meta?.silentError) return;
        if (isApiError(error) && error.status === 401) return; // auth redirect handles it
        toast.error(error);
      },
    }),
  });
}

export function AppProviders({
  children,
  repositories,
  queryClient,
}: {
  children: ReactNode;
  repositories?: Repositories;
  queryClient?: QueryClient;
}) {
  const [client] = useState(() => queryClient ?? createQueryClient());
  const [repos] = useState(() => createRepositories(repositories));
  return (
    <QueryClientProvider client={client}>
      <RepositoryProvider value={repos}>
        <TooltipProvider delayDuration={300}>
          <AuthProvider>{children}</AuthProvider>
          <Toaster />
        </TooltipProvider>
      </RepositoryProvider>
    </QueryClientProvider>
  );
}
