import {
  BarChart3,
  FlaskConical,
  Home,
  LayoutTemplate,
  MonitorCog,
  PlusCircle,
  Settings
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { AppFeature } from "../../types/api";

export type NavItem = {
  to: string;
  labelKey: string;
  icon: LucideIcon;
  placement: "main" | "footer";
  feature?: AppFeature;
  end?: boolean;
};

export const navItems: NavItem[] = [
  { to: "/", labelKey: "nav.dashboard", icon: Home, placement: "main", feature: "dashboard", end: true },
  { to: "/templates", labelKey: "nav.templates", icon: LayoutTemplate, placement: "main", feature: "strategies" },
  { to: "/instances", labelKey: "nav.instances", icon: PlusCircle, placement: "main", feature: "dashboard" },
  { to: "/evolution", labelKey: "nav.evolution", icon: FlaskConical, placement: "main", feature: "strategies" },
  { to: "/agents", labelKey: "nav.agents", icon: MonitorCog, placement: "main", feature: "agents" },
  { to: "/backtesting", labelKey: "nav.backtesting", icon: BarChart3, placement: "main" },
  { to: "/settings", labelKey: "nav.settings", icon: Settings, placement: "footer", feature: "settings" }
];
