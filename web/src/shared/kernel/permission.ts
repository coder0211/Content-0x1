/** Permission codes — SPEC-01 §2. The backend is the authority; FE only hides/guards UI. */
export const PERMISSIONS = [
  "user.manage",
  "provider.manage",
  "provider.view",
  "budget.manage",
  "brand.manage",
  "brand.view",
  "campaign.manage",
  "generate.run",
  "asset.review",
  "asset.download",
  "usage.view",
  "audit.view",
] as const;

export type Permission = (typeof PERMISSIONS)[number];

export const ROLES = ["admin", "marketing_lead", "content_creator", "reviewer"] as const;
export type Role = (typeof ROLES)[number];

/**
 * One code, a list (ALL required), or `{ anyOf }` (at least one). `undefined` means no requirement.
 */
export type PermissionRequirement = Permission | readonly Permission[] | { anyOf: readonly Permission[] } | undefined;

export function hasPermission(
  granted: ReadonlySet<Permission> | readonly Permission[],
  required: PermissionRequirement,
): boolean {
  if (required === undefined) return true;
  const set: ReadonlySet<Permission> = granted instanceof Set ? granted : new Set(granted as readonly Permission[]);
  if (typeof required === "string") return set.has(required);
  if ("anyOf" in required) return required.anyOf.some((p) => set.has(p));
  return required.every((p) => set.has(p));
}
