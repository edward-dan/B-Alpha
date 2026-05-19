import type { AppFeature, AppRole, SystemStatus } from "../../types/api";
import { useSystemStatusStore } from "../../stores/systemStatusStore";

const roleDefaults: Record<AppRole, Record<AppFeature, boolean>> = {
  saas: {
    dashboard: true,
    strategies: false,
    agents: true,
    risk: false,
    backtesting: false,
    settings: true
  },
  lab: {
    dashboard: true,
    strategies: true,
    agents: true,
    risk: false,
    backtesting: true,
    settings: true
  },
  dev: {
    dashboard: true,
    strategies: true,
    agents: true,
    risk: true,
    backtesting: true,
    settings: true
  }
};

export function hasFeature(feature: AppFeature, status: SystemStatus | null | undefined = useSystemStatusStore.getState().status) {
  const role = status?.app_role ?? "dev";
  const defaultValue = roleDefaults[role]?.[feature] ?? false;
  return status?.features?.[feature] ?? defaultValue;
}

export function normalizeAppRole(value: unknown): AppRole {
  if (value === "saas" || value === "lab" || value === "dev") return value;
  return "dev";
}

export function defaultFeaturesForRole(role: AppRole): Record<AppFeature, boolean> {
  return roleDefaults[role];
}
