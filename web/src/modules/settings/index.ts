import { Settings2 } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    path: "settings",
    lazy: lazyPage(() => import("./presentation/pages/SettingsPage")),
    handle: { crumb: "Cài đặt", permission: { anyOf: ["user.manage", "provider.view", "budget.manage", "audit.view"] } },
  },
];

export const navItems: NavItem[] = [
  { to: "/settings", label: "Cài đặt", icon: Settings2, group: "system", order: 20, permission: { anyOf: ["user.manage", "provider.view", "budget.manage", "audit.view"] } },
];
