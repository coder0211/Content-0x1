import type { AssetFilters, ReviewQueueFilters } from "../domain/asset";

export const libraryKeys = {
  all: ["library"] as const,
  lists: () => [...libraryKeys.all, "list"] as const,
  list: (f: AssetFilters) => [...libraryKeys.lists(), f] as const,
  detail: (id: string) => [...libraryKeys.all, "detail", id] as const,
  versions: (id: string) => [...libraryKeys.all, "versions", id] as const,
  queue: (f: ReviewQueueFilters) => [...libraryKeys.all, "review-queue", f] as const,
  queues: () => [...libraryKeys.all, "review-queue"] as const,
  campaigns: () => [...libraryKeys.all, "campaign-options"] as const,
};
