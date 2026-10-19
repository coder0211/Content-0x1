import { http as mswHttp, HttpResponse } from "msw";
import { server } from "@/app/mocks/server";
import { ApiError } from "@/shared/kernel";
import { api, fail, ok } from "./mock";
import { http, onUnauthorized, setCsrfToken } from "./http";

describe("http", () => {
  it("unwraps the data envelope", async () => {
    server.use(mswHttp.get(api("/things"), () => ok({ id: "1" })));
    await expect(http.get("/things")).resolves.toEqual({ id: "1" });
  });

  it("returns cursor pages", async () => {
    server.use(mswHttp.get(api("/things"), () => HttpResponse.json({ data: [1, 2], meta: { request_id: "r", next_cursor: "c2" } })));
    await expect(http.page<number>("/things", { query: { limit: 2 } })).resolves.toEqual({ items: [1, 2], nextCursor: "c2" });
  });

  it("throws typed ApiError with code, details and request_id", async () => {
    server.use(
      mswHttp.post(api("/things"), () =>
        HttpResponse.json(
          { error: { code: "VALIDATION_FAILED", message: "bad", details: [{ field: "name", issue: "required" }] }, meta: { request_id: "req-1" } },
          { status: 400 },
        ),
      ),
    );
    const err = await http.post("/things", {}).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    const e = err as ApiError;
    expect(e.code).toBe("VALIDATION_FAILED");
    expect(e.status).toBe(400);
    expect(e.requestId).toBe("req-1");
    expect(e.fieldErrors()).toEqual({ name: "required" });
  });

  it("sends CSRF, request id and idempotency headers on writes only", async () => {
    setCsrfToken("tok");
    const seen: Record<string, string | null>[] = [];
    server.use(
      mswHttp.all(api("/echo"), ({ request }) => {
        seen.push({
          csrf: request.headers.get("X-CSRF-Token"),
          rid: request.headers.get("X-Request-Id"),
          idem: request.headers.get("Idempotency-Key"),
        });
        return ok(null);
      }),
    );
    await http.get("/echo");
    await http.post("/echo", { a: 1 }, { idempotencyKey: "k-1" });
    expect(seen[0]?.csrf).toBeNull();
    expect(seen[0]?.rid).toBeTruthy();
    expect(seen[1]).toMatchObject({ csrf: "tok", idem: "k-1" });
  });

  it("notifies 401 listeners unless skipAuthRedirect", async () => {
    server.use(mswHttp.get(api("/secret"), () => fail(401, "SESSION_EXPIRED")));
    const listener = vi.fn();
    const off = onUnauthorized(listener);
    await expect(http.get("/secret")).rejects.toMatchObject({ code: "SESSION_EXPIRED" });
    await expect(http.get("/secret", { skipAuthRedirect: true })).rejects.toBeInstanceOf(ApiError);
    off();
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("maps network failures to NETWORK_ERROR", async () => {
    server.use(mswHttp.get(api("/down"), () => HttpResponse.error()));
    await expect(http.get("/down")).rejects.toMatchObject({ code: "NETWORK_ERROR", status: 0 });
  });
});
