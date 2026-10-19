import { Fragment } from "react";
import { Link, useMatches } from "react-router";
import { Bell, ChevronRight, LogOut, Search } from "lucide-react";
import { ROLE_LABELS } from "@/shared/lib/labels";
import type { RouteHandle } from "@/shared/lib/module";
import { useSession } from "@/shared/lib/session";
import { Avatar } from "@/shared/ui/avatar";
import { Button } from "@/shared/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu";
import { Kbd } from "@/shared/ui/kbd";
import { Tooltip } from "@/shared/ui/tooltip";

function Breadcrumb() {
  const crumbs = useMatches()
    .map((m) => ({ path: m.pathname, crumb: (m.handle as RouteHandle | undefined)?.crumb }))
    .filter((c): c is { path: string; crumb: string } => Boolean(c.crumb));
  return (
    <nav aria-label="Breadcrumb" className="flex min-w-0 items-center gap-1 text-body">
      {crumbs.map((c, i) => (
        <Fragment key={c.path}>
          {i > 0 ? <ChevronRight className="size-4 text-neutral-400" aria-hidden /> : null}
          {i === crumbs.length - 1 ? (
            <span className="truncate font-medium text-neutral-900" aria-current="page">
              {c.crumb}
            </span>
          ) : (
            <Link to={c.path} className="truncate text-neutral-500 hover:text-neutral-900">
              {c.crumb}
            </Link>
          )}
        </Fragment>
      ))}
    </nav>
  );
}

export function Topbar() {
  const { user, logout } = useSession();
  return (
    <header className="flex h-topbar shrink-0 items-center gap-4 border-b border-neutral-200 bg-neutral-0 px-6">
      <div className="min-w-0 flex-1">
        <Breadcrumb />
      </div>
      <Tooltip content="Tìm kiếm nhanh (sắp có)">
        <span>
          <Button variant="secondary" size="sm" disabled className="w-56 justify-start text-neutral-400">
            <Search />
            <span className="flex-1 text-left">Tìm kiếm</span>
            <Kbd>⌘K</Kbd>
          </Button>
        </span>
      </Tooltip>
      <Tooltip content="Thông báo">
        <Button variant="ghost" size="icon" aria-label="Thông báo">
          <Bell />
        </Button>
      </Tooltip>
      {user ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" className="gap-2 px-1.5" aria-label={`Tài khoản ${user.name}`}>
              <Avatar name={user.name} />
              <span className="max-w-40 truncate">{user.name}</span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-60">
            <DropdownMenuLabel>
              <span className="block text-body font-medium text-neutral-900">{user.name}</span>
              <span className="block font-normal">{user.email}</span>
              <span className="block font-normal">{user.roles.map((r) => ROLE_LABELS[r] ?? r).join(", ")}</span>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => void logout()}>
              <LogOut />
              Đăng xuất
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </header>
  );
}
