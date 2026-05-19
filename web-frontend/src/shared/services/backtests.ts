import type { BacktestRun, CreateBacktestPayload } from "../../types/api";
import { apiRequest } from "./client";

export const backtestsService = {
  create(payload: CreateBacktestPayload) {
    return apiRequest<{ backtest: BacktestRun }>("/backtests", {
      method: "POST",
      body: payload
    });
  },

  get(id: number) {
    return apiRequest<{ backtest: BacktestRun }>(`/backtests/${id}`);
  }
};
