import { isApiError, type ApiErrorCode, type ApiErrorDetail, type ErrorKind } from "@/shared/kernel";

/** Vietnamese, user-facing copy per `error.code` (SPEC-04 §1.2, SPEC-05 §5–6). */
const MESSAGES: Record<ApiErrorCode, string> = {
  VALIDATION_FAILED: "Dữ liệu chưa hợp lệ. Kiểm tra lại các trường được đánh dấu.",
  UNAUTHENTICATED: "Bạn cần đăng nhập để tiếp tục.",
  SESSION_EXPIRED: "Phiên đăng nhập đã hết hạn. Đăng nhập lại để tiếp tục.",
  INVALID_CREDENTIALS: "Email hoặc mật khẩu không đúng.",
  FORBIDDEN: "Bạn không có quyền thực hiện thao tác này. Liên hệ Admin.",
  CSRF_INVALID: "Phiên làm việc không hợp lệ. Tải lại trang rồi thử lại.",
  SELF_APPROVAL_NOT_ALLOWED: "Bạn không thể tự duyệt asset do chính mình tạo.",
  NOT_FOUND: "Không tìm thấy dữ liệu. Có thể mục này đã bị xóa.",
  INVALID_STATE_TRANSITION: "Trạng thái đã thay đổi nên không thể thực hiện thao tác này. Tải lại để xem trạng thái mới.",
  IDEMPOTENCY_CONFLICT: "Yêu cầu này đang được xử lý. Chờ một lát rồi kiểm tra lại.",
  VERSION_CONFLICT: "Dữ liệu vừa được người khác cập nhật. Tải lại để xem phiên bản mới nhất.",
  FILE_TOO_LARGE: "File vượt dung lượng cho phép.",
  UNSUPPORTED_MEDIA_TYPE: "Định dạng file không được hỗ trợ.",
  CAPABILITY_NOT_SUPPORTED: "Model không hỗ trợ tham số đã chọn.",
  ACCOUNT_LOCKED: "Tài khoản tạm khóa do đăng nhập sai nhiều lần. Thử lại sau 15 phút.",
  RATE_LIMITED: "Hệ thống đang bận, thử lại sau ít phút.",
  BUDGET_EXCEEDED: "Đã hết ngân sách tháng.",
  JOB_BUDGET_EXCEEDED: "Dự toán vượt hạn mức mỗi job. Giảm khối lượng hoặc xin Marketing Lead duyệt.",
  CONCURRENCY_LIMIT: "Đang có quá nhiều job chạy cùng lúc. Thử lại khi job hiện tại xong.",
  PROVIDER_ERROR: "Không kết nối được nhà cung cấp. Bấm Thử lại.",
  SERVICE_UNAVAILABLE: "Hệ thống đang bảo trì hoặc quá tải. Thử lại sau ít phút.",
  INTERNAL: "Đã có lỗi xảy ra phía máy chủ.",
  ACCOUNT_DISABLED: "Tài khoản của bạn đã bị vô hiệu hóa. Liên hệ Admin.",
  CONFLICT: "Dữ liệu đã tồn tại hoặc bị trùng. Kiểm tra lại rồi thử lại.",
  IDEMPOTENCY_KEY_REQUIRED: "Yêu cầu thiếu mã chống gửi trùng. Tải lại trang rồi thử lại.",
  PRICING_MISSING: "Model này chưa có bảng giá nên chưa tính được dự toán. Liên hệ Admin.",
  METHOD_NOT_ALLOWED: "Thao tác này không được hỗ trợ.",
  MUSIC_NOT_LICENSED: "Nhạc nền chưa có giấy phép hợp lệ. Chọn bản nhạc khác.",
  VIDEO_JOB_IN_PROGRESS: "Video đang có job chạy. Chờ job hoàn tất rồi thử lại.",
  VIDEO_QUALITY_CHECK_FAILED: "Video render không đạt kiểm tra chất lượng. Bấm Thử lại.",
  JOB_CANCELLED: "Job đã bị hủy.",
  CANCELLED: "Job đã bị hủy.",
  STEP_FAILED: "Một bước của job bị lỗi. Bấm Thử lại.",
  STEP_TIMEOUT: "Một bước của job chạy quá lâu nên đã dừng. Bấm Thử lại.",
  LEASE_EXPIRED: "Job bị gián đoạn do máy chủ khởi động lại. Bấm Thử lại.",
  PROVIDER_WAIT_TIMEOUT: "Nhà cung cấp phản hồi quá lâu. Bấm Thử lại.",
  STEP_TYPE_UNKNOWN: "Job có bước không được hỗ trợ. Báo IT.",
  PROVIDER_CONTENT_POLICY: "Nội dung yêu cầu bị nhà cung cấp từ chối. Hãy chỉnh lại mô tả.",
  PROVIDER_RATE_LIMITED: "Nhà cung cấp đang giới hạn tần suất. Thử lại sau ít phút.",
  PROVIDER_TRANSIENT: "Không kết nối được nhà cung cấp. Bấm Thử lại.",
  PROVIDER_INVALID_REQUEST: "Nhà cung cấp không chấp nhận tham số đã chọn. Kiểm tra lại cấu hình.",
  PROVIDER_AUTH: "Không xác thực được với nhà cung cấp. Báo Admin kiểm tra API key.",
  PROVIDER_UNKNOWN: "Nhà cung cấp gặp lỗi không xác định. Bấm Thử lại.",
  NETWORK_ERROR: "Không kết nối được máy chủ. Kiểm tra mạng rồi thử lại.",
  ABORTED: "Yêu cầu đã bị hủy.",
  UNKNOWN: "Đã có lỗi xảy ra.",
};

function detailSuffix(code: string, details: ApiErrorDetail[]): string {
  if (code === "CAPABILITY_NOT_SUPPORTED") {
    const parts = details
      .filter((d) => d.field)
      .map((d) => (Array.isArray(d.supported) ? `${d.field} (hỗ trợ: ${d.supported.join(", ")})` : d.field));
    return parts.length ? ` ${parts.join("; ")}.` : "";
  }
  return "";
}

/** Message for a code; unknown codes fall back to the server message, then a generic line. */
export function messageForCode(code: string, details: ApiErrorDetail[] = [], serverMessage?: string): string {
  if (code === "BUDGET_EXCEEDED" && details.some((d) => d.scope === "campaign")) {
    return "Đã hết ngân sách campaign.";
  }
  const known = (MESSAGES as Record<string, string | undefined>)[code];
  if (known) return known + detailSuffix(code, details);
  return serverMessage || MESSAGES.UNKNOWN;
}

export interface ErrorDescription {
  code: string;
  message: string;
  requestId: string | null;
}

export function describeError(error: unknown): ErrorDescription {
  if (isApiError(error)) {
    return {
      code: error.code,
      message: messageForCode(error.code, error.details, error.message),
      requestId: error.requestId,
    };
  }
  return { code: "UNKNOWN", message: MESSAGES.UNKNOWN, requestId: null };
}

/**
 * Job/step failure → explanation (SPEC-05 §5). Accepts a step `error_kind` (ContentPolicy, Transient, …)
 * or a job `error.code` (PROVIDER_CONTENT_POLICY, STEP_TIMEOUT, BUDGET_EXCEEDED, …).
 */
export function describeErrorKind(kind: ErrorKind | string | null | undefined, detail?: string): string {
  switch (kind) {
    case "ContentPolicy":
    case "PROVIDER_CONTENT_POLICY":
      return "Nội dung yêu cầu bị nhà cung cấp từ chối. Hãy chỉnh lại mô tả.";
    case "RateLimited":
    case "PROVIDER_RATE_LIMITED":
      return "Hệ thống đang bận, sẽ tự thử lại.";
    case "Transient":
    case "PROVIDER_TRANSIENT":
      return "Không kết nối được nhà cung cấp. Bấm Thử lại.";
    case "InvalidRequest":
    case "PROVIDER_INVALID_REQUEST":
      return `Tham số không hợp lệ: ${detail ?? "không rõ"}.`;
    case "Auth":
    case "PROVIDER_AUTH":
      return MESSAGES.PROVIDER_AUTH;
    case "Timeout":
      return MESSAGES.STEP_TIMEOUT;
    case "LeaseExpired":
      return MESSAGES.LEASE_EXPIRED;
    default: {
      const known = kind ? (MESSAGES as Record<string, string | undefined>)[kind] : undefined;
      if (known) return known;
      return detail ? `Job lỗi: ${detail}` : "Job gặp lỗi không xác định. Bấm Thử lại hoặc báo IT.";
    }
  }
}

/** Kinds/codes where offering "Thử lại" makes sense (mirrors the API's retryable job codes). */
const RETRYABLE = new Set<string>([
  "Transient",
  "RateLimited",
  "Timeout",
  "LeaseExpired",
  "Internal",
  "STEP_FAILED",
  "STEP_TIMEOUT",
  "LEASE_EXPIRED",
  "PROVIDER_WAIT_TIMEOUT",
  "PROVIDER_TRANSIENT",
  "PROVIDER_RATE_LIMITED",
  "PROVIDER_UNKNOWN",
]);

export function isRetryableErrorKind(kind: string | null | undefined): boolean {
  return kind != null && RETRYABLE.has(kind);
}
