import type { AppFeature, AppRole, SystemStatus } from "../../types/api";
import { useSystemStatusStore } from "../../stores/systemStatusStore";

const roleDefaults: Record<AppRole, Record<AppFeature, boolean>> = {
  saas: {
    dashboard: true,
    strategies: true,
    agents: true,
    risk: true,
    backtesting: true,
    settings: true
  }
};

export function hasFeature(feature: AppFeature, status: SystemStatus | null | undefined = useSystemStatusStore.getState().status) {
  const role = status?.app_role ?? "saas";
  const defaultValue = roleDefaults[role]?.[feature] ?? false;
  return status?.features?.[feature] ?? defaultValue;
}

export function normalizeAppRole(value: unknown): AppRole {
  if (value === "saas") return value;
  return "saas";
}

export function defaultFeaturesForRole(role: AppRole): Record<AppFeature, boolean> {
  return roleDefaults[role];
}
