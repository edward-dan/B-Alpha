import { LogOut, Mail, RefreshCw } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../lib/api";
import { Button } from "../ui/button";
import { StatusPill } from "./StatusPill";
import { useAuthStore } from "../../stores/authStore";
import { useSystemStatusStore } from "../../stores/systemStatusStore";
import { useI18n } from "../../i18n/useI18n";
import { formatDateTime } from "../../lib/format";
import { ReconciliationModal } from "./ReconciliationModal";
import { useEffect, useState } from "react";

export function Topbar() {
  const { t } = useI18n();
  const token = useAuthStore((state) => state.token);
  const user = useAuthStore((state) => state.user);
  const clearSession = useAuthStore((state) => state.clearSession);
  const setStatus = useSystemStatusStore((state) => state.setStatus);
  const [modalOpen, setModalOpen] = useState(false);

  const statusQuery = useQuery({
    queryKey: ["system-status"],
    queryFn: () => api.getSystemStatus(token ?? ""),
    enabled: Boolean(token),
    refetchInterval: 30_000
  });

  const agentQuery = useQuery({
    queryKey: ["agent-status"],
    queryFn: () => api.getAgentStatus(token ?? ""),
    enabled: Boolean(token),
    refetchInterval: 30_000
  });

  const status = statusQuery.data
    ? {
        ...statusQuery.data,
        agent_connected: agentQuery.data?.connected ?? statusQuery.data.agent_connected
      }
    : null;

  useEffect(() => {
    if (!status) return;
    setStatus(status);
    if (status.needs_reconciliation) setModalOpen(true);
  }, [setStatus, status]);

  const engineLabel =
    status?.engine_state === "halted"
      ? t("topbar.engine.halted")
      : status?.engine_state === "paused"
        ? t("topbar.engine.paused")
        : t("topbar.engine.running");
  const engineTone = status?.engine_state === "halted" ? "danger" : status?.engine_state === "paused" ? "warning" : "success";

  return (
    <header className="sticky top-0 z-20 border-b border-white/[0.04] bg-slate-950/35 px-4 py-3 backdrop-blur-xl lg:px-6">
      <div className="mx-auto flex max-w-[1800px] items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <StatusPill tone={engineTone}>{engineLabel}</StatusPill>
          <StatusPill tone={status?.agent_connected ? "success" : "warning"}>
            {status?.agent_connected ? t("topbar.agent.online") : t("topbar.agent.offline")}
          </StatusPill>
          <span className="hidden items-center gap-2 text-xs text-text-weak md:inline-flex">
            <RefreshCw className="h-3.5 w-3.5" />
            {formatDateTime(status?.checked_at)}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <div className="hidden items-center gap-2 rounded-md border border-white/[0.04] bg-white/[0.03] px-3 py-2 text-xs text-text-muted sm:flex">
            <Mail className="h-3.5 w-3.5 text-text-weak" />
            <span className="max-w-48 truncate">{user?.email}</span>
          </div>
          <Button
            title={t("common.logout")}
            aria-label={t("common.logout")}
            variant="ghost"
            size="icon"
            icon={<LogOut className="h-4 w-4" />}
            onClick={clearSession}
          />
        </div>
      </div>
      <ReconciliationModal
        open={modalOpen}
        reason={status?.reconciliation_reason}
        onClose={() => setModalOpen(false)}
      />
    </header>
  );
}
