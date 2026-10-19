import { Outlet } from "react-router";
import { RouteGuard } from "./RouteGuard";
import { Sidebar } from "./Sidebar";
import { Topbar } from "./Topbar";
import { useSidebarCollapsed } from "./useSidebarCollapsed";

/** SPEC-05 §4.1: sidebar 232px (56px collapsed) + topbar 48px + content padding 24px. */
export function AppShell() {
  const [collapsed, toggle] = useSidebarCollapsed();
  return (
    <div className="flex h-screen min-w-[1280px] bg-neutral-25">
      <Sidebar collapsed={collapsed} onToggle={toggle} />
      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar />
        <main className="flex-1 overflow-y-auto p-6">
          <RouteGuard>
            <Outlet />
          </RouteGuard>
        </main>
      </div>
    </div>
  );
}
