import { createContext, useContext, useEffect, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import type { SystemStatus } from "../../types/api";
import { useAuthStore } from "../../stores/authStore";
import { useSystemStatusStore } from "../../stores/systemStatusStore";
import { systemService } from "../services/system";

type SystemStatusContextValue = {
  status: SystemStatus | null;
  loading: boolean;
  refetch: () => void;
};

const SystemStatusContext = createContext<SystemStatusContextValue | null>(null);

export function SystemStatusProvider({ children }: { children: ReactNode }) {
  const token = useAuthStore((state) => state.token);
  const setStatus = useSystemStatusStore((state) => state.setStatus);
  const statusQuery = useQuery({
    queryKey: ["system-status"],
    queryFn: systemService.status,
    enabled: Boolean(token),
    refetchInterval: 30_000
  });

  useEffect(() => {
    if (statusQuery.data) setStatus(statusQuery.data);
  }, [setStatus, statusQuery.data]);

  return (
    <SystemStatusContext.Provider
      value={{
        status: statusQuery.data ?? null,
        loading: statusQuery.isLoading,
        refetch: () => {
          void statusQuery.refetch();
        }
      }}
    >
      {children}
    </SystemStatusContext.Provider>
  );
}

export function useSystemStatusContext() {
  const value = useContext(SystemStatusContext);
  if (!value) {
    return { status: useSystemStatusStore.getState().status, loading: false, refetch: () => undefined };
  }
  return value;
}
