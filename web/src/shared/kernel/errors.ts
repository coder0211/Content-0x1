/** Error codes from SPEC-04 §1.2 plus client-side codes. FE branches on `code`, never on `message`. */
export type ApiErrorCode =
  | "VALIDATION_FAILED"
  | "UNAUTHENTICATED"
  | "SESSION_EXPIRED"
  | "INVALID_CREDENTIALS"
  | "FORBIDDEN"
  | "CSRF_INVALID"
  | "SELF_APPROVAL_NOT_ALLOWED"
  | "NOT_FOUND"
  | "INVALID_STATE_TRANSITION"
  | "IDEMPOTENCY_CONFLICT"
  | "VERSION_CONFLICT"
  | "FILE_TOO_LARGE"
  | "UNSUPPORTED_MEDIA_TYPE"
  | "CAPABILITY_NOT_SUPPORTED"
  | "ACCOUNT_LOCKED"
  | "RATE_LIMITED"
  | "BUDGET_EXCEEDED"
  | "JOB_BUDGET_EXCEEDED"
  | "CONCURRENCY_LIMIT"
  | "PROVIDER_ERROR"
  | "SERVICE_UNAVAILABLE"
  | "INTERNAL"
  // client-side only
  | "NETWORK_ERROR"
  | "ABORTED"
  | "UNKNOWN";

export interface ApiErrorDetail {
  field?: string;
  issue?: string;
  [key: string]: unknown;
}

/** Typed error thrown by `shared/api/http`. Lives in kernel so every layer may inspect it. */
export class ApiError extends Error {
  override readonly name = "ApiError";
  readonly code: ApiErrorCode | (string & {});
  readonly status: number;
  readonly details: ApiErrorDetail[];
  readonly requestId: string | null;

  constructor(init: {
    code: ApiErrorCode | (string & {});
    message: string;
    status: number;
    details?: ApiErrorDetail[];
    requestId?: string | null;
  }) {
    super(init.message);
    this.code = init.code;
    this.status = init.status;
    this.details = init.details ?? [];
    this.requestId = init.requestId ?? null;
  }

  is(...codes: ApiErrorCode[]): boolean {
    return (codes as string[]).includes(this.code);
  }

  /** Field-level errors keyed by `details[].field` (for forms). */
  fieldErrors(): Record<string, string> {
    const out: Record<string, string> = {};
    for (const d of this.details) {
      if (d.field && d.issue && !(d.field in out)) out[d.field] = d.issue;
    }
    return out;
  }
}

export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError;
}
