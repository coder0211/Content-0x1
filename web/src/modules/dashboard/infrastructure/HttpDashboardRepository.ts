import { http } from "@/shared/api/http";
import type { DashboardRepository } from "../application/ports";
import type { DashboardSummary } from "../domain/summary";
import { toSummary, type DashboardSummaryDto } from "./dto";

export class HttpDashboardRepository implements DashboardRepository {
  async summary(): Promise<DashboardSummary> {
    return toSummary(await http.get<DashboardSummaryDto>("/dashboard/summary"));
  }
}
