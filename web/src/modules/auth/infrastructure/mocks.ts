import type { HttpHandler } from "msw";
import { api, fail, MOCK_PASSWORD, MOCK_USERS, mockSession, noContent, ok, rest } from "@/shared/api/mock";
import type { LoginResponseDto, MeDto } from "./dto";

const FAILED_LIMIT = 5;
const failures = new Map<string, number>();

export const handlers: HttpHandler[] = [
  rest.post(api("/auth/login"), async ({ request }) => {
    const body = (await request.json()) as { email?: string; password?: string };
    const email = (body.email ?? "").toLowerCase();
    if (!email || !body.password) {
      return fail(400, "VALIDATION_FAILED", "Dữ liệu không hợp lệ", [
        ...(!email ? [{ field: "email", issue: "required" }] : []),
        ...(!body.password ? [{ field: "password", issue: "required" }] : []),
      ]);
    }
    if ((failures.get(email) ?? 0) >= FAILED_LIMIT) return fail(423, "ACCOUNT_LOCKED", "Tài khoản tạm khóa");
    const user = MOCK_USERS.find((u) => u.email === email);
    if (!user || body.password !== MOCK_PASSWORD) {
      failures.set(email, (failures.get(email) ?? 0) + 1);
      return fail(401, "INVALID_CREDENTIALS", "Sai email hoặc mật khẩu");
    }
    failures.delete(email);
    mockSession.set(user);
    return ok<LoginResponseDto>({ user, csrf_token: "mock-csrf-token" });
  }),

  rest.post(api("/auth/logout"), () => {
    mockSession.reset();
    return noContent();
  }),

  rest.get(api("/me"), () => {
    const user = mockSession.get();
    return user ? ok<MeDto>(user) : fail(401, "UNAUTHENTICATED", "Chưa đăng nhập");
  }),
];

/** Test helper: clear lockout counters between tests. */
export function resetAuthMocks(): void {
  failures.clear();
}
