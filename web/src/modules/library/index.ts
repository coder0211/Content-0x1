import { Library } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    path: "library",
    lazy: lazyPage(() => import("./presentation/pages/LibraryPage")),
    handle: { crumb: "Thư viện", permission: "brand.view" },
  },
];

export const navItems: NavItem[] = [
  { to: "/library", label: "Thư viện", icon: Library, group: "manage", order: 10, permission: "brand.view" },
];
