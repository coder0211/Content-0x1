import { http } from "@/shared/api/http";
import type { ID } from "@/shared/kernel";
import type { DashboardRepository } from "../application/ports";
import type { AppNotification } from "../domain/notification";
import type { DashboardSummary } from "../domain/summary";
import { toNotification, toSummary, type DashboardSummaryDto, type NotificationDto } from "./dto";

export class HttpDashboardRepository implements DashboardRepository {
  async summary(): Promise<DashboardSummary> {
    return toSummary(await http.get<DashboardSummaryDto>("/dashboard/summary"));
  }

  async notifications(limit: number): Promise<AppNotification[]> {
    const page = await http.page<NotificationDto>("/notifications", { query: { limit } });
    return page.items.map(toNotification);
  }

  async markNotificationRead(id: ID): Promise<void> {
    await http.post<void>(`/notifications/${encodeURIComponent(id)}/read`);
  }
}
