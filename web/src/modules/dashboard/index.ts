import { LayoutDashboard } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    index: true,
    lazy: lazyPage(() => import("./presentation/pages/DashboardPage")),
    handle: { crumb: "Tổng quan" },
  },
];

export const navItems: NavItem[] = [
  { to: "/", label: "Tổng quan", icon: LayoutDashboard, group: "work", order: 10 },
];
