import type { ID, ISODateTime } from "@/shared/kernel";

export interface AuditEntry {
  id: number;
  actorId: ID | null;
  actorEmail: string;
  action: string;
  resourceType: string;
  resourceId: string;
  before: unknown;
  after: unknown;
  ip: string;
  requestId: string;
  createdAt: ISODateTime;
}

/** UI filter; dates are `YYYY-MM-DD` (local day) as entered in the form. */
export interface AuditFilter {
  actorId?: string;
  resourceType?: string;
  action?: string;
  fromDate?: string;
  toDate?: string;
}

export interface AuditQuery {
  actorId?: string;
  resourceType?: string;
  action?: string;
  /** RFC3339, inclusive. */
  from?: string;
  /** RFC3339, exclusive. */
  to?: string;
}

/** Local-day dates → RFC3339 range [from 00:00, to+1 00:00). Empty fields are dropped. */
export function toAuditQuery(f: AuditFilter): AuditQuery {
  const q: AuditQuery = {};
  if (f.actorId) q.actorId = f.actorId;
  if (f.resourceType?.trim()) q.resourceType = f.resourceType.trim();
  if (f.action?.trim()) q.action = f.action.trim();
  if (f.fromDate) q.from = new Date(`${f.fromDate}T00:00:00`).toISOString();
  if (f.toDate) {
    const d = new Date(`${f.toDate}T00:00:00`);
    d.setDate(d.getDate() + 1);
    q.to = d.toISOString();
  }
  return q;
}

export function isDateRangeValid(f: AuditFilter): boolean {
  return !f.fromDate || !f.toDate || f.fromDate <= f.toDate;
}

export const RESOURCE_TYPES = ["user", "asset", "brand", "campaign", "budget", "provider", "model", "job", "session"] as const;

/** Pretty JSON for before/after panels; `null`/`undefined` → "—". */
export function formatJson(v: unknown): string {
  if (v === null || v === undefined) return "—";
  return JSON.stringify(v, null, 2);
}

/** Top-level keys whose values differ between before and after (for highlighting). */
export function changedKeys(before: unknown, after: unknown): string[] {
  const b = isObject(before) ? before : {};
  const a = isObject(after) ? after : {};
  const keys = new Set([...Object.keys(b), ...Object.keys(a)]);
  return [...keys].filter((k) => JSON.stringify(b[k]) !== JSON.stringify(a[k])).sort();
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}
