import type { Permission, Role } from "@/shared/kernel";

export interface AuthUser {
  id: string;
  email: string;
  name: string;
  roles: Role[];
  permissions: Permission[];
}

export interface Credentials {
  email: string;
  password: string;
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Client-side pre-check; the server remains authoritative. Returns field → message. */
export function validateCredentials(c: Credentials): Partial<Record<keyof Credentials, string>> {
  const errors: Partial<Record<keyof Credentials, string>> = {};
  if (!c.email.trim()) errors.email = "Nhập email.";
  else if (!EMAIL_RE.test(c.email.trim())) errors.email = "Email không hợp lệ.";
  if (!c.password) errors.password = "Nhập mật khẩu.";
  return errors;
}

/** Only allow same-app relative redirects after login (prevents open redirects). */
export function safeRedirectTarget(next: string | null | undefined): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/login")) return "/";
  return next;
}
