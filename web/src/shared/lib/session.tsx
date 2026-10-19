import { createContext, useContext, type ReactNode } from "react";
import { hasPermission, type Permission, type PermissionRequirement, type Role } from "@/shared/kernel";

export interface SessionUser {
  id: string;
  email: string;
  name: string;
  roles: Role[];
  permissions: Permission[];
}

export type SessionStatus = "loading" | "authenticated" | "anonymous";

export interface Session {
  status: SessionStatus;
  user: SessionUser | null;
  can: (required: PermissionRequirement) => boolean;
  logout: () => Promise<void>;
}

/** Provided by the auth module (`AuthProvider`); consumed anywhere through these hooks. */
export const SessionContext = createContext<Session | null>(null);

export function useSession(): Session {
  const s = useContext(SessionContext);
  if (!s) throw new Error("useSession must be used inside the auth module's <AuthProvider>");
  return s;
}

export function useCan(required: PermissionRequirement): boolean {
  return useSession().can(required);
}

/** Renders children only when the user holds `permission` (all of them, if a list). */
export function Can({
  permission,
  children,
  fallback = null,
}: {
  permission: PermissionRequirement;
  children: ReactNode;
  fallback?: ReactNode;
}) {
  return <>{useCan(permission) ? children : fallback}</>;
}

export function makeCan(user: SessionUser | null): Session["can"] {
  return (required) => (user ? hasPermission(user.permissions, required) : false);
}
