import { Settings2 } from "lucide-react";
import type { RepositoryFactories } from "@/shared/lib/di";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";
import { HttpSettingsRepository } from "./infrastructure/HttpSettingsRepository";

const SETTINGS_PERMISSION = { anyOf: ["user.manage", "provider.view", "budget.manage", "audit.view"] } as const;

export const routes: AppRoute[] = [
  {
    path: "settings",
    lazy: lazyPage(() => import("./presentation/pages/SettingsLayout")),
    handle: { crumb: "Cài đặt", permission: { anyOf: [...SETTINGS_PERMISSION.anyOf] } },
    children: [
      { index: true, lazy: lazyPage(() => import("./presentation/pages/SettingsIndexPage")) },
      {
        path: "users",
        lazy: lazyPage(() => import("./presentation/pages/UsersPage")),
        handle: { crumb: "Người dùng & vai trò", permission: "user.manage" },
      },
      {
        path: "providers",
        lazy: lazyPage(() => import("./presentation/pages/ProvidersPage")),
        handle: { crumb: "Provider & model", permission: "provider.view" },
      },
      {
        path: "audit",
        lazy: lazyPage(() => import("./presentation/pages/AuditLogPage")),
        handle: { crumb: "Nhật ký hệ thống", permission: "audit.view" },
      },
    ],
  },
];

export const navItems: NavItem[] = [
  { to: "/settings", label: "Cài đặt", icon: Settings2, group: "system", order: 20, permission: { anyOf: [...SETTINGS_PERMISSION.anyOf] } },
];

export const repositories: RepositoryFactories = { settings: () => new HttpSettingsRepository() };
