import { useEffect, type ReactNode } from "react";
import { useAuthStore } from "../../stores/authStore";

export function AuthProvider({ children }: { children: ReactNode }) {
  const restore = useAuthStore((state) => state.restore);

  useEffect(() => {
    void restore();
  }, [restore]);

  return <>{children}</>;
}

export function useAuth() {
  return useAuthStore();
}
