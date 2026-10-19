import type { HttpHandler } from "msw";
import { api, ok, rest, withAuth } from "@/shared/api/mock";
import type { DashboardSummaryDto } from "./dto";

/** MSW handler for GET /dashboard/summary (api/openapi/dashboard.yaml). Widgets follow the mock user's permissions. */

const ago = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString();

/** Test hook: override the next responses (null → default seeded summary). */
let override: ((d: DashboardSummaryDto) => DashboardSummaryDto) | null = null;
export function setDashboardMock(fn: ((d: DashboardSummaryDto) => DashboardSummaryDto) | null): void {
  override = fn;
}

/** In-memory notifications of the mock session (GET /notifications, POST /notifications/{id}/read). */
const readIds = new Set<string>();
const mockNotifications = () => [
  {
    id: "40000000-0000-4000-8000-000000000001",
    type: "asset_approved",
    payload: { title: "Bài Facebook: Bánh kem Tết vị trà xanh", actor_name: "Người duyệt", asset_id: "50000000-0000-4000-8000-000000000001" },
    created_at: ago(15),
  },
].map((n) => ({ ...n, read_at: readIds.has(n.id) ? ago(1) : null }));

export const handlers: HttpHandler[] = [
  rest.get(api("/notifications"), withAuth(null, () => ok(mockNotifications()))),
  rest.post(
    api("/notifications/:id/read"),
    withAuth(null, ({ params }) => {
      readIds.add(String(params.id));
      return new Response(null, { status: 204 });
    }),
  ),
  rest.get(
    api("/dashboard/summary"),
    withAuth(null, ({ user }) => {
      const can = (p: string) => (user.permissions as string[]).includes(p);
      const base: DashboardSummaryDto = {
        campaigns: can("brand.view")
          ? {
              active_count: 2,
              recent: [
                { id: "20000000-0000-4000-8000-000000000001", name: "Tết 2027 – Bánh Kem Hương Việt", status: "in_progress", updated_at: ago(35) },
                { id: "20000000-0000-4000-8000-000000000002", name: "Back to school 2026", status: "review", updated_at: ago(60 * 26) },
              ],
            }
          : null,
        jobs: {
          scope: can("provider.view") ? "all" : "own",
          queued: 1,
          running: 1,
          failed_24h: 1,
          recent: [
            { id: "30000000-0000-4000-8000-0000000000a1", job_type: "video_production", status: "running", progress: 45, error_code: null, created_at: ago(4) },
            { id: "30000000-0000-4000-8000-0000000000a2", job_type: "image_generation", status: "failed", progress: 100, error_code: "PROVIDER_TIMEOUT", created_at: ago(90) },
          ],
        },
        reviews: {
          pending_review: can("asset.review") ? 3 : null,
          my_pending_review: can("generate.run") ? 2 : 0,
          my_changes_requested: can("generate.run") ? 1 : 0,
          my_drafts: can("generate.run") ? 4 : 0,
        },
        cost: can("usage.view") ? { currency: "USD", today: 3.42, month: 412.5, monthly_limit: 500, month_used_pct: 82.5 } : null,
        generated_at: new Date().toISOString(),
      };
      return ok(override ? override(base) : base);
    }),
  ),
];
