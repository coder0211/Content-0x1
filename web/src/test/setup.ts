import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { server } from "@/app/mocks/server";
import { resetAuthMocks } from "@/modules/auth/infrastructure/mocks";
import { mockSession } from "@/shared/api/mock";
import { setCsrfToken } from "@/shared/api/http";

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
  mockSession.reset();
  resetAuthMocks();
  setCsrfToken(null);
  window.localStorage.clear();
  window.sessionStorage.clear();
});
afterAll(() => server.close());
