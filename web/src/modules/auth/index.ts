import { lazyPage, type AppRoute } from "@/shared/lib/module";
import type { RepositoryFactories } from "@/shared/lib/di";
import { HttpAuthRepository } from "./infrastructure/HttpAuthRepository";

export { AuthProvider } from "./application/AuthProvider";
export { RequireAuth } from "./presentation/components/RequireAuth";
export type { AuthRepository } from "./application/ports";

export const publicRoutes: AppRoute[] = [
  { path: "login", lazy: lazyPage(() => import("./presentation/pages/LoginPage")), handle: { crumb: "Đăng nhập" } },
];

export const repositories: RepositoryFactories = {
  auth: () => new HttpAuthRepository(),
};
