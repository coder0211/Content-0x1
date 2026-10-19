import type { BatchQuery, EstimateQuery } from "./ports";
import type { StudioTarget } from "../domain/image";

export const imageKeys = {
  all: ["image"] as const,
  batches: (q: Omit<BatchQuery, "cursor">) => ["image", "batches", q] as const,
  estimate: (q: EstimateQuery) => ["image", "estimate", q] as const,
  references: (t: StudioTarget | null) => ["image", "references", t] as const,
  suggestions: (jobId: string | null) => ["image", "suggestions", jobId] as const,
  campaigns: () => ["image", "targets", "campaigns"] as const,
  brands: () => ["image", "targets", "brands"] as const,
};
