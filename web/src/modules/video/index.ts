import { Clapperboard } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";
import type { RepositoryFactories } from "@/shared/lib/di";
import { HttpVideoRepository } from "./infrastructure/HttpVideoRepository";

export type { VideoRepository } from "./application/ports";

export const routes: AppRoute[] = [
  {
    path: "videos",
    lazy: lazyPage(() => import("./presentation/pages/VideoStudioPage")),
    handle: { crumb: "Video Studio", permission: "generate.run" },
  },
  {
    path: "videos/:assetId",
    lazy: lazyPage(() => import("./presentation/pages/VideoProjectPage")),
    handle: { crumb: "Dự án video", permission: "brand.view" },
  },
];

export const navItems: NavItem[] = [
  { to: "/videos", label: "Video", icon: Clapperboard, group: "work", order: 50, permission: "generate.run" },
];

export const repositories: RepositoryFactories = {
  video: () => new HttpVideoRepository(),
};
