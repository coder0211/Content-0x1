import { NavLink } from "react-router";
import { cn } from "@/shared/lib/cn";

const LINKS = [
  { to: "/usage", label: "Chi phí", end: true },
  { to: "/usage/budgets", label: "Ngân sách", end: false },
];

/** Section tabs for /usage (route-based, so each tab is linkable). */
export function UsageSubnav() {
  return (
    <nav aria-label="Mục chi phí" className="mb-4 flex items-center gap-4 border-b border-neutral-200">
      {LINKS.map((l) => (
        <NavLink
          key={l.to}
          to={l.to}
          end={l.end}
          className={({ isActive }) =>
            cn(
              "-mb-px inline-flex h-9 items-center border-b-2 text-body font-medium transition-colors duration-150",
              isActive ? "border-accent-600 text-neutral-900" : "border-transparent text-neutral-500 hover:text-neutral-900",
            )
          }
        >
          {l.label}
        </NavLink>
      ))}
    </nav>
  );
}
