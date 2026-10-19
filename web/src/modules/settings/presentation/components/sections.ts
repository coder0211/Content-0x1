import { Cpu, ScrollText, Users, Wallet, type LucideIcon } from "lucide-react";
import type { Permission } from "@/shared/kernel";

export interface SettingsSection {
  to: string;
  label: string;
  icon: LucideIcon;
  permission: Permission;
}

/** Side navigation inside /settings. Budgets live in the budget module (/usage). */
export const SETTINGS_SECTIONS: SettingsSection[] = [
  { to: "/settings/users", label: "Người dùng & vai trò", icon: Users, permission: "user.manage" },
  { to: "/settings/providers", label: "Provider & model", icon: Cpu, permission: "provider.view" },
  { to: "/usage", label: "Ngân sách", icon: Wallet, permission: "budget.manage" },
  { to: "/settings/audit", label: "Nhật ký hệ thống", icon: ScrollText, permission: "audit.view" },
];
