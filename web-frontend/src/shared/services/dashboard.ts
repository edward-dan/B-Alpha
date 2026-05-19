import type { DashboardResponse, EquitySnapshot } from "../../types/api";
import { apiRequest } from "./client";

export const dashboardService = {
  overview() {
    return apiRequest<DashboardResponse>("/dashboard");
  },

  equitySnapshots(instanceId: number, days: number) {
    const params = new URLSearchParams({ instance_id: String(instanceId), days: String(days) });
    return apiRequest<{ snapshots: EquitySnapshot[] }>(`/dashboard/equity-snapshots?${params.toString()}`);
  }
};
