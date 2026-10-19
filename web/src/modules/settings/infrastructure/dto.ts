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

/** `AIProvider` in api/openapi/provider.yaml. The API never returns secret values, only `secret_ref`. */
export interface ProviderDto {
  id: string;
  code: string;
  name: string;
  kinds: ModelKind[];
  status: ActiveStatus;
  secret_ref: string;
  secret_configured: boolean;
  adapter_available: boolean;
  config_json: Record<string, unknown> | null;
  updated_at: string;
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
    kinds: d.kinds ?? [],
    status: d.status,
    secretRef: d.secret_ref,
    secretConfigured: d.secret_configured ?? null,
    config: d.config_json ?? {},
    updatedAt: d.updated_at ?? null,
  };
}

/** `AIModel` in api/openapi/provider.yaml. */
export interface ModelDto {
  id: string;
  provider_id: string;
  provider_code: string;
  provider_status: ActiveStatus;
  model_code: string;
  kind: ModelKind;
  capabilities: Record<string, unknown> | null;
  concurrency_limit: number;
  is_default: boolean;
  status: ActiveStatus;
  adapter_available: boolean;
  usable: boolean;
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

/** `AIPriceItem`: unit_price is a decimal string (shopspring/decimal). */
export interface PriceItemDto {
  unit: string;
  unit_price: string;
}

/** `AIPricingVersion` in api/openapi/provider.yaml — one version with all its unit prices. */
export interface PricingVersionDto {
  pricing_version: string;
  currency: string;
  effective_from: string;
  effective_to: string | null;
  is_current: boolean;
  items: PriceItemDto[];
}

/** POST /settings/models/{id}/pricing — appends a whole version. `effective_from` omitted = now. */
export interface CreatePricingDto {
  pricing_version: string;
  effective_from?: string;
  currency: string;
  items: PriceItemDto[];
}

/** Flattens a pricing version into one domain row per unit. */
export function toPricingRows(modelId: string, d: PricingVersionDto): PricingRow[] {
  return (d.items ?? []).map((it) => ({
    id: `${d.pricing_version}:${it.unit}`,
    modelId,
    pricingVersion: d.pricing_version,
    unit: it.unit,
    unitPrice: Number(it.unit_price),
    currency: d.currency,
    effectiveFrom: d.effective_from,
    effectiveTo: d.effective_to ?? null,
    isCurrent: d.is_current,
  }));
}
