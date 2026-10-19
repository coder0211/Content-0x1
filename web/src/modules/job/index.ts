import { lazyPage, type AppRoute } from "@/shared/lib/module";

export const routes: AppRoute[] = [
  {
    path: "jobs",
    lazy: lazyPage(() => import("./presentation/pages/JobsPage")),
    handle: { crumb: "Job" },
  },
];
