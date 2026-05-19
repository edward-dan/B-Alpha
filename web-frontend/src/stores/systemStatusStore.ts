import { create } from "zustand";
import type { SystemStatus } from "../types/api";

type SystemStatusStore = {
  status: SystemStatus | null;
  setStatus: (status: SystemStatus) => void;
};

export const useSystemStatusStore = create<SystemStatusStore>((set) => ({
  status: null,
  setStatus: (status) => set({ status })
}));
