import { Image } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";
import type { RepositoryFactories } from "@/shared/lib/di";
import { HttpImageRepository } from "./infrastructure/HttpImageRepository";

export const routes: AppRoute[] = [
  {
    path: "images",
    lazy: lazyPage(() => import("./presentation/pages/ImageStudioPage")),
    handle: { crumb: "Image Studio", permission: "generate.run" },
  },
];

export const navItems: NavItem[] = [
  { to: "/images", label: "Hình ảnh", icon: Image, group: "work", order: 40, permission: "generate.run" },
];

export const repositories: RepositoryFactories = { image: () => new HttpImageRepository() };

/** Deep link into Image Studio for a campaign (campaign detail "Images" tab, dashboard shortcuts). */
export const imageStudioPath = (campaignId?: string): string => (campaignId ? `/images?campaign_id=${campaignId}` : "/images");
