import { ApiError, type ApiErrorCode, type ApiErrorDetail, type Page } from "@/shared/kernel";
import { env } from "@/shared/lib/env";
import { uuid } from "@/shared/lib/id";

/**
 * Fetch wrapper for `/api/v1` (SPEC-04 §1):
 * - unwraps `{ data, meta }`, throws `ApiError` built from `{ error: { code, message, details } }`
 * - sends cookies, `X-Request-Id`, `X-CSRF-Token` on writes, optional `Idempotency-Key`
 * - notifies `onUnauthorized` listeners on 401 (auth module redirects to /login)
 * Only `infrastructure/` code may import this module.
 */

export interface ApiMeta {
  request_id: string | null;
  next_cursor?: string | null;
  [key: string]: unknown;
}

export interface ApiResponse<T> {
  data: T;
  meta: ApiMeta;
  status: number;
}

type QueryValue = string | number | boolean | null | undefined;

export interface RequestOptions {
  query?: Record<string, QueryValue | readonly QueryValue[]>;
  headers?: Record<string, string>;
  /** Required for every POST that creates an AI job (SPEC-04). Use `newIdempotencyKey()`. */
  idempotencyKey?: string;
  signal?: AbortSignal;
  /** Do not broadcast 401 (used by /me probe and /auth/login). */
  skipAuthRedirect?: boolean;
}

type Method = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

export const CSRF_COOKIE = "acs_csrf";
export const CSRF_HEADER = "X-CSRF-Token";

let csrfFallback: string | null = null;

/** Stores the `csrf_token` returned by `/auth/login`; used when the CSRF cookie is unavailable. */
export function setCsrfToken(token: string | null): void {
  csrfFallback = token;
}

function readCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const hit = document.cookie.split("; ").find((c) => c.startsWith(`${name}=`));
  return hit ? decodeURIComponent(hit.slice(name.length + 1)) : null;
}

export function getCsrfToken(): string | null {
  return readCookie(CSRF_COOKIE) ?? csrfFallback;
}

type UnauthorizedListener = (error: ApiError) => void;
const unauthorizedListeners = new Set<UnauthorizedListener>();

export function onUnauthorized(listener: UnauthorizedListener): () => void {
  unauthorizedListeners.add(listener);
  return () => unauthorizedListeners.delete(listener);
}

export function apiUrl(path: string, query?: RequestOptions["query"]): string {
  const base = env.apiBaseUrl.replace(/\/$/, "");
  const url = new URL(`${base}${path.startsWith("/") ? path : `/${path}`}`, window.location.origin);
  for (const [key, raw] of Object.entries(query ?? {})) {
    const values = Array.isArray(raw) ? raw : [raw];
    for (const v of values) {
      if (v !== undefined && v !== null && v !== "") url.searchParams.append(key, String(v));
    }
  }
  return url.toString();
}

const FALLBACK_CODES: Record<number, ApiErrorCode> = {
  400: "VALIDATION_FAILED",
  401: "UNAUTHENTICATED",
  403: "FORBIDDEN",
  404: "NOT_FOUND",
  413: "FILE_TOO_LARGE",
  415: "UNSUPPORTED_MEDIA_TYPE",
  423: "ACCOUNT_LOCKED",
  429: "RATE_LIMITED",
  502: "PROVIDER_ERROR",
  503: "SERVICE_UNAVAILABLE",
};

interface ErrorPayload {
  error?: { code?: string; message?: string; details?: ApiErrorDetail[] };
  meta?: { request_id?: string };
}

function isBodyInit(body: unknown): body is BodyInit {
  return body instanceof FormData || body instanceof Blob || body instanceof URLSearchParams;
}

export async function request<T>(
  method: Method,
  path: string,
  body?: unknown,
  opts: RequestOptions = {},
): Promise<ApiResponse<T>> {
  const sentRequestId = uuid();
  const headers: Record<string, string> = {
    Accept: "application/json",
    "X-Request-Id": sentRequestId,
    ...opts.headers,
  };
  let payload: BodyInit | undefined;
  if (body !== undefined) {
    if (isBodyInit(body)) {
      payload = body;
    } else {
      headers["Content-Type"] = "application/json";
      payload = JSON.stringify(body);
    }
  }
  if (method !== "GET") {
    const csrf = getCsrfToken();
    if (csrf) headers[CSRF_HEADER] = csrf;
  }
  if (opts.idempotencyKey) headers["Idempotency-Key"] = opts.idempotencyKey;

  let res: Response;
  try {
    res = await fetch(apiUrl(path, opts.query), {
      method,
      headers,
      body: payload,
      credentials: "include",
      signal: opts.signal,
    });
  } catch (cause) {
    const aborted = cause instanceof DOMException && cause.name === "AbortError";
    throw new ApiError({
      code: aborted ? "ABORTED" : "NETWORK_ERROR",
      message: aborted ? "Request aborted" : "Network error",
      status: 0,
      requestId: sentRequestId,
    });
  }

  const headerRequestId = res.headers.get("X-Request-Id") ?? sentRequestId;
  const text = res.status === 204 ? "" : await res.text();
  let json: unknown = null;
  if (text) {
    try {
      json = JSON.parse(text);
    } catch {
      json = null;
    }
  }

  if (!res.ok) {
    const p = (json ?? {}) as ErrorPayload;
    const error = new ApiError({
      code: p.error?.code ?? FALLBACK_CODES[res.status] ?? (res.status >= 500 ? "INTERNAL" : "UNKNOWN"),
      message: p.error?.message ?? res.statusText,
      status: res.status,
      details: p.error?.details ?? [],
      requestId: p.meta?.request_id ?? headerRequestId,
    });
    if (res.status === 401 && !opts.skipAuthRedirect) {
      unauthorizedListeners.forEach((l) => l(error));
    }
    throw error;
  }

  const envelope = (json ?? {}) as { data?: T; meta?: Partial<ApiMeta> };
  return {
    data: envelope.data as T,
    meta: { ...envelope.meta, request_id: envelope.meta?.request_id ?? headerRequestId },
    status: res.status,
  };
}

const data = <T>(p: Promise<ApiResponse<T>>): Promise<T> => p.then((r) => r.data);

export const http = {
  get: <T>(path: string, opts?: RequestOptions) => data(request<T>("GET", path, undefined, opts)),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) => data(request<T>("POST", path, body, opts)),
  put: <T>(path: string, body?: unknown, opts?: RequestOptions) => data(request<T>("PUT", path, body, opts)),
  patch: <T>(path: string, body?: unknown, opts?: RequestOptions) => data(request<T>("PATCH", path, body, opts)),
  delete: <T = void>(path: string, opts?: RequestOptions) => data(request<T>("DELETE", path, undefined, opts)),
  /** Cursor-paginated list: `?limit&cursor` → `{ items, nextCursor }`. */
  page: async <T>(path: string, opts?: RequestOptions): Promise<Page<T>> => {
    const res = await request<T[]>("GET", path, undefined, opts);
    return { items: res.data ?? [], nextCursor: res.meta.next_cursor ?? null };
  },
  request,
};
