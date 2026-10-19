import type { JobFilter } from "../domain/job";

export const jobKeys = {
  all: ["job"] as const,
  lists: () => ["job", "list"] as const,
  list: (f: JobFilter) => ["job", "list", f] as const,
  detail: (id: string) => ["job", "detail", id] as const,
  history: (id: string) => ["job", "history", id] as const,
};
