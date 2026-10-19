import { Library } from "lucide-react";
import type { RepositoryFactories } from "@/shared/lib/di";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";
import { HttpLibraryRepository } from "./infrastructure/HttpLibraryRepository";

export const routes: AppRoute[] = [
  {
    path: "library",
    lazy: lazyPage(() => import("./presentation/pages/LibraryPage")),
    handle: { crumb: "Thư viện", permission: "brand.view" },
  },
  {
    path: "library/:assetId",
    lazy: lazyPage(() => import("./presentation/pages/AssetDetailPage")),
    handle: { crumb: "Chi tiết asset", permission: "brand.view" },
  },
];

export const navItems: NavItem[] = [
  { to: "/library", label: "Thư viện", icon: Library, group: "manage", order: 10, permission: "brand.view" },
];

export const repositories: RepositoryFactories = { library: () => new HttpLibraryRepository() };

// Public API for other modules. The library module is the asset API client; the review
// module reuses its hooks (queue, detail, versions, decisions) and the version renderer.
export { AssetVersionContent } from "./presentation/components/AssetVersionContent";
export { versionText, ASSET_TYPE_LABELS, ASSET_TYPES } from "./domain/asset";
export { useAsset, useAssetVersions, useCampaignOptions, useReviewQueue, useReviewDecision, type ReviewDecision } from "./application/useLibrary";
export type { AssetSummary, AssetDetail, AssetVersion, AssetType, VersionFile, Approval, ReviewQueueFilters } from "./domain/asset";
