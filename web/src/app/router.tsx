import { createBrowserRouter, type RouteObject } from "react-router";
import { RequireAuth } from "@/modules/auth";
import type { AppModule, AppRoute } from "@/shared/lib/module";
import { lazyPage } from "@/shared/lib/module";
import { AppShell } from "./layout/AppShell";
import { appModules } from "./modules";
import { NotFoundPage } from "./pages/NotFoundPage";
import { RouteErrorPage } from "./pages/RouteErrorPage";

/** Builds the route tree from the module registry. Shared by the browser router and tests. */
export function buildRoutes(modules: readonly AppModule[] = appModules): RouteObject[] {
  const publicRoutes = modules.flatMap((m) => m.publicRoutes ?? []);
  const shellRoutes: AppRoute[] = modules.flatMap((m) => m.routes ?? []);
  return [
    {
      path: "/",
      errorElement: <RouteErrorPage />,
      children: [
        ...(publicRoutes as RouteObject[]),
        { path: "_design", lazy: lazyPage(() => import("./pages/DesignPage")) },
        {
          element: (
            <RequireAuth>
              <AppShell />
            </RequireAuth>
          ),
          errorElement: <RouteErrorPage />,
          children: [...(shellRoutes as RouteObject[]), { path: "*", element: <NotFoundPage /> }],
        },
      ],
    },
  ];
}

export function createAppRouter() {
  return createBrowserRouter(buildRoutes());
}
