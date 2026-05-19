import { FormEvent, useState } from "react";
import { Link, Navigate, useNavigate } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { api, ApiRequestError } from "../lib/api";
import { AuthScaffold } from "../components/layout/AuthScaffold";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { useAuthStore } from "../stores/authStore";
import { useI18n } from "../i18n/useI18n";

export function RegisterPage() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const token = useAuthStore((state) => state.token);
  const setSession = useAuthStore((state) => state.setSession);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const mutation = useMutation({
    mutationFn: () => api.register(email, password),
    onSuccess: (data) => {
      setSession(data.token, data.user);
      navigate("/");
    }
  });

  if (token) return <Navigate to="/" replace />;

  function onSubmit(event: FormEvent) {
    event.preventDefault();
    mutation.mutate();
  }

  const error = mutation.error instanceof ApiRequestError ? mutation.error.message : null;

  return (
    <AuthScaffold title={t("auth.register.title")}>
      <form className="space-y-4" onSubmit={onSubmit}>
        <div className="space-y-2">
          <Label htmlFor="email">{t("auth.email")}</Label>
          <Input id="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="password">{t("auth.password")}</Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </div>
        {error && <p className="rounded-md border border-danger/20 bg-danger/10 p-3 text-xs text-danger">{error}</p>}
        <Button className="w-full" variant="primary" type="submit" disabled={mutation.isPending}>
          {mutation.isPending ? t("common.loading") : t("auth.register.action")}
        </Button>
        <Link className="block text-center text-xs text-accent hover:text-accent/80" to="/login">
          {t("auth.toLogin")}
        </Link>
      </form>
    </AuthScaffold>
  );
}
