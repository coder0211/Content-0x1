import { safeRedirectTarget, validateCredentials } from "./user";

describe("validateCredentials", () => {
  it("requires email and password", () => {
    expect(validateCredentials({ email: "", password: "" })).toEqual({ email: "Nhập email.", password: "Nhập mật khẩu." });
  });
  it("rejects malformed email", () => {
    expect(validateCredentials({ email: "abc", password: "x" }).email).toBe("Email không hợp lệ.");
  });
  it("accepts valid input", () => {
    expect(validateCredentials({ email: "a@b.vn", password: "x" })).toEqual({});
  });
});

describe("safeRedirectTarget", () => {
  it.each([
    [null, "/"],
    ["/campaigns?x=1", "/campaigns?x=1"],
    ["//evil.com", "/"],
    ["https://evil.com", "/"],
    ["/login?next=/", "/"],
  ])("%s → %s", (input, expected) => {
    expect(safeRedirectTarget(input)).toBe(expected);
  });
});
