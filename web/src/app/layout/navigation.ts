import type { AppModule, NavGroup, NavItem } from "@/shared/lib/module";
import type { Session } from "@/shared/lib/session";

export const NAV_GROUPS: { id: NavGroup; label: string }[] = [
  { id: "work", label: "Làm việc" },
  { id: "manage", label: "Quản lý" },
  { id: "system", label: "Hệ thống" },
];

/** Nav items the user may see, grouped and ordered (SPEC-05 §4.1). */
export function visibleNav(modules: readonly AppModule[], can: Session["can"]): { id: NavGroup; label: string; items: NavItem[] }[] {
  const items = modules.flatMap((m) => m.navItems ?? []).filter((i) => can(i.permission));
  return NAV_GROUPS.map((g) => ({
    ...g,
    items: items.filter((i) => i.group === g.id).sort((a, b) => a.order - b.order),
  })).filter((g) => g.items.length > 0);
}
