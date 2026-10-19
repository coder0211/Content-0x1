import type { ID, ISODateTime } from "@/shared/kernel";
import { JOB_TYPE_LABELS } from "./summary";

/** In-app notification (GET /notifications, api/openapi/audit.yaml). Pure TS. */
export type NotificationType =
  | "job_failed"
  | "job_succeeded"
  | "review_requested"
  | "changes_requested"
  | "asset_approved"
  | "budget_alert";

export interface AppNotification {
  id: ID;
  type: NotificationType | string;
  payload: Record<string, unknown>;
  readAt: ISODateTime | null;
  createdAt: ISODateTime;
}

const str = (v: unknown): string => (typeof v === "string" ? v : "");

/** One-line Vietnamese text for a notification. */
export function notificationText(n: Pick<AppNotification, "type" | "payload">): string {
  const p = n.payload;
  const title = str(p.title) || str(p.asset_title);
  const who = str(p.actor_name);
  switch (n.type) {
    case "asset_approved":
      return `${who || "Reviewer"} đã duyệt “${title || "asset"}”`;
    case "changes_requested":
      return `${who || "Reviewer"} yêu cầu sửa “${title || "asset"}”`;
    case "review_requested":
      return `${who || "Có người"} gửi duyệt “${title || "asset"}”`;
    case "job_failed":
      return `${JOB_TYPE_LABELS[str(p.job_type)] ?? "Job"} bị lỗi`;
    case "job_succeeded":
      return `${JOB_TYPE_LABELS[str(p.job_type)] ?? "Job"} đã xong`;
    case "budget_alert": {
      const pct = typeof p.threshold === "number" ? p.threshold : null;
      const scope = p.scope === "campaign" ? "Ngân sách campaign" : "Ngân sách toàn hệ thống";
      return pct !== null ? `${scope} đã dùng ${pct}%` : "Cảnh báo ngân sách";
    }
    default:
      return title || "Thông báo mới";
  }
}

/** Where clicking a notification goes (null: nowhere). */
export function notificationLink(n: Pick<AppNotification, "type" | "payload">): string | null {
  const p = n.payload;
  if (n.type === "review_requested" && str(p.asset_id)) return "/review";
  if (str(p.asset_id)) return `/library/${str(p.asset_id)}`;
  if (str(p.job_id)) return `/jobs/${str(p.job_id)}`;
  if (n.type === "budget_alert") return "/usage/budgets";
  return null;
}
