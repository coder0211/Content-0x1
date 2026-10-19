import type { Page } from "@/shared/kernel";
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

export interface SettingsRepository {
  // Users & roles (ACS-008)
  listUsers(q: UserQuery): Promise<Page<ManagedUser>>;
  createUser(input: CreateUserInput): Promise<ManagedUser>;
  updateUser(id: string, input: UpdateUserInput): Promise<ManagedUser>;

  // Audit log (ACS-094)
  listAuditLogs(q: AuditQuery, cursor: string | null): Promise<Page<AuditEntry>>;

  // Providers / models / pricing (ACS-090)
  listProviders(): Promise<Provider[]>;
  updateProvider(id: string, input: UpdateProviderInput): Promise<Provider>;
  listModels(providerId?: string): Promise<AiModel[]>;
  createModel(input: CreateModelInput): Promise<AiModel>;
  updateModel(id: string, input: UpdateModelInput): Promise<AiModel>;
  listPricing(modelId: string): Promise<PricingRow[]>;
  addPricing(modelId: string, input: NewPricingInput): Promise<PricingRow[]>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    settings: SettingsRepository;
  }
}
