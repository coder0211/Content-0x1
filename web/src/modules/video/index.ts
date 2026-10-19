import { Clapperboard } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    path: "videos",
    lazy: lazyPage(() => import("./presentation/pages/VideoStudioPage")),
    handle: { crumb: "Video Studio", permission: "generate.run" },
  },
];

export const navItems: NavItem[] = [
  { to: "/videos", label: "Video", icon: Clapperboard, group: "work", order: 50, permission: "generate.run" },
];
