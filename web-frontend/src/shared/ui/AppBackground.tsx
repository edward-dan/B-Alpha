import type { ReactNode } from "react";
import { CosmicBackground } from "../../components/layout/CosmicBackground";

export function AppBackground({ children }: { children: ReactNode }) {
  return <CosmicBackground>{children}</CosmicBackground>;
}
