import { ROLES, type ID, type ISODateTime, type Permission, type Role, type UserStatus } from "@/shared/kernel";

export interface ManagedUser {
  id: ID;
  email: string;
  name: string;
  status: UserStatus;
  roles: Role[];
  lockedUntil: ISODateTime | null;
  lastLoginAt: ISODateTime | null;
  createdAt: ISODateTime;
}

export interface UserQuery {
  q?: string;
  status?: UserStatus;
  cursor?: string | null;
}

export interface CreateUserInput {
  email: string;
  name: string;
  roleCodes: Role[];
  tempPassword: string;
}

export interface UpdateUserInput {
  name?: string;
  status?: UserStatus;
  roleCodes?: Role[];
}

/** Matches the backend minimum (DECISIONS, Agent 1). */
export const MIN_PASSWORD_LENGTH = 10;

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export type CreateUserErrors = Partial<Record<"email" | "name" | "role_codes" | "temp_password", string>>;

/** Client-side validation mirroring POST /users; the server re-validates. */
export function validateCreateUser(input: CreateUserInput): CreateUserErrors {
  const e: CreateUserErrors = {};
  if (!input.email.trim()) e.email = "Nhập email.";
  else if (!EMAIL_RE.test(input.email.trim())) e.email = "Email không hợp lệ.";
  if (!input.name.trim()) e.name = "Nhập họ tên.";
  if (input.roleCodes.length === 0) e.role_codes = "Chọn ít nhất một vai trò.";
  if (input.tempPassword.length < MIN_PASSWORD_LENGTH) e.temp_password = `Mật khẩu tạm cần ít nhất ${MIN_PASSWORD_LENGTH} ký tự.`;
  return e;
}

export const ROLE_DESCRIPTIONS: Record<Role, string> = {
  admin: "Toàn quyền, quản lý user, provider, ngân sách và nhật ký.",
  marketing_lead: "Quản lý brand, campaign, ngân sách; duyệt và tải mọi asset.",
  content_creator: "Tạo campaign, sinh nội dung; tải asset đã duyệt.",
  reviewer: "Duyệt hoặc yêu cầu sửa asset; tải asset đã duyệt.",
};

export const PERMISSION_LABELS: Record<Permission, string> = {
  "user.manage": "Quản lý user",
  "provider.manage": "Quản lý provider/model",
  "provider.view": "Xem provider/model, job",
  "budget.manage": "Quản lý ngân sách",
  "brand.manage": "Quản lý brand",
  "brand.view": "Xem brand, asset",
  "campaign.manage": "Quản lý campaign",
  "generate.run": "Chạy sinh AI",
  "asset.review": "Duyệt asset",
  "asset.download": "Tải asset",
  "usage.view": "Xem chi phí",
  "audit.view": "Xem nhật ký",
};

/** SPEC-01 §2 role matrix, displayed read-only (the backend is the authority). */
export const ROLE_MATRIX: Record<Role, readonly Permission[]> = {
  admin: Object.keys(PERMISSION_LABELS) as Permission[],
  marketing_lead: [
    "provider.view",
    "budget.manage",
    "brand.manage",
    "brand.view",
    "campaign.manage",
    "generate.run",
    "asset.review",
    "asset.download",
    "usage.view",
  ],
  content_creator: ["brand.view", "campaign.manage", "generate.run", "asset.download"],
  reviewer: ["brand.view", "asset.review", "asset.download"],
};

export const ALL_ROLES: readonly Role[] = ROLES;

export interface StatusAction {
  to: UserStatus;
  label: string;
  confirmTitle: string;
  description: string;
  tone: "primary" | "danger";
}

/** Status changes offered for a user. Admins cannot lock/disable themselves (backend rule). */
export function statusActions(user: ManagedUser, currentUserId: ID | null): StatusAction[] {
  if (user.id === currentUserId) return [];
  const lock: StatusAction = {
    to: "locked",
    label: "Khóa tài khoản",
    confirmTitle: `Khóa tài khoản ${user.name}?`,
    description: "Mọi phiên đăng nhập của user bị thu hồi ngay. Có thể mở khóa lại sau.",
    tone: "danger",
  };
  const disable: StatusAction = {
    to: "disabled",
    label: "Vô hiệu hóa",
    confirmTitle: `Vô hiệu hóa ${user.name}?`,
    description: "User không thể đăng nhập; mọi phiên bị thu hồi. Dữ liệu và lịch sử được giữ lại.",
    tone: "danger",
  };
  const activate: StatusAction = {
    to: "active",
    label: "Kích hoạt lại",
    confirmTitle: `Kích hoạt lại ${user.name}?`,
    description: "User có thể đăng nhập lại bằng mật khẩu hiện có.",
    tone: "primary",
  };
  switch (user.status) {
    case "active":
      return [lock, disable];
    case "locked":
      return [activate, disable];
    case "disabled":
      return [activate];
  }
}
