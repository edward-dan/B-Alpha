import type { CreateEvolutionTaskPayload, EvolutionStatus, EvolutionTask, GeneRecord } from "../../types/api";
import { apiRequest } from "./client";

export const evolutionService = {
  tasks() {
    return apiRequest<EvolutionStatus>("/evolution/tasks");
  },

  createTask(payload: CreateEvolutionTaskPayload) {
    return apiRequest<EvolutionTask | EvolutionStatus | Record<string, unknown>>("/evolution/tasks", {
      method: "POST",
      body: payload
    });
  },

  cancelTask(taskId: number) {
    return apiRequest<{ task: EvolutionTask }>(`/evolution/tasks/${taskId}/cancel`, { method: "POST" });
  },

  genomes(instanceId?: number) {
    const suffix = instanceId ? `?instance_id=${instanceId}` : "";
    return apiRequest<{ genomes: GeneRecord[] }>(`/evolution/genomes${suffix}`);
  },

  promote(taskId: number, geneId?: number) {
    return apiRequest<{ champion: GeneRecord }>(`/evolution/tasks/${taskId}/promote`, {
      method: "POST",
      body: geneId ? { gene_id: geneId } : {}
    });
  }
};
