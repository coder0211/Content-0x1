import { Wallet } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    path: "usage",
    lazy: lazyPage(() => import("./presentation/pages/UsagePage")),
    handle: { crumb: "Chi phí & ngân sách", permission: "usage.view" },
  },
];

export const navItems: NavItem[] = [
  { to: "/usage", label: "Chi phí & ngân sách", icon: Wallet, group: "system", order: 10, permission: "usage.view" },
];
