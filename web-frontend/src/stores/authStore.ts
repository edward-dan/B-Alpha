import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { User } from "../types/api";

type AuthStore = {
  token: string | null;
  user: User | null;
  loading: boolean;
  login: (token: string, user: User) => void;
  logout: () => void;
  restore: () => Promise<User | null>;
  setSession: (token: string, user: User) => void;
  clearSession: () => void;
};

export const useAuthStore = create<AuthStore>()(
  persist(
    (set, get) => ({
      token: null,
      user: null,
      loading: true,
      login: (token, user) => set({ token, user, loading: false }),
      logout: () => set({ token: null, user: null, loading: false }),
      setSession: (token, user) => set({ token, user, loading: false }),
      clearSession: () => set({ token: null, user: null, loading: false }),
      restore: async () => {
        const token = get().token;
        if (!token) {
          set({ loading: false });
          return null;
        }
        set({ loading: true });
        const controller = new AbortController();
        const timeoutID = window.setTimeout(() => controller.abort(), 8_000);
        try {
          const response = await fetch("/api/v1/auth/me", {
            headers: { Authorization: `Bearer ${token}` },
            signal: controller.signal
          });
          if (!response.ok) throw new Error(response.statusText);
          const data = (await response.json()) as { user: User };
          set({ user: data.user, loading: false });
          return data.user;
        } catch {
          set({ token: null, user: null, loading: false });
          return null;
        } finally {
          window.clearTimeout(timeoutID);
        }
      }
    }),
    {
      name: "b-alpha-auth",
      partialize: (state) => ({ token: state.token, user: state.user })
    }
  )
);
