import { NavLink, Outlet } from "react-router";
import { cn } from "@/shared/lib/cn";
import { useSession } from "@/shared/lib/session";
import { SETTINGS_SECTIONS } from "../components/sections";

/** /settings shell: left section nav (only sections the user may open) + section content. */
export default function SettingsLayout() {
  const { can } = useSession();
  const sections = SETTINGS_SECTIONS.filter((s) => can(s.permission));
  return (
    <div className="flex gap-8">
      <nav aria-label="Mục cài đặt" className="w-52 shrink-0">
        <p className="mb-2 px-2 text-caption font-medium uppercase text-neutral-500">Cài đặt</p>
        <ul className="flex flex-col gap-0.5">
          {sections.map((s) => (
            <li key={s.to}>
              <NavLink
                to={s.to}
                className={({ isActive }) =>
                  cn(
                    "flex h-8 items-center gap-2 rounded px-2 text-body text-neutral-700 hover:bg-neutral-100 hover:text-neutral-900",
                    isActive && "bg-neutral-100 font-medium text-neutral-900",
                  )
                }
              >
                <s.icon className="size-4 shrink-0" aria-hidden />
                {s.label}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
      <div className="min-w-0 flex-1">
        <Outlet />
      </div>
    </div>
  );
}
