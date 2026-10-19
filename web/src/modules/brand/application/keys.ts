import type { ID } from "@/shared/kernel";
import type { ProductStatus } from "../domain/types";
import type { BrandQuery } from "./ports";

export const brandKeys = {
  all: ["brand"] as const,
  list: (q: BrandQuery) => ["brand", "list", q] as const,
  detail: (id: ID) => ["brand", "detail", id] as const,
  versions: (id: ID) => ["brand", "versions", id] as const,
  products: (id: ID, status?: ProductStatus) => ["brand", "products", id, status ?? "all"] as const,
  assets: (id: ID) => ["brand", "assets", id] as const,
};
