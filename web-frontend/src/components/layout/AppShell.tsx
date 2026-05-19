import { Outlet } from "react-router-dom";
import { AppBackground } from "../../shared/ui/AppBackground";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";

export function AppShell() {
  return (
    <AppBackground>
      <div className="flex min-h-screen">
        <Sidebar />
        <div className="flex min-h-screen min-w-0 flex-1 flex-col">
          <Topbar />
          <main className="min-h-0 flex-1 overflow-y-auto p-4 lg:p-6">
            <div className="mx-auto max-w-[1800px]">
              <Outlet />
            </div>
          </main>
        </div>
      </div>
    </AppBackground>
  );
}
