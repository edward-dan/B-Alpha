import { CheckCircle2, Download, FileCode2, KeyRound, RadioTower } from "lucide-react";
import { useI18n } from "../../i18n/useI18n";
import { formatDateTime } from "../../lib/format";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../../shared/ui/Card";
import { useSystemStatusContext } from "../../shared/contexts/SystemStatusContext";

export function AgentsPage() {
  const { t } = useI18n();
  const { status } = useSystemStatusContext();
  const online = Boolean(status?.api_connected ?? status?.agent_connected);

  return (
    <div className="space-y-5">
      <h1 className="text-xl font-semibold tracking-wider text-slate-200">{t("agents.title")}</h1>

      <Card className="bg-slate-900/30">
        <CardContent className="grid gap-4 p-5 md:grid-cols-[auto_1fr_1fr_1fr] md:items-center">
          <div className="flex items-center gap-3">
            <span className={`h-5 w-5 rounded-full ${online ? "animate-pulse bg-accent shadow-[0_0_24px_rgb(45_212_191/0.7)]" : "bg-slate-500"}`} />
            <div>
              <p className="text-lg font-semibold text-slate-200">{online ? t("agents.online") : t("agents.offline")}</p>
              <p className="mt-1 text-xs text-slate-500">{t("agents.lastHeartbeat")}</p>
            </div>
          </div>
          <Info label={t("agents.lastHeartbeat")} value={formatDateTime(status?.last_heartbeat_at ?? status?.checked_at)} mono />
          <Info label={t("agents.version")} value={status?.agent_version ?? "-"} mono />
          <Info label={t("common.status")} value={online ? t("status.running") : t("status.stopped")} />
        </CardContent>
      </Card>

      <Card className="bg-slate-900/30">
        <CardHeader>
          <div>
            <CardTitle>{t("agents.configGuide")}</CardTitle>
            <CardDescription>{t("agents.localSecretHint")}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-3">
          <StepCard icon={<Download className="h-4 w-4" />} title={t("agents.step1")} body={t("agents.download")} />
          <StepCard icon={<FileCode2 className="h-4 w-4" />} title={t("agents.step2")} body={t("agents.localSecretHint")} code="config.agent.yaml" />
          <StepCard icon={<RadioTower className="h-4 w-4" />} title={t("agents.step3")} body={online ? t("agents.online") : t("agents.offline")} />
        </CardContent>
      </Card>

      <Card className="bg-slate-900/30">
        <CardHeader>
          <div>
            <CardTitle>{t("agents.apiCheck")}</CardTitle>
            <CardDescription>{t("agents.localSecretHint")}</CardDescription>
          </div>
          <KeyRound className="h-4 w-4 text-accent" />
        </CardHeader>
        <CardContent>
          <div className="inline-flex items-center gap-2 rounded-full border border-white/[0.06] bg-slate-950/35 px-3 py-2 text-sm">
            <CheckCircle2 className={`h-4 w-4 ${status?.api_configured ? "text-accent" : "text-slate-500"}`} />
            <span className={status?.api_configured ? "text-accent" : "text-slate-400"}>
              {status?.api_configured ? t("agents.configured") : t("agents.notConfigured")}
            </span>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function Info({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-lg border border-white/[0.04] bg-slate-950/35 p-3">
      <p className="text-xs text-slate-500">{label}</p>
      <p className={`mt-1 truncate text-sm text-slate-200 ${mono ? "font-mono" : ""}`}>{value}</p>
    </div>
  );
}

function StepCard({ icon, title, body, code }: { icon: JSX.Element; title: string; body: string; code?: string }) {
  return (
    <div className="rounded-xl border border-white/[0.04] bg-slate-950/35 p-4">
      <div className="flex h-9 w-9 items-center justify-center rounded-lg border border-accent/20 bg-accent/10 text-accent">{icon}</div>
      <h2 className="mt-4 text-sm font-semibold text-slate-200">{title}</h2>
      <p className="mt-2 text-sm leading-6 text-slate-500">{body}</p>
      {code && <code className="mt-3 block rounded-md bg-slate-900/80 p-3 text-xs text-accent">{code}</code>}
    </div>
  );
}
