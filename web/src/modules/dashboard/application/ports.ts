import type { ID } from "@/shared/kernel";
import type { AppNotification } from "../domain/notification";
import type { DashboardSummary } from "../domain/summary";

export interface DashboardRepository {
  summary(): Promise<DashboardSummary>;
  /** Latest notifications of the current user (newest first). */
  notifications(limit: number): Promise<AppNotification[]>;
  markNotificationRead(id: ID): Promise<void>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    dashboard: DashboardRepository;
  }
}
