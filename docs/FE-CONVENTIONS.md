# FE-CONVENTIONS — rules for feature work in `web/`

Binding for every agent and developer building a feature module. Sources: SPEC-05 (design), SPEC-06 §3 (structure), SPEC-04 (API). If something here conflicts with a spec, the spec wins; record the deviation in `docs/DECISIONS.md`.

## 0. Commands

```bash
cd web
pnpm install
pnpm dev            # real API via proxy /api → VITE_API_PROXY_TARGET (default http://localhost:8080)
pnpm dev:mock       # whole UI on MSW, no backend. Accounts: admin|lead|creator|reviewer@acs.local / matkhau123
pnpm lint           # ESLint (boundaries + SPEC-05 forbidden classes) + CSS check
pnpm typecheck
pnpm test           # Vitest + Testing Library + MSW (node)
pnpm build
pnpm e2e            # Playwright against `vite --mode mock` (first time: pnpm exec playwright install chromium)
pnpm gen:api        # regenerate src/shared/api/schema.d.ts from ../api/openapi*.yaml (no-op if absent)
```

All of `lint`, `typecheck`, `test`, and `build` must pass before you hand off.

## 1. Ownership and merge-conflict rules

- You own **only** `web/src/modules/<your-module>/**` (plus `web/e2e/<your-module>*.spec.ts`).
- **Do not edit** `src/app/**`, `src/shared/**`, `package.json`, `pnpm-lock.yaml`, or any other module. The registries (`app/modules.ts`, `app/mocks/handlers.ts`) already contain one line per module. Your module plugs in through its own `index.ts` and `infrastructure/mocks.ts`.
- Need something in `shared/` (a new UI primitive, kernel type, formatter)? First build it locally inside your module (`presentation/components`). Then note it in `docs/DECISIONS.md` (under your agent heading) so the orchestrator can promote it later.
- **No new dependencies.** These are preinstalled: react-hook-form, zod, @hookform/resolvers, zustand, @dnd-kit/{core,sortable,utilities}, all Radix primitives in use, lucide-react, sonner. If you truly need another package, write it in DECISIONS and ask the orchestrator; do not touch the lockfile.

## 2. Module layout (SPEC-06 §3.1)

```
src/modules/<m>/
├── domain/            pure TS: types, rules, state helpers (no React, no fetch). Imports: shared/kernel only
├── application/       ports.ts (repository interface + DI registration), keys.ts (query keys), use-case hooks useXxx.ts
├── infrastructure/    HttpXxxRepository.ts, dto.ts (snake_case DTO + mappers), mocks.ts (MSW handlers)
├── presentation/
│   ├── pages/         one default-exported page per route (lazy-loaded)
│   └── components/
└── index.ts           public API: routes, navItems?, repositories?, and hooks/types other modules may use
```

Import rules are enforced by `eslint-plugin-boundaries` (`pnpm lint` fails otherwise):

| From             | May import                                                                                        |
| ---------------- | ------------------------------------------------------------------------------------------------- |
| `domain`         | `shared/kernel`, own `domain`                                                                     |
| `application`    | `shared/kernel`, `shared/lib`, own `domain`/`application`, **other modules' `index.ts`**           |
| `infrastructure` | `shared/kernel`, `shared/lib`, `shared/api`, own `domain`/`application`/`infrastructure`          |
| `presentation`   | `shared/kernel`, `shared/lib`, `shared/ui`, own `domain`/`application`/`presentation`, **other modules' `index.ts`** |
| `index.ts`       | own layers + `shared/kernel`, `shared/lib`                                                        |

- Components and hooks never call `fetch`, `EventSource`, or `shared/api`. Only `infrastructure/` does.
- Cross-module access goes **only** through `@/modules/<x>` (its `index.ts`), never through deep paths.
- Use the `@/` alias for every cross-folder import.
- Test files (`*.test.ts[x]`) are exempt from boundaries.

## 3. Routes and navigation

`index.ts` exports (typed by `AppModule` in `shared/lib/module.ts`):

```ts
import { Megaphone } from "lucide-react";
import { lazyPage, type AppRoute, type NavItem } from "@/shared/lib/module";
import type { RepositoryFactories } from "@/shared/lib/di";
import { HttpCampaignRepository } from "./infrastructure/HttpCampaignRepository";

export const routes: AppRoute[] = [
  {
    path: "campaigns",                         // relative, no leading slash; rendered inside the app shell
    lazy: lazyPage(() => import("./presentation/pages/CampaignsPage")),
    handle: { crumb: "Campaign", permission: "brand.view" },
  },
  {
    path: "campaigns/:campaignId",
    lazy: lazyPage(() => import("./presentation/pages/CampaignDetailPage")),
    handle: { crumb: "Chi tiết", permission: "brand.view" },
  },
];

export const navItems: NavItem[] = [
  { to: "/campaigns", label: "Campaign", icon: Megaphone, group: "work", order: 20, permission: "brand.view" },
];

export const repositories: RepositoryFactories = { campaign: () => new HttpCampaignRepository() };
```

- `handle.permission` is enforced by the shell (`app/layout/RouteGuard.tsx`). A user without it sees the `NoPermission` state; routes are never hidden silently. The value can be one code, an array (all required), or `{ anyOf: [...] }`.
- `handle.crumb` feeds the Topbar breadcrumb. Nested routes (`children`) are allowed.
- Pre-assigned paths: dashboard `/` · campaign `/campaigns` · content `/content` · image `/images` · video `/videos` · library `/library` · review `/review` · brand `/brands` · budget `/usage` · settings `/settings` · job `/jobs`. Keep your module's URLs under its prefix.
- To show a nav badge (e.g. the Review count), set `Badge: ReviewCountBadge` on the nav item. It is a component, so it may use hooks.

## 4. DI: repositories

```ts
// application/ports.ts
export interface CampaignRepository {
  list(q: CampaignQuery): Promise<Page<Campaign>>;
  create(input: CreateCampaignInput, idempotencyKey: string): Promise<Campaign>;
}
declare module "@/shared/lib/di" {
  interface RepositoryRegistry { campaign: CampaignRepository }   // registry key = module name
}

// application/useCampaigns.ts
export function useCampaigns(q: CampaignQuery) {
  const repo = useRepository("campaign");
  return useQuery({ queryKey: campaignKeys.list(q), queryFn: () => repo.list(q) });
}
```

- Tests swap implementations with `renderApp(path, { repositories: { campaign: new FakeCampaignRepository() } })` or `renderWithProviders(ui, { repositories })` from `src/test/render.tsx`.
- Query keys live in `application/keys.ts` as `["<module>", ...]` and are never shared across modules.

## 5. Calling the API (infrastructure only)

```ts
import { http } from "@/shared/api/http";
import { newIdempotencyKey } from "@/shared/lib/id";

await http.get<CampaignDto>(`/campaigns/${id}`);                      // returns `data` (envelope unwrapped)
await http.page<CampaignDto>("/campaigns", { query: { status, limit: 20, cursor } }); // { items, nextCursor }
await http.post<JobAcceptedDto>("/content/generations", dto, { idempotencyKey: key });
```

- Paths are relative to `/api/v1`. Request and response DTOs stay `snake_case` in `dto.ts`; map them to camelCase domain types in mappers there.
- Every POST that creates an AI job **must** send `idempotencyKey`. Create one key per user intent (`newIdempotencyKey()` in the mutation's `mutationFn` caller) and reuse it on retry.
- Errors are thrown as `ApiError` (`@/shared/kernel`) with `code`, `status`, `details`, and `requestId`. Branch on `error.code`, never on `message`. Use `error.fieldErrors()` for forms.
- CSRF (`X-CSRF-Token` from the `acs_csrf` cookie), `X-Request-Id`, cookies, and 401 → login redirect are automatic.
- Types: once `api/openapi.yaml` exists, run `pnpm gen:api` and type DTOs from `components["schemas"][...]` in `@/shared/api/schema`. Until then, hand-write DTOs that follow SPEC-04.
- SSE (job progress): `openEventStream(\`/jobs/${id}/events\`, { events: { status, step, done }, fallbackPoll })` from `@/shared/api/sse`. It falls back to polling every 3 s on its own. The `job` module should expose `useJobProgress(jobId)` from its `index.ts` for other modules.

## 6. Errors, toasts, and session

- **Mutations**: a failed mutation triggers a toast automatically (mapped from `error.code` → Vietnamese, `shared/lib/errors.ts`). If you render the error yourself, opt out with `useMutation({ ..., meta: { silentError: true } })`.
- **Queries**: no toast. Render `<ErrorState error={q.error} onRetry={q.refetch} />` in place (it shows `request_id`).
- Success toasts: `toast.success("Đã lưu phiên bản 4", { action: { label: "Xem", onClick } })` from `@/shared/ui/toast` (presentation only).
- Job failures: `describeErrorKind(error_kind, detail)` from `@/shared/lib/errors`.
- Session: `useSession()`, `useCan("usage.view")`, and `<Can permission="generate.run">…</Can>` from `@/shared/lib/session`. Hide actions the user cannot perform. The backend still enforces everything.

## 7. UI rules (SPEC-05) — read the spec; these are the essentials

- Use components from `@/shared/ui/*` and patterns from `@/shared/ui/patterns`. Open `/_design` (`pnpm dev:mock` → http://localhost:5173/_design) to see every token and component.
- Patterns: `PageHeader`, `DataTable` (loading/empty/error/row actions built in), `FormField`, `ConfirmDialog`, `StatusBadge kind=… status=…`, `CostChip amount estimate`, `JobProgress`, `AIProvenance`, `EmptyState`, `ErrorState`, `NoPermission`. Primitives: `Button` (primary|secondary|ghost|danger; sizes sm|md|lg|icon), `Input`, `Textarea`, `Select`, `Checkbox`, `Switch`, `RadioGroup`, `Tabs`, `Dialog`, `Sheet` (480px drawer), `Tooltip`, `DropdownMenu`, `Popover`, `Badge`, `Banner`, `Skeleton`, `Avatar`, `Kbd`.
- Tokens only. Colors are `neutral-{0..900}`, `accent-{50,100,600,700}`, and `success|warning|danger|info|muted` (+`-bg`, `-border`), plus `video-bg/fg` for the player area. Text sizes are `text-page|section|body|small|caption|editor`. Weights are `font-normal|medium|semibold`. Radius is `rounded-sm|rounded|rounded-lg`. Shadows are `shadow-sm` (popover) and `shadow-md` (dialog). Other Tailwind palettes and sizes **do not exist** (the theme is replaced), and `pnpm lint` rejects gradients, blur, `rounded-2xl+`, `shadow-lg+`, `text-sm/lg/xl…`, `font-bold`, and `rounded-full` outside `shared/ui`.
- At most one `primary` button per area. Button labels are verb + object ("Viết 3 phương án", "Gửi duyệt"), never "OK"/"Submit"/"Generate with AI". No sparkle, robot, or wand icons and no emoji. Icons come from Lucide at 16px (20px in headers/empty states); see the SPEC-05 §2.4 icon list.
- Numbers, money, and dates go **only** through `@/shared/lib/format`: `formatUsd` (`$0.42`, `< $0.01`, `≈ $0.42`), `formatVnd`, `formatDate`, `formatTime`, `formatDateTime`, `formatRelative`, `formatDuration`, `formatEta`, `formatFileSize`. Add the `tabular` class to numeric cells (`DataTable` does this for `align: "right"`).
- Tables beat card grids for data. Cards are only for media assets. Never nest cards.
- Copy: Vietnamese, short, address the user as "bạn"; follow the SPEC-05 §6 glossary (campaign, brief, asset, phiên bản, cảnh, duyệt, yêu cầu sửa, dự toán, chi phí thực tế).
- Desktop only, ≥ 1280px. Studio layout: 320px brief | flexible canvas (`bg-neutral-100`) | 300px inspector.

## 8. Required screen states (SPEC-05 §5) — every page

| State | Implementation |
| --- | --- |
| Loading | Shape-matching `Skeleton` / `DataTable isLoading`; never block the whole page |
| Empty | `EmptyState` with one sentence + one action ("Chưa có campaign nào." + "Tạo campaign") |
| Load error | `ErrorState` with retry + request_id |
| No permission | Route `handle.permission` (automatic) or `<NoPermission/>` for in-page sections |
| Job running | `JobProgress` with concrete step text + ETA + "Hủy"; the user can leave the page; toast on completion |
| Job failed | `JobProgress status="failed" errorKind=…` + "Thử lại" |
| Over budget | `<Banner tone="warning">Dự toán $3.20 vượt hạn mức mỗi job $2.00. …</Banner>` at the generate area |

Each UI hand-off must include screenshots of these states for every new page. Produce them with Playwright in mock mode (`page.screenshot`) and save them to `web/e2e/screenshots/<module>/` (gitignored if large; otherwise attach to the PR).

## 9. Mocks (MSW)

- `infrastructure/mocks.ts` exports `handlers: HttpHandler[]`. It is already registered in `app/mocks/handlers.ts` and is used by `pnpm dev:mock`, Vitest, and Playwright.
- Use the helpers in `@/shared/api/mock`: `rest.get(api("/campaigns"), withAuth("brand.view", ({ request, params, user }) => page(items, null)))`, `ok`, `accepted` (202), `noContent`, `fail(status, code, message, details)`, `mockSession`, `MOCK_USERS`, `ROLE_PERMISSIONS`.
- Mocks must mirror SPEC-04 exactly: snake_case, the envelope, the documented status codes and error codes, and permission checks via `withAuth`.
- Keep mock state in module-level in-memory stores inside `mocks.ts`, and export a `reset<M>Mocks()` used by your tests in `afterEach`.

## 10. Testing

| Layer | What | How |
| --- | --- | --- |
| domain | every rule and state helper | plain Vitest |
| application | hooks with fake repositories | `renderHook` + `renderWithProviders`-style wrapper, `repositories` override |
| infrastructure | repository ↔ MSW contract, mappers | Vitest + `server.use(...)` from `@/app/mocks/server` |
| presentation | pages and components with logic; all §8 states | Testing Library + `renderApp("/path")` (full router + MSW) |
| e2e | the main happy path of your module | `web/e2e/<module>.spec.ts`, mock mode |

- MSW runs with `onUnhandledRequest: "error"`, so every endpoint your page calls needs a handler.
- Sign in for tests with `mockSession.signInAs("creator@acs.local")` before `renderApp`.
- Query by role and accessible name (Vietnamese labels), not test ids.
