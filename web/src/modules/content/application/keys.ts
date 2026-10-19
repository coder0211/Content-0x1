import type { ID } from "@/shared/kernel";

export const contentKeys = {
  all: ["content"] as const,
  asset: (id: ID) => ["content", "asset", id] as const,
  versions: (id: ID) => ["content", "asset", id, "versions"] as const,
  brands: () => ["content", "brands"] as const,
  products: (brandId: ID) =>
    ["content", "brands", brandId, "products"] as const,
  campaign: (id: ID) => ["content", "campaign", id] as const,
};
