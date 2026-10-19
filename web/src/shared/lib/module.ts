import type { ComponentType } from "react";
import type { RouteObject } from "react-router";
import type { LucideIcon } from "lucide-react";
import type { PermissionRequirement } from "@/shared/kernel";
import type { RepositoryFactories } from "./di";

/** Put on every route's `handle`. The app shell renders breadcrumbs and enforces `permission`. */
export interface RouteHandle {
  crumb?: string;
  permission?: PermissionRequirement;
}

export type AppRoute = Omit<RouteObject, "handle" | "children"> & {
  handle?: RouteHandle;
  children?: AppRoute[];
};

export type NavGroup = "work" | "manage" | "system";

export interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  group: NavGroup;
  /** Sort order inside the group (ascending). */
  order: number;
  permission?: PermissionRequirement;
  /** Optional count badge, e.g. Review queue size. Rendered as a component so it may use hooks. */
  Badge?: ComponentType;
}

/** Shape of every `modules/<name>/index.ts` as consumed by `app/modules.ts`. */
export interface AppModule {
  /** Authenticated routes rendered inside the app shell. Paths are relative (no leading slash). */
  routes?: AppRoute[];
  /** Routes rendered outside the shell without auth (login only). */
  publicRoutes?: AppRoute[];
  navItems?: NavItem[];
  repositories?: RepositoryFactories;
}

/** Route `lazy` loader for a page module with a default export — keeps each page in its own chunk. */
export function lazyPage(load: () => Promise<{ default: ComponentType }>): NonNullable<RouteObject["lazy"]> {
  return async () => ({ Component: (await load()).default });
}
