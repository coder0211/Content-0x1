import { createContext, useContext, type ReactNode } from "react";

/**
 * Repository registry, extended by each module via declaration merging in its
 * `application/ports.ts`:
 *
 *   declare module "@/shared/lib/di" {
 *     interface RepositoryRegistry { content: ContentRepository }
 *   }
 *
 * Implementations are registered in the module's `index.ts` (`repositories`), wired by `app/providers.tsx`.
 */
// eslint-disable-next-line @typescript-eslint/no-empty-object-type
export interface RepositoryRegistry {}

export type RepositoryKey = keyof RepositoryRegistry;

export type RepositoryFactories = { [K in RepositoryKey]?: () => RepositoryRegistry[K] };

export type Repositories = Partial<RepositoryRegistry>;

const RepositoryContext = createContext<Repositories | null>(null);

export function RepositoryProvider({ value, children }: { value: Repositories; children: ReactNode }) {
  return <RepositoryContext.Provider value={value}>{children}</RepositoryContext.Provider>;
}

export function useRepository<K extends RepositoryKey>(key: K): RepositoryRegistry[K] {
  const repos = useContext(RepositoryContext);
  if (!repos) throw new Error("useRepository must be used inside <RepositoryProvider>");
  const repo = repos[key];
  if (!repo) throw new Error(`Repository "${String(key)}" is not registered`);
  return repo as RepositoryRegistry[K];
}

export function buildRepositories(factories: readonly (RepositoryFactories | undefined)[]): Repositories {
  const out: Record<string, unknown> = {};
  for (const group of factories) {
    if (!group) continue;
    for (const [key, factory] of Object.entries(group) as [string, (() => unknown) | undefined][]) {
      if (factory) out[key] = factory();
    }
  }
  return out as Repositories;
}
