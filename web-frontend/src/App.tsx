import { Navigate, Route, Routes } from "react-router-dom";
import type { ReactNode } from "react";
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

function ProtectedRoute({ children }: { children: ReactNode }) {
  const token = useAuthStore((state) => state.token);
  const loading = useAuthStore((state) => state.loading);
  if (loading) return <div className="min-h-screen bg-background" />;
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
        <Route
          path="/backtesting"
          element={<BacktestingPage />}
        />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
