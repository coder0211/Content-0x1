import { useState } from "react";
import { useNavigate } from "react-router";
import { Bell } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import { formatRelative } from "@/shared/lib/format";
import { Button } from "@/shared/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui/popover";
import { useMarkNotificationRead, useNotifications } from "../../application/useDashboardSummary";
import { notificationLink, notificationText, type AppNotification } from "../../domain/notification";

/** Topbar bell (GET /notifications): unread count, latest items; opening one marks it read. */
export function NotificationBell() {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const list = useNotifications();
  const markRead = useMarkNotificationRead();
  const items = list.data ?? [];
  const unread = items.filter((n) => !n.readAt).length;

  const openItem = (n: AppNotification) => {
    if (!n.readAt) markRead.mutate(n.id);
    const to = notificationLink(n);
    setOpen(false);
    if (to) void navigate(to);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={unread ? `Thông báo (${unread} chưa đọc)` : "Thông báo"} className="relative">
          <Bell />
          {unread ? (
            <span
              aria-hidden
              className="absolute right-0.5 top-0.5 min-w-4 rounded bg-danger px-1 text-center text-caption leading-4 text-neutral-0"
            >
              {unread > 9 ? "9+" : unread}
            </span>
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 p-0" aria-label="Thông báo">
        <p className="border-b border-neutral-200 px-4 py-2 text-body font-medium text-neutral-900">Thông báo</p>
        {list.isLoading ? (
          <p className="px-4 py-3 text-body text-neutral-500">Đang tải…</p>
        ) : list.error ? (
          <p className="px-4 py-3 text-body text-neutral-500">Không tải được thông báo.</p>
        ) : items.length === 0 ? (
          <p className="px-4 py-3 text-body text-neutral-500">Chưa có thông báo nào.</p>
        ) : (
          <ul aria-label="Danh sách thông báo" className="max-h-96 overflow-y-auto">
            {items.map((n) => (
              <li key={n.id}>
                <button
                  type="button"
                  onClick={() => openItem(n)}
                  className={cn(
                    "flex w-full flex-col gap-0.5 border-b border-neutral-100 px-4 py-2 text-left hover:bg-neutral-50",
                    !n.readAt && "bg-accent-50",
                  )}
                >
                  <span className="text-body text-neutral-900">{notificationText(n)}</span>
                  {typeof n.payload.comment === "string" && n.payload.comment ? (
                    <span className="line-clamp-2 text-small text-neutral-600">“{n.payload.comment}”</span>
                  ) : null}
                  <span className="text-small text-neutral-500">
                    {!n.readAt ? "Chưa đọc · " : ""}
                    {formatRelative(n.createdAt)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  );
}
