import { Activity } from "lucide-react";
import { NavLink } from "react-router-dom";
import { hasFeature } from "../../shared/config/features";
import { cn } from "../../lib/cn";
import { useI18n } from "../../i18n/useI18n";
import { useSystemStatusStore } from "../../stores/systemStatusStore";
import { navItems, type NavItem } from "./navItems";

function SidebarLink({ item }: { item: NavItem }) {
  const { t } = useI18n();
  const Icon = item.icon;
  return (
    <NavLink
      to={item.to}
      end={item.end}
      title={t(item.labelKey)}
      className={({ isActive }) =>
        cn(
          "flex h-11 items-center justify-center gap-3 rounded-lg border px-0 text-sm text-text-muted transition duration-150 lg:justify-start lg:px-3",
          isActive
            ? "border-accent/10 bg-accent/[0.06] text-accent"
            : "border-transparent hover:bg-white/[0.04] hover:text-text-main"
        )
      }
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span className="hidden truncate lg:inline">{t(item.labelKey)}</span>
    </NavLink>
  );
}

export function Sidebar() {
  const status = useSystemStatusStore((state) => state.status);
  const { t } = useI18n();
  const visibleItems = navItems.filter((item) => !item.feature || hasFeature(item.feature, status));
  const mainItems = visibleItems.filter((item) => item.placement !== "footer");
  const footerItems = visibleItems.filter((item) => item.placement === "footer");

  return (
    <aside className="sticky top-0 flex h-screen w-16 shrink-0 flex-col border-r border-white/[0.04] bg-slate-950/45 p-3 backdrop-blur-xl lg:w-64">
      <div className="mb-6 flex h-11 items-center justify-center gap-3 rounded-lg border border-white/[0.04] bg-white/[0.03] lg:justify-start lg:px-3">
        <Activity className="h-5 w-5 text-warm" />
        <div className="hidden min-w-0 lg:block">
          <p className="truncate text-sm font-semibold text-text-main">{t("app.name")}</p>
          <p className="truncate text-[11px] text-text-weak">{t("app.subtitle")}</p>
        </div>
      </div>
      <nav className="flex flex-1 flex-col gap-1">
        {mainItems.map((item) => (
          <SidebarLink key={item.to} item={item} />
        ))}
      </nav>
      <nav className="mt-4 flex flex-col gap-1">
        {footerItems.map((item) => (
          <SidebarLink key={item.to} item={item} />
        ))}
      </nav>
    </aside>
  );
}
