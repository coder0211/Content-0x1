import { SwatchBook } from "lucide-react";
import type { RepositoryFactories } from "@/shared/lib/di";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";
import { HttpBrandRepository } from "./infrastructure/HttpBrandRepository";

export const routes: AppRoute[] = [
  {
    path: "brands",
    lazy: lazyPage(() => import("./presentation/pages/BrandsPage")),
    handle: { crumb: "Brand", permission: "brand.view" },
  },
  {
    path: "brands/:brandId",
    lazy: lazyPage(() => import("./presentation/pages/BrandDetailPage")),
    handle: { crumb: "Chi tiết brand", permission: "brand.view" },
  },
];

export const navItems: NavItem[] = [
  { to: "/brands", label: "Brand", icon: SwatchBook, group: "manage", order: 30, permission: "brand.view" },
];

export const repositories: RepositoryFactories = { brand: () => new HttpBrandRepository() };

// Public API for other modules (campaign wizard, studios).
export { useBrand, useBrandOptions, useBrands, useProducts } from "./application/useBrands";
export { activeOffers, offerStatus } from "./domain/rules";
export type { Brand, BrandProfile, Offer, Product, ProductFacts } from "./domain/types";
