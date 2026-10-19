import { CheckCheck } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    path: "review",
    lazy: lazyPage(() => import("./presentation/pages/ReviewQueuePage")),
    handle: { crumb: "Duyệt", permission: "asset.review" },
  },
];

export const navItems: NavItem[] = [
  { to: "/review", label: "Duyệt", icon: CheckCheck, group: "manage", order: 20, permission: "asset.review" },
];
