import { NavLink } from "react-router";
import { PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import { useSession } from "@/shared/lib/session";
import { Tooltip } from "@/shared/ui/tooltip";
import { appModules } from "../modules";
import { visibleNav } from "./navigation";

export function Sidebar({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  const { can } = useSession();
  const groups = visibleNav(appModules, can);

  return (
    <aside
      className={cn(
        "flex h-screen shrink-0 flex-col border-r border-neutral-200 bg-neutral-50 transition-[width] duration-150",
        collapsed ? "w-sidebar-collapsed" : "w-sidebar",
      )}
      aria-label="Điều hướng chính"
    >
      <div className={cn("flex h-topbar items-center border-b border-neutral-200", collapsed ? "justify-center" : "px-4")}>
        {collapsed ? (
          <span className="text-section text-neutral-900">A</span>
        ) : (
          <span className="truncate text-body font-semibold text-neutral-900">AI Content Studio</span>
        )}
      </div>
      <nav className="flex-1 overflow-y-auto px-2 py-3">
        {groups.map((g) => (
          <div key={g.id} className="mb-4">
            {collapsed ? null : <p className="px-2 pb-1 text-caption text-neutral-500">{g.label}</p>}
            <ul className="flex flex-col gap-0.5">
              {g.items.map((item) => {
                const link = (
                  <NavLink
                    to={item.to}
                    end={item.to === "/"}
                    className={({ isActive }) =>
                      cn(
                        "flex h-8 items-center gap-2 rounded px-2 text-body transition-colors duration-150",
                        collapsed && "justify-center",
                        isActive
                          ? "bg-accent-50 font-medium text-accent-700"
                          : "text-neutral-700 hover:bg-neutral-100 hover:text-neutral-900",
                      )
                    }
                    aria-label={collapsed ? item.label : undefined}
                  >
                    <item.icon className="size-4 shrink-0" strokeWidth={1.5} aria-hidden />
                    {collapsed ? null : <span className="flex-1 truncate">{item.label}</span>}
                    {!collapsed && item.Badge ? <item.Badge /> : null}
                  </NavLink>
                );
                return (
                  <li key={item.to}>
                    {collapsed ? (
                      <Tooltip content={item.label} side="right">
                        {link}
                      </Tooltip>
                    ) : (
                      link
                    )}
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>
      <div className="border-t border-neutral-200 p-2">
        <button
          type="button"
          onClick={onToggle}
          className={cn(
            "flex h-8 w-full items-center gap-2 rounded px-2 text-body text-neutral-500 hover:bg-neutral-100 hover:text-neutral-900",
            collapsed && "justify-center",
          )}
          aria-label={collapsed ? "Mở rộng thanh bên" : "Thu gọn thanh bên"}
        >
          {collapsed ? <PanelLeftOpen className="size-4" /> : <PanelLeftClose className="size-4" />}
          {collapsed ? null : "Thu gọn"}
        </button>
      </div>
    </aside>
  );
}
