import { Image } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    path: "images",
    lazy: lazyPage(() => import("./presentation/pages/ImageStudioPage")),
    handle: { crumb: "Image Studio", permission: "generate.run" },
  },
];

export const navItems: NavItem[] = [
  { to: "/images", label: "Hình ảnh", icon: Image, group: "work", order: 40, permission: "generate.run" },
];
