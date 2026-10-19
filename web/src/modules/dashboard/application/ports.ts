import type { DashboardSummary } from "../domain/summary";

export interface DashboardRepository {
  summary(): Promise<DashboardSummary>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    dashboard: DashboardRepository;
  }
}
