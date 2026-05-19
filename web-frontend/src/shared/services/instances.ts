import type { CreateInstancePayload, StrategyInstance } from "../../types/api";
import { apiRequest } from "./client";

export const instancesService = {
  list() {
    return apiRequest<{ instances: StrategyInstance[] }>("/instances");
  },

  create(payload: CreateInstancePayload) {
    return apiRequest<{ instance: StrategyInstance }>("/instances", {
      method: "POST",
      body: payload
    });
  },

  updateStatus(id: number, status: "running" | "stopped") {
    return apiRequest<{ id: number; status: string }>(`/instances/${id}`, {
      method: "PATCH",
      body: { status }
    });
  },

  start(id: number) {
    return apiRequest<{ id: number; status: string }>(`/instances/${id}/start`, { method: "POST" });
  },

  stop(id: number) {
    return apiRequest<{ id: number; status: string }>(`/instances/${id}/stop`, { method: "POST" });
  },

  remove(id: number) {
    return apiRequest<{ id: number; status: string }>(`/instances/${id}`, { method: "DELETE" });
  }
};
