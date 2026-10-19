/**
 * MSW helpers for `modules/<m>/infrastructure/mocks.ts` (dev `pnpm dev:mock`, Vitest, Playwright).
 * Never imported by production code paths — only by mock handler files.
 */
import { http as mswHttp, HttpResponse, type HttpResponseResolver, type PathParams, type DefaultBodyType } from "msw";
import type { ApiErrorCode, ApiErrorDetail, Permission, Role } from "@/shared/kernel";
import { env } from "@/shared/lib/env";
import { uuid } from "@/shared/lib/id";
import type { SessionUser } from "@/shared/lib/session";

/** `/api/v1` + path, relative so MSW matches against the current origin. */
export const api = (path: string): string => `${env.apiBaseUrl.replace(/\/$/, "")}${path}`;

export function ok<T>(data: T, init: { status?: number; meta?: Record<string, unknown> } = {}) {
  return HttpResponse.json({ data, meta: { request_id: uuid(), ...init.meta } }, { status: init.status ?? 200 });
}

export function accepted<T>(data: T) {
  return ok(data, { status: 202 });
}

export function page<T>(items: T[], nextCursor: string | null = null) {
  return ok(items, { meta: { next_cursor: nextCursor } });
}

export function noContent() {
  return new HttpResponse(null, { status: 204 });
}

export function fail(status: number, code: ApiErrorCode | (string & {}), message = code, details: ApiErrorDetail[] = []) {
  return HttpResponse.json({ error: { code, message, details }, meta: { request_id: uuid() } }, { status });
}

/** SPEC-01 §2 role → permission matrix (mock only; the backend is the authority). */
export const ROLE_PERMISSIONS: Record<Role, Permission[]> = {
  admin: [
    "user.manage",
    "provider.manage",
    "provider.view",
    "budget.manage",
    "brand.manage",
    "brand.view",
    "campaign.manage",
    "generate.run",
    "asset.review",
    "asset.download",
    "usage.view",
    "audit.view",
  ],
  marketing_lead: [
    "provider.view",
    "budget.manage",
    "brand.manage",
    "brand.view",
    "campaign.manage",
    "generate.run",
    "asset.review",
    "asset.download",
    "usage.view",
  ],
  content_creator: ["brand.view", "campaign.manage", "generate.run", "asset.download"],
  reviewer: ["brand.view", "asset.review", "asset.download"],
};

function mockUser(id: string, email: string, name: string, roles: Role[]): SessionUser {
  const permissions = [...new Set(roles.flatMap((r) => ROLE_PERMISSIONS[r]))];
  return { id, email, name, roles, permissions };
}

/** Seed accounts. Password for all: `matkhau123`. */
export const MOCK_PASSWORD = "matkhau123";
export const MOCK_USERS: SessionUser[] = [
  mockUser("00000000-0000-4000-8000-000000000001", "admin@acs.local", "Nguyễn Quản Trị", ["admin"]),
  mockUser("00000000-0000-4000-8000-000000000002", "lead@acs.local", "Trần Thị Lan", ["marketing_lead"]),
  mockUser("00000000-0000-4000-8000-000000000003", "creator@acs.local", "Lê Minh Khoa", ["content_creator"]),
  mockUser("00000000-0000-4000-8000-000000000004", "reviewer@acs.local", "Phạm Thu Hà", ["reviewer"]),
];

const STORAGE_KEY = "acs.mock.session";
let currentUserId: string | null = null;

function storage(): Storage | null {
  try {
    return typeof window !== "undefined" ? window.sessionStorage : null;
  } catch {
    return null;
  }
}

/** Mock session shared by every module's handlers (persisted per tab in dev). */
export const mockSession = {
  get(): SessionUser | null {
    const id = currentUserId ?? storage()?.getItem(STORAGE_KEY) ?? null;
    return MOCK_USERS.find((u) => u.id === id) ?? null;
  },
  set(user: SessionUser | null): void {
    currentUserId = user?.id ?? null;
    const s = storage();
    if (!s) return;
    if (user) s.setItem(STORAGE_KEY, user.id);
    else s.removeItem(STORAGE_KEY);
  },
  signInAs(email: string): SessionUser {
    const user = MOCK_USERS.find((u) => u.email === email);
    if (!user) throw new Error(`Unknown mock user ${email}`);
    this.set(user);
    return user;
  },
  reset(): void {
    this.set(null);
  },
};

type Resolver<P extends PathParams> = (
  info: Parameters<HttpResponseResolver<P, DefaultBodyType>>[0] & { user: SessionUser },
) => ReturnType<HttpResponseResolver<P, DefaultBodyType>>;

/**
 * Wrap a resolver with auth + permission checks that mirror the backend:
 *   withAuth("generate.run", async ({ request, user }) => accepted({...}))
 */
export function withAuth<P extends PathParams = PathParams>(
  permission: Permission | null,
  resolver: Resolver<P>,
): HttpResponseResolver<P, DefaultBodyType> {
  return (info) => {
    const user = mockSession.get();
    if (!user) return fail(401, "UNAUTHENTICATED", "Chưa đăng nhập");
    if (permission && !user.permissions.includes(permission)) return fail(403, "FORBIDDEN", "Thiếu quyền");
    return resolver({ ...info, user });
  };
}

export { mswHttp as rest, HttpResponse };
