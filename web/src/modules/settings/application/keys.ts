import type { AuditQuery } from "../domain/audit";
import type { UserQuery } from "../domain/user";

export const settingsKeys = {
  all: ["settings"] as const,
  users: () => [...settingsKeys.all, "users"] as const,
  userList: (q: UserQuery) => [...settingsKeys.users(), q] as const,
  audit: (q: AuditQuery) => [...settingsKeys.all, "audit", q] as const,
  providers: () => [...settingsKeys.all, "providers"] as const,
  models: (providerId?: string) => [...settingsKeys.all, "models", providerId ?? "all"] as const,
  modelsAll: () => [...settingsKeys.all, "models"] as const,
  pricing: (modelId: string) => [...settingsKeys.all, "pricing", modelId] as const,
};
