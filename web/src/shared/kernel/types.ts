/** Opaque UUID string. Kept as a plain alias so DTOs map without ceremony. */
export type ID = string;

/** ISO-8601 UTC timestamp as returned by the API. */
export type ISODateTime = string;

export type Currency = "USD" | "VND";

export interface Money {
  amount: number;
  currency: Currency;
}

/** Cost as reported by the API (`estimated_cost`, `cost`). */
export interface Cost extends Money {
  isEstimate: boolean;
}

export interface Page<T> {
  items: T[];
  nextCursor: string | null;
}

export type Result<T, E = Error> = { ok: true; value: T } | { ok: false; error: E };

export const ok = <T>(value: T): Result<T, never> => ({ ok: true, value });
export const err = <E>(error: E): Result<never, E> => ({ ok: false, error });
