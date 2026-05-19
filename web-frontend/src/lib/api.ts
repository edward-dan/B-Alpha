import type { CreateBacktestPayload, CreateEvolutionTaskPayload, CreateInstancePayload, StrategyTemplate } from "../types/api";
import {
  ApiRequestError,
  authService,
  backtestsService,
  dashboardService,
  evolutionService,
  instancesService,
  systemService
} from "../shared/services";

export { ApiRequestError };

export const api = {
  login: authService.login,
  register: authService.register,
  me: authService.me,
  getDashboard: (_token?: string) => dashboardService.overview(),
  getStrategies: async (_token?: string): Promise<{ strategies: StrategyTemplate[] }> => ({ strategies: [] }),
  getInstances: (_token?: string) => instancesService.list(),
  createInstance: (_token: string | undefined, payload: CreateInstancePayload) => instancesService.create(payload),
  startInstance: (_token: string | undefined, id: number) => instancesService.start(id),
  stopInstance: (_token: string | undefined, id: number) => instancesService.stop(id),
  deleteInstance: (_token: string | undefined, id: number) => instancesService.remove(id),
  getAgentStatus: (_token?: string) => systemService.agentStatus(),
  getSystemStatus: (_token?: string) => systemService.status(),
  getEvolutionTasks: (_token?: string) => evolutionService.tasks(),
  createEvolutionTask: (_token: string | undefined, payload: CreateEvolutionTaskPayload) => evolutionService.createTask(payload),
  getChallengerGenomes: async (_token?: string) => {
    const data = await evolutionService.genomes();
    return { challengers: data.genomes.filter((genome) => genome.role === "challenger") };
  },
  promoteEvolutionTask: (_token: string | undefined, taskId: number, geneId?: number) => evolutionService.promote(taskId, geneId),
  createBacktest: (_token: string | undefined, payload: CreateBacktestPayload) => backtestsService.create(payload),
  getBacktest: (_token: string | undefined, id: number) => backtestsService.get(id)
};
