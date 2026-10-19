import type { StatusKind, StatusKinds } from "@/shared/kernel";
import { cn } from "@/shared/lib/cn";

type Tone = "neutral" | "info" | "success" | "warning" | "danger";

const TONE_CLASS: Record<Tone, { pill: string; dot: string }> = {
  neutral: { pill: "border-muted-border bg-muted-bg text-muted", dot: "bg-muted" },
  info: { pill: "border-info-border bg-info-bg text-info", dot: "bg-info" },
  success: { pill: "border-success-border bg-success-bg text-success", dot: "bg-success" },
  warning: { pill: "border-warning-border bg-warning-bg text-warning", dot: "bg-warning" },
  danger: { pill: "border-danger-border bg-danger-bg text-danger", dot: "bg-danger" },
};

type StatusMap = { [K in StatusKind]: Record<StatusKinds[K], { label: string; tone: Tone }> };

/** 1-1 with SPEC-03 state machines. One status ⇒ one label and one color everywhere. */
export const STATUS_MAP: StatusMap = {
  asset: {
    draft: { label: "Nháp", tone: "neutral" },
    pending_review: { label: "Chờ duyệt", tone: "warning" },
    changes_requested: { label: "Yêu cầu sửa", tone: "danger" },
    approved: { label: "Đã duyệt", tone: "success" },
    archived: { label: "Đã lưu trữ", tone: "neutral" },
  },
  campaign: {
    draft: { label: "Nháp", tone: "neutral" },
    in_progress: { label: "Đang thực hiện", tone: "info" },
    review: { label: "Đang duyệt", tone: "warning" },
    completed: { label: "Hoàn thành", tone: "success" },
    archived: { label: "Đã lưu trữ", tone: "neutral" },
  },
  job: {
    queued: { label: "Đang chờ", tone: "neutral" },
    running: { label: "Đang chạy", tone: "info" },
    succeeded: { label: "Thành công", tone: "success" },
    failed: { label: "Lỗi", tone: "danger" },
    cancelled: { label: "Đã hủy", tone: "neutral" },
  },
  step: {
    pending: { label: "Chờ", tone: "neutral" },
    queued: { label: "Trong hàng đợi", tone: "neutral" },
    running: { label: "Đang chạy", tone: "info" },
    waiting_provider: { label: "Chờ nhà cung cấp", tone: "info" },
    succeeded: { label: "Xong", tone: "success" },
    failed: { label: "Lỗi", tone: "danger" },
    cancelled: { label: "Đã hủy", tone: "neutral" },
    skipped: { label: "Bỏ qua", tone: "neutral" },
  },
  scene: {
    draft: { label: "Nháp", tone: "neutral" },
    generating: { label: "Đang sinh", tone: "info" },
    ready: { label: "Sẵn sàng", tone: "success" },
    failed: { label: "Lỗi", tone: "danger" },
  },
  user: {
    active: { label: "Hoạt động", tone: "success" },
    locked: { label: "Tạm khóa", tone: "warning" },
    disabled: { label: "Vô hiệu", tone: "neutral" },
  },
  provider: {
    active: { label: "Đang bật", tone: "success" },
    disabled: { label: "Đã tắt", tone: "neutral" },
  },
  brand: {
    active: { label: "Đang dùng", tone: "success" },
    archived: { label: "Đã lưu trữ", tone: "neutral" },
  },
};

export function statusLabel<K extends StatusKind>(kind: K, status: StatusKinds[K]): string {
  return (STATUS_MAP[kind] as Record<string, { label: string }>)[status]?.label ?? String(status);
}

export interface StatusBadgeProps<K extends StatusKind> {
  kind: K;
  status: StatusKinds[K];
  className?: string;
}

/** 6px dot + label. Never re-color by context (SPEC-05 §3). */
export function StatusBadge<K extends StatusKind>({ kind, status, className }: StatusBadgeProps<K>) {
  const entry = (STATUS_MAP[kind] as Record<string, { label: string; tone: Tone }>)[status] ?? {
    label: String(status),
    tone: "neutral" as const,
  };
  const tone = TONE_CLASS[entry.tone];
  return (
    <span
      data-status={status}
      className={cn("inline-flex h-5 items-center gap-1.5 whitespace-nowrap rounded-sm border px-1.5 text-caption", tone.pill, className)}
    >
      <span aria-hidden className={cn("size-1.5 shrink-0 rounded-full", tone.dot)} />
      {entry.label}
    </span>
  );
}
