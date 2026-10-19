import { SwatchBook } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    path: "brands",
    lazy: lazyPage(() => import("./presentation/pages/BrandsPage")),
    handle: { crumb: "Brand", permission: "brand.view" },
  },
];

export const navItems: NavItem[] = [
  { to: "/brands", label: "Brand", icon: SwatchBook, group: "manage", order: 30, permission: "brand.view" },
];
