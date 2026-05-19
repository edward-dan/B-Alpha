import type { AgentStatus, SystemStatus } from "../../types/api";
import { normalizeAppRole } from "../config/features";
import { apiRequest, ApiRequestError } from "./client";

export const systemService = {
  async status(): Promise<SystemStatus> {
    try {
      const status = await apiRequest<Partial<SystemStatus> & { app_role?: unknown }>("/system/status");
      return {
        engine_state: status.engine_state ?? "running",
        app_role: normalizeAppRole(status.app_role),
        features: status.features ?? {},
        agent_connected: status.agent_connected ?? status.api_connected ?? false,
        api_connected: status.api_connected ?? status.agent_connected ?? false,
        api_configured: status.api_configured ?? false,
        agent_version: status.agent_version,
        last_heartbeat_at: status.last_heartbeat_at ?? null,
        needs_reconciliation: status.needs_reconciliation ?? false,
        reconciliation_reason: status.reconciliation_reason,
        checked_at: status.checked_at ?? new Date().toISOString()
      };
    } catch (error) {
      if (error instanceof ApiRequestError && error.status === 404) {
        return {
          engine_state: "running",
          app_role: "saas",
          features: { dashboard: true, strategies: true, agents: true, risk: true, backtesting: true, settings: true },
          agent_connected: false,
          api_connected: false,
          api_configured: false,
          needs_reconciliation: false,
          checked_at: new Date().toISOString()
        };
      }
      throw error;
    }
  },

  agentStatus() {
    return apiRequest<AgentStatus>("/agents/status");
  }
};
