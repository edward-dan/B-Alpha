import { Navigate, Route, Routes } from "react-router-dom";
import { Component } from "react";
import type { ErrorInfo, ReactNode } from "react";
import { AppShell } from "./components/layout/AppShell";
import { hasFeature } from "./shared/config/features";
import { useAuthStore } from "./stores/authStore";
import { useSystemStatusStore } from "./stores/systemStatusStore";
import type { AppFeature } from "./types/api";
import { AgentsPage } from "./features/agents/AgentsPage";
import { BacktestingPage } from "./features/backtesting/BacktestingPage";
import { DashboardPage } from "./features/dashboard/DashboardPage";
import { LoginPage } from "./features/auth/LoginPage";
import { RegisterPage } from "./features/auth/RegisterPage";
import { EvolutionPage } from "./features/strategies/EvolutionPage";
import { InstanceCreatePage } from "./features/strategies/InstanceCreatePage";
import { InstanceListPage } from "./features/strategies/InstanceListPage";
import { TemplatesPage } from "./features/strategies/TemplatesPage";
import { SettingsPage } from "./pages/SettingsPage";

class AppErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error("B-Alpha UI crashed", error, errorInfo);
  }

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div className="flex min-h-screen items-center justify-center bg-background p-6">
        <div className="max-w-md rounded-lg border border-red-400/20 bg-red-400/10 p-5 text-sm text-red-100">
          <p className="font-semibold">页面加载失败</p>
          <p className="mt-2 text-red-200/80">{this.state.error.message || "请刷新页面后重试。"}</p>
        </div>
      </div>
    );
  }
}

function AppLoading() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background text-sm text-slate-400">
      正在载入...
    </div>
  );
}

function ProtectedRoute({ children }: { children: ReactNode }) {
  const token = useAuthStore((state) => state.token);
  const loading = useAuthStore((state) => state.loading);
  if (loading) return <AppLoading />;
  if (!token) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

function FeatureRoute({ feature, children }: { feature: AppFeature; children: ReactNode }) {
  const status = useSystemStatusStore((state) => state.status);
  if (!hasFeature(feature, status)) return <Navigate to="/" replace />;
  return <>{children}</>;
}

export function App() {
  return (
    <AppErrorBoundary>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route
          element={
            <ProtectedRoute>
              <AppShell />
            </ProtectedRoute>
          }
        >
          <Route index element={<DashboardPage />} />
          <Route
            path="/templates"
            element={
              <FeatureRoute feature="strategies">
                <TemplatesPage />
              </FeatureRoute>
            }
          />
          <Route path="/instances" element={<InstanceListPage />} />
          <Route path="/instances/new" element={<InstanceCreatePage />} />
          <Route
            path="/evolution"
            element={
              <FeatureRoute feature="strategies">
                <EvolutionPage />
              </FeatureRoute>
            }
          />
          <Route
            path="/agents"
            element={
              <FeatureRoute feature="agents">
                <AgentsPage />
              </FeatureRoute>
            }
          />
          <Route path="/backtesting" element={<BacktestingPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </AppErrorBoundary>
  );
}
