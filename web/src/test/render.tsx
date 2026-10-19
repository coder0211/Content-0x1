import type { ReactElement } from "react";
import { render } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { AppProviders, createQueryClient } from "@/app/providers";
import { buildRoutes } from "@/app/router";
import type { Repositories } from "@/shared/lib/di";

/** Render the full app (real router + module registry) at `path`. Use MSW or `repositories` overrides for data. */
export function renderApp(path = "/", opts: { repositories?: Repositories } = {}) {
  const router = createMemoryRouter(buildRoutes(), { initialEntries: [path] });
  const queryClient = createQueryClient();
  const utils = render(
    <AppProviders repositories={opts.repositories} queryClient={queryClient}>
      <RouterProvider router={router} />
    </AppProviders>,
  );
  return { ...utils, router, queryClient };
}

/** Render a single component inside all providers (no router). */
export function renderWithProviders(ui: ReactElement, opts: { repositories?: Repositories } = {}) {
  return render(
    <AppProviders repositories={opts.repositories} queryClient={createQueryClient()}>
      {ui}
    </AppProviders>,
  );
}
