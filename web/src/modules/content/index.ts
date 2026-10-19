import { PenLine } from "lucide-react";
import type { RepositoryFactories } from "@/shared/lib/di";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";
import { HttpContentRepository } from "./infrastructure/HttpContentRepository";

export type { ContentRepository } from "./application/ports";
export { videoStudioLink } from "./domain/content";

export const routes: AppRoute[] = [
  {
    path: "content",
    lazy: lazyPage(() => import("./presentation/pages/ContentStudioPage")),
    handle: { crumb: "Content Studio", permission: "generate.run" },
  },
];

export const navItems: NavItem[] = [
  {
    to: "/content",
    label: "Nội dung",
    icon: PenLine,
    group: "work",
    order: 30,
    permission: "generate.run",
  },
];

export const repositories: RepositoryFactories = {
  content: () => new HttpContentRepository(),
};
