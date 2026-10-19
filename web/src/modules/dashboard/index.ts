import { LayoutDashboard } from "lucide-react";
import type { RepositoryFactories } from "@/shared/lib/di";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";
import { HttpDashboardRepository } from "./infrastructure/HttpDashboardRepository";

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

export const repositories: RepositoryFactories = { dashboard: () => new HttpDashboardRepository() };
