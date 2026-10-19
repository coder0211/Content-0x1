import type { ActiveStatus, Role, UserStatus } from "@/shared/kernel";
import type { AuditEntry } from "../domain/audit";
import type { AiModel, ModelKind, PricingRow, Provider } from "../domain/provider";
import type { ManagedUser } from "../domain/user";

/** `User` in api/openapi/identity.yaml. */
export interface UserDto {
  id: string;
  email: string;
  name: string;
  status: UserStatus;
  roles: Role[];
  locked_until: string | null;
  last_login_at: string | null;
  created_at: string;
}

export interface CreateUserDto {
  email: string;
  name: string;
  role_codes: Role[];
  temp_password: string;
}

export interface UpdateUserDto {
  name?: string;
  status?: UserStatus;
  role_codes?: Role[];
}

export function toUser(d: UserDto): ManagedUser {
  return {
    id: d.id,
    email: d.email,
    name: d.name,
    status: d.status,
    roles: d.roles ?? [],
    lockedUntil: d.locked_until ?? null,
    lastLoginAt: d.last_login_at ?? null,
    createdAt: d.created_at,
  };
}

/** `AuditLog` in api/openapi/audit.yaml. */
export interface AuditLogDto {
  id: number;
  actor_id: string | null;
  actor_email?: string;
  action: string;
  resource_type: string;
  resource_id: string;
  before?: unknown;
  after?: unknown;
  ip?: string;
  request_id?: string;
  created_at: string;
}

export function toAuditEntry(d: AuditLogDto): AuditEntry {
  return {
    id: d.id,
    actorId: d.actor_id ?? null,
    actorEmail: d.actor_email ?? "",
    action: d.action,
    resourceType: d.resource_type,
    resourceId: d.resource_id,
    before: d.before ?? null,
    after: d.after ?? null,
    ip: d.ip ?? "",
    requestId: d.request_id ?? "",
    createdAt: d.created_at,
  };
}

/*
 * Provider admin DTOs (SPEC-04 §2.9, columns from SPEC-03 ai_providers / ai_models / model_pricing).
 * The provider API is built in parallel (Agent 4); these shapes are an assumption recorded in
 * docs/DECISIONS.md. The API never returns secret values: only `secret_ref` (a name) and,
 * optionally, `secret_configured` (whether that secret resolves on the server).
 */
export interface ProviderDto {
  id: string;
  code: string;
  name: string;
  kind: ModelKind[];
  status: ActiveStatus;
  secret_ref: string;
  secret_configured?: boolean | null;
  config_json: Record<string, unknown> | null;
  created_at?: string;
  updated_at?: string | null;
}

export interface UpdateProviderDto {
  status?: ActiveStatus;
  secret_ref?: string;
  config_json?: Record<string, unknown>;
}

export function toProvider(d: ProviderDto): Provider {
  return {
    id: d.id,
    code: d.code,
    name: d.name,
    kinds: d.kind ?? [],
    status: d.status,
    secretRef: d.secret_ref,
    secretConfigured: d.secret_configured ?? null,
    config: d.config_json ?? {},
    updatedAt: d.updated_at ?? null,
  };
}

export interface ModelDto {
  id: string;
  provider_id: string;
  provider_code?: string | null;
  model_code: string;
  kind: ModelKind;
  capabilities: Record<string, unknown> | null;
  concurrency_limit: number;
  is_default: boolean;
  status: ActiveStatus;
}

export interface CreateModelDto {
  provider_id: string;
  model_code: string;
  kind: ModelKind;
  capabilities: Record<string, unknown>;
  concurrency_limit: number;
  is_default: boolean;
}

export interface UpdateModelDto {
  capabilities?: Record<string, unknown>;
  concurrency_limit?: number;
  is_default?: boolean;
  status?: ActiveStatus;
}

export function toModel(d: ModelDto): AiModel {
  return {
    id: d.id,
    providerId: d.provider_id,
    providerCode: d.provider_code ?? null,
    modelCode: d.model_code,
    kind: d.kind,
    capabilities: d.capabilities ?? {},
    concurrencyLimit: d.concurrency_limit,
    isDefault: d.is_default,
    status: d.status,
  };
}

/** NUMERIC(14,8) may arrive as a JSON number or a decimal string. */
export interface PricingDto {
  id: string;
  model_id: string;
  pricing_version: string;
  unit: string;
  unit_price: number | string;
  currency: string;
  effective_from: string;
  effective_to: string | null;
}

/** POST /settings/models/{id}/pricing — appends a whole version (one row per unit). */
export interface CreatePricingDto {
  pricing_version: string;
  effective_from: string;
  currency: string;
  prices: { unit: string; unit_price: number }[];
}

export function toPricing(d: PricingDto): PricingRow {
  return {
    id: d.id,
    modelId: d.model_id,
    pricingVersion: d.pricing_version,
    unit: d.unit,
    unitPrice: typeof d.unit_price === "string" ? Number(d.unit_price) : d.unit_price,
    currency: d.currency,
    effectiveFrom: d.effective_from,
    effectiveTo: d.effective_to ?? null,
  };
}
