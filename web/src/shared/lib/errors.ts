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

/** Job/step `error_kind` → explanation (SPEC-05 §5). */
export function describeErrorKind(kind: ErrorKind | string | null | undefined, detail?: string): string {
  switch (kind) {
    case "ContentPolicy":
      return "Nội dung yêu cầu bị nhà cung cấp từ chối. Hãy chỉnh lại mô tả.";
    case "RateLimited":
      return "Hệ thống đang bận, sẽ tự thử lại.";
    case "Transient":
      return "Không kết nối được nhà cung cấp. Bấm Thử lại.";
    case "InvalidRequest":
      return `Tham số không hợp lệ: ${detail ?? "không rõ"}.`;
    case "BUDGET_EXCEEDED":
      return MESSAGES.BUDGET_EXCEEDED;
    default:
      return detail ? `Job lỗi: ${detail}` : "Job gặp lỗi không xác định. Bấm Thử lại hoặc báo IT.";
  }
}

/** Kinds where offering "Thử lại" makes sense. */
export function isRetryableErrorKind(kind: string | null | undefined): boolean {
  return kind === "Transient" || kind === "RateLimited";
}
