import { Megaphone } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    path: "campaigns",
    lazy: lazyPage(() => import("./presentation/pages/CampaignsPage")),
    handle: { crumb: "Campaign", permission: "brand.view" },
  },
];

export const navItems: NavItem[] = [
  { to: "/campaigns", label: "Campaign", icon: Megaphone, group: "work", order: 20, permission: "brand.view" },
];
