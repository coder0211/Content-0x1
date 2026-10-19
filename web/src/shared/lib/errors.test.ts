import { describe, expect, it } from "vitest";
import { ApiError } from "@/shared/kernel";
import { describeError, describeErrorKind, isRetryableErrorKind, messageForCode } from "./errors";

describe("messageForCode", () => {
  it.each([
    "MUSIC_NOT_LICENSED",
    "VIDEO_JOB_IN_PROGRESS",
    "VIDEO_QUALITY_CHECK_FAILED",
    "PRICING_MISSING",
    "JOB_BUDGET_EXCEEDED",
    "IDEMPOTENCY_CONFLICT",
    "IDEMPOTENCY_KEY_REQUIRED",
    "RATE_LIMITED",
    "VERSION_CONFLICT",
    "SELF_APPROVAL_NOT_ALLOWED",
    "ACCOUNT_DISABLED",
    "CONFLICT",
    "STEP_TIMEOUT",
  ])("maps %s to a Vietnamese message instead of the server text", (code) => {
    const msg = messageForCode(code, [], "server text");
    expect(msg).not.toBe("server text");
    expect(msg.length).toBeGreaterThan(5);
  });

  it("uses the campaign budget message when scope is campaign", () => {
    expect(messageForCode("BUDGET_EXCEEDED", [{ scope: "campaign" }])).toBe("Đã hết ngân sách campaign.");
  });

  it("appends supported values for CAPABILITY_NOT_SUPPORTED", () => {
    const msg = messageForCode("CAPABILITY_NOT_SUPPORTED", [{ field: "size", supported: ["1024x1024"] }]);
    expect(msg).toContain("size (hỗ trợ: 1024x1024)");
  });

  it("falls back to the server message, then a generic line, for unknown codes", () => {
    expect(messageForCode("SOMETHING_NEW", [], "Máy chủ nói vậy")).toBe("Máy chủ nói vậy");
    expect(messageForCode("SOMETHING_NEW")).toBe("Đã có lỗi xảy ra.");
  });
});

describe("describeError", () => {
  it("keeps code and request id from ApiError", () => {
    const err = new ApiError({ code: "VIDEO_JOB_IN_PROGRESS", status: 409, message: "x", requestId: "req_1" });
    expect(describeError(err)).toEqual({
      code: "VIDEO_JOB_IN_PROGRESS",
      message: messageForCode("VIDEO_JOB_IN_PROGRESS"),
      requestId: "req_1",
    });
  });

  it("returns UNKNOWN for non-API errors", () => {
    expect(describeError(new Error("boom")).code).toBe("UNKNOWN");
  });
});

describe("describeErrorKind", () => {
  it("explains provider auth failures", () => {
    expect(describeErrorKind("Auth")).toContain("API key");
  });
});

describe("job error codes", () => {
  it("explains provider job codes like their step kinds", () => {
    expect(describeErrorKind("PROVIDER_CONTENT_POLICY")).toBe(describeErrorKind("ContentPolicy"));
    expect(describeErrorKind("BUDGET_EXCEEDED")).toBe("Đã hết ngân sách tháng.");
    expect(describeErrorKind("MUSIC_NOT_LICENSED")).toContain("giấy phép");
    expect(describeErrorKind("SOMETHING", "chi tiết")).toBe("Job lỗi: chi tiết");
  });

  it("treats the API's retryable job codes as retryable", () => {
    expect(isRetryableErrorKind("STEP_TIMEOUT")).toBe(true);
    expect(isRetryableErrorKind("PROVIDER_RATE_LIMITED")).toBe(true);
    expect(isRetryableErrorKind("PROVIDER_CONTENT_POLICY")).toBe(false);
    expect(isRetryableErrorKind(null)).toBe(false);
  });
});
