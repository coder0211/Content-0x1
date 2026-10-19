import type { Page } from "@/shared/kernel";
import { http } from "@/shared/api/http";
import type { SettingsRepository } from "../application/ports";
import type { AuditEntry, AuditQuery } from "../domain/audit";
import type {
  AiModel,
  CreateModelInput,
  NewPricingInput,
  PricingRow,
  Provider,
  UpdateModelInput,
  UpdateProviderInput,
} from "../domain/provider";
import type { CreateUserInput, ManagedUser, UpdateUserInput, UserQuery } from "../domain/user";
import {
  toAuditEntry,
  toModel,
  toPricingRows,
  toProvider,
  toUser,
  type AuditLogDto,
  type CreateModelDto,
  type CreatePricingDto,
  type CreateUserDto,
  type ModelDto,
  type PricingVersionDto,
  type ProviderDto,
  type UpdateModelDto,
  type UpdateProviderDto,
  type UpdateUserDto,
  type UserDto,
} from "./dto";

const PAGE_SIZE = 50;

export class HttpSettingsRepository implements SettingsRepository {
  async listUsers(q: UserQuery): Promise<Page<ManagedUser>> {
    const p = await http.page<UserDto>("/users", {
      query: { q: q.q || undefined, status: q.status, limit: PAGE_SIZE, cursor: q.cursor ?? undefined },
    });
    return { items: p.items.map(toUser), nextCursor: p.nextCursor };
  }

  async createUser(input: CreateUserInput): Promise<ManagedUser> {
    const body: CreateUserDto = {
      email: input.email.trim(),
      name: input.name.trim(),
      role_codes: input.roleCodes,
      temp_password: input.tempPassword,
    };
    return toUser(await http.post<UserDto>("/users", body));
  }

  async updateUser(id: string, input: UpdateUserInput): Promise<ManagedUser> {
    const body: UpdateUserDto = {};
    if (input.name !== undefined) body.name = input.name.trim();
    if (input.status !== undefined) body.status = input.status;
    if (input.roleCodes !== undefined) body.role_codes = input.roleCodes;
    return toUser(await http.patch<UserDto>(`/users/${id}`, body));
  }

  async listAuditLogs(q: AuditQuery, cursor: string | null): Promise<Page<AuditEntry>> {
    const p = await http.page<AuditLogDto>("/audit-logs", {
      query: {
        actor_id: q.actorId,
        resource_type: q.resourceType,
        action: q.action,
        from: q.from,
        to: q.to,
        limit: PAGE_SIZE,
        cursor: cursor ?? undefined,
      },
    });
    return { items: p.items.map(toAuditEntry), nextCursor: p.nextCursor };
  }

  async listProviders(): Promise<Provider[]> {
    return (await http.get<ProviderDto[]>("/settings/providers")).map(toProvider);
  }

  async updateProvider(id: string, input: UpdateProviderInput): Promise<Provider> {
    const body: UpdateProviderDto = {};
    if (input.status !== undefined) body.status = input.status;
    if (input.secretRef !== undefined) body.secret_ref = input.secretRef.trim();
    if (input.config !== undefined) body.config_json = input.config;
    return toProvider(await http.patch<ProviderDto>(`/settings/providers/${id}`, body));
  }

  async listModels(providerId?: string): Promise<AiModel[]> {
    // GET /settings/models filters by `kind` only; narrow to one provider on the client.
    const all = await http.get<ModelDto[]>("/settings/models");
    return all.filter((m) => !providerId || m.provider_id === providerId).map(toModel);
  }

  async createModel(input: CreateModelInput): Promise<AiModel> {
    const body: CreateModelDto = {
      provider_id: input.providerId,
      model_code: input.modelCode.trim(),
      kind: input.kind,
      capabilities: input.capabilities,
      concurrency_limit: input.concurrencyLimit,
      is_default: input.isDefault,
    };
    return toModel(await http.post<ModelDto>("/settings/models", body));
  }

  async updateModel(id: string, input: UpdateModelInput): Promise<AiModel> {
    const body: UpdateModelDto = {};
    if (input.capabilities !== undefined) body.capabilities = input.capabilities;
    if (input.concurrencyLimit !== undefined) body.concurrency_limit = input.concurrencyLimit;
    if (input.isDefault !== undefined) body.is_default = input.isDefault;
    if (input.status !== undefined) body.status = input.status;
    return toModel(await http.patch<ModelDto>(`/settings/models/${id}`, body));
  }

  async listPricing(modelId: string): Promise<PricingRow[]> {
    const versions = await http.get<PricingVersionDto[]>(`/settings/models/${modelId}/pricing`);
    return versions.flatMap((v) => toPricingRows(modelId, v));
  }

  async addPricing(modelId: string, input: NewPricingInput): Promise<PricingRow[]> {
    const body: CreatePricingDto = {
      pricing_version: input.pricingVersion,
      currency: input.currency,
      items: input.prices.map((p) => ({ unit: p.unit, unit_price: String(p.unitPrice) })),
    };
    // The API rejects a start in the past (`in_past`); "today" means "from now", so omit it.
    if (Date.parse(input.effectiveFrom) > Date.now()) body.effective_from = input.effectiveFrom;
    const created = await http.post<PricingVersionDto>(`/settings/models/${modelId}/pricing`, body);
    return toPricingRows(modelId, created);
  }
}
