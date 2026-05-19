import { Globe2, UserRound } from "lucide-react";
import { PageHeader } from "../components/layout/PageHeader";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { useAuthStore } from "../stores/authStore";
import { useI18n } from "../i18n/useI18n";

export function SettingsPage() {
  const user = useAuthStore((state) => state.user);
  const { locale, setLocale, t } = useI18n();

  return (
    <div>
      <PageHeader title={t("settings.title")} description={t("settings.language")} />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t("settings.account")}</CardTitle>
              <CardDescription>订阅状态用于后端配额校验。</CardDescription>
            </div>
            <UserRound className="h-4 w-4 text-accent" />
          </CardHeader>
          <CardContent className="space-y-3">
            <SettingLine label="邮箱" value={user?.email ?? "暂无"} />
            <SettingLine label="订阅计划" value={user?.subscription_plan ?? "free"} />
            <div className="flex items-center justify-between gap-3 rounded-md bg-white/[0.025] p-3">
              <span className="text-xs text-text-muted">订阅状态</span>
              <Badge tone={user?.subscription_status === "active" ? "success" : "warning"}>
                {user?.subscription_status === "active" ? "生效中" : "需确认"}
              </Badge>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t("settings.language")}</CardTitle>
              <CardDescription>轻量本地化设置保存在浏览器本地。</CardDescription>
            </div>
            <Globe2 className="h-4 w-4 text-info" />
          </CardHeader>
          <CardContent className="flex gap-2">
            <Button variant={locale === "zh" ? "primary" : "secondary"} onClick={() => setLocale("zh")}>
              中文
            </Button>
            <Button variant={locale === "en" ? "primary" : "secondary"} onClick={() => setLocale("en")}>
              English
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function SettingLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-md bg-white/[0.025] p-3">
      <span className="text-xs text-text-muted">{label}</span>
      <span className="truncate text-sm text-text-main">{value}</span>
    </div>
  );
}
