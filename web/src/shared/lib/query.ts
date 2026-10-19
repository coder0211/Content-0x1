import { isApiError } from "@/shared/kernel";

/** Typed `meta` for TanStack Query (read by the global MutationCache in app/providers). */
declare module "@tanstack/react-query" {
  interface Register {
    mutationMeta: {
      /** Skip the automatic error toast (the component renders the error itself). */
      silentError?: boolean;
    };
  }
}

/** Retry transient failures only; never retry 4xx. */
export function shouldRetry(failureCount: number, error: unknown): boolean {
  if (isApiError(error) && error.status >= 400 && error.status < 500) return false;
  return failureCount < 2;
}
