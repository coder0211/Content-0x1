/**
 * Display formatters — SPEC-05 §6. All UI numbers/dates MUST go through these.
 * Dates use the viewer's local timezone.
 */
const NBSP = " ";

const usd = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});
const viNumber = new Intl.NumberFormat("vi-VN");
const viDecimal1 = new Intl.NumberFormat("vi-VN", { maximumFractionDigits: 1 });
const viDate = new Intl.DateTimeFormat("vi-VN", { day: "2-digit", month: "2-digit", year: "numeric" });
const viTime = new Intl.DateTimeFormat("vi-VN", { hour: "2-digit", minute: "2-digit", hour12: false });

type DateInput = Date | string | number;
const toDate = (d: DateInput): Date => (d instanceof Date ? d : new Date(d));

/** AI cost in USD: `$0.42`, `< $0.01`, `≈ $0.42` when `estimate`. */
export function formatUsd(amount: number, opts: { estimate?: boolean } = {}): string {
  const body = amount > 0 && amount < 0.01 ? "< $0.01" : usd.format(amount);
  return opts.estimate ? `≈ ${body}` : body;
}

/** VND: `2.990.000 ₫`. */
export function formatVnd(amount: number): string {
  return `${viNumber.format(Math.round(amount))}${NBSP}₫`;
}

export function formatMoney(m: { amount: number; currency: string }, opts: { estimate?: boolean } = {}): string {
  return m.currency === "VND" ? formatVnd(m.amount) : formatUsd(m.amount, opts);
}

export function formatNumber(n: number): string {
  return viNumber.format(n);
}

/** `16/10/2026` */
export function formatDate(d: DateInput): string {
  return viDate.format(toDate(d));
}

/** `14:05` */
export function formatTime(d: DateInput): string {
  return viTime.format(toDate(d));
}

/** `16/10/2026 14:05` */
export function formatDateTime(d: DateInput): string {
  const date = toDate(d);
  return `${viDate.format(date)} ${viTime.format(date)}`;
}

/** `Vừa xong` / `5 phút trước` / `3 giờ trước` under 24h, otherwise absolute date. */
export function formatRelative(d: DateInput, now: Date = new Date()): string {
  const diffSec = Math.round((now.getTime() - toDate(d).getTime()) / 1000);
  if (diffSec >= 0 && diffSec < 60) return "Vừa xong";
  if (diffSec >= 60 && diffSec < 3600) return `${Math.floor(diffSec / 60)} phút trước`;
  if (diffSec >= 3600 && diffSec < 86400) return `${Math.floor(diffSec / 3600)} giờ trước`;
  return formatDate(d);
}

/** Video/media duration from seconds: `0:20`, `1:05`, `1:02:03`. */
export function formatDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.round(totalSeconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${sec}` : `${m}:${sec}`;
}

export function formatDurationMs(ms: number): string {
  return formatDuration(ms / 1000);
}

/** Remaining-time hint for long jobs: `~30 giây`, `~2 phút`, `~1 giờ`. */
export function formatEta(seconds: number): string {
  if (seconds < 60) return `~${Math.max(5, Math.round(seconds / 5) * 5)} giây`;
  if (seconds < 3600) return `~${Math.round(seconds / 60)} phút`;
  return `~${viDecimal1.format(seconds / 3600)} giờ`;
}

/** `12,4 MB` (binary units, vi-VN decimal comma). */
export function formatFileSize(bytes: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${i === 0 ? v : viDecimal1.format(v)} ${units[i]}`;
}

export function formatPercent(ratio: number): string {
  return `${Math.round(ratio * 100)}%`;
}
