import { FormEvent, useState } from "react";
import { Link, Navigate, useNavigate } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { AuthScaffold } from "../../components/layout/AuthScaffold";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import { Label } from "../../components/ui/label";
import { useI18n } from "../../i18n/useI18n";
import { authService, ApiRequestError } from "../../shared/services";
import { useAuth } from "./AuthProvider";

export function LoginPage() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const { token, login } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const mutation = useMutation({
    mutationFn: () => authService.login(email, password),
    onSuccess: (data) => {
      login(data.token, data.user);
      navigate("/", { replace: true });
    }
  });

  if (token) return <Navigate to="/" replace />;

  function onSubmit(event: FormEvent) {
    event.preventDefault();
    mutation.mutate();
  }

  const disabled = mutation.isPending;
  const error = mutation.error instanceof ApiRequestError ? mutation.error.message : null;

  return (
    <AuthScaffold title={t("auth.login.title")}>
      <form className="space-y-4" onSubmit={onSubmit}>
        <div className="space-y-2">
          <Label htmlFor="email" className="text-xs uppercase tracking-wider text-slate-400">
            {t("auth.email")}
          </Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            value={email}
            disabled={disabled}
            onChange={(event) => setEmail(event.target.value)}
            required
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="password" className="text-xs uppercase tracking-wider text-slate-400">
            {t("auth.password")}
          </Label>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            value={password}
            disabled={disabled}
            onChange={(event) => setPassword(event.target.value)}
            required
          />
        </div>
        <Button className="w-full uppercase tracking-wider" variant="primary" type="submit" disabled={disabled}>
          {disabled && <Loader2 className="h-4 w-4 animate-spin" />}
          {t("auth.login.action")}
        </Button>
        {error && <p className="text-center text-xs leading-5 text-red-300">{error}</p>}
        <Link className="block text-center text-xs tracking-wider text-accent hover:text-accent/80" to="/register">
          {t("auth.toRegister")}
        </Link>
      </form>
    </AuthScaffold>
  );
}
