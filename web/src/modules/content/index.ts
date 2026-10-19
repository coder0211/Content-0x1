import { PenLine } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    path: "content",
    lazy: lazyPage(() => import("./presentation/pages/ContentStudioPage")),
    handle: { crumb: "Content Studio", permission: "generate.run" },
  },
];

export const navItems: NavItem[] = [
  { to: "/content", label: "Nội dung", icon: PenLine, group: "work", order: 30, permission: "generate.run" },
];
