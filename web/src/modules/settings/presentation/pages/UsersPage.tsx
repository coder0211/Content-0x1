import { useMemo, useState } from "react";
import { Ban, Lock, Pencil, Plus, Unlock, Users } from "lucide-react";
import type { UserStatus } from "@/shared/kernel";
import { formatDateTime, formatRelative } from "@/shared/lib/format";
import { ROLE_LABELS } from "@/shared/lib/labels";
import { useSession } from "@/shared/lib/session";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { ConfirmDialog, DataTable, EmptyState, PageHeader, StatusBadge, type DataTableColumn, type RowAction } from "@/shared/ui/patterns";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { toast } from "@/shared/ui/toast";
import { useUpdateUser, useUsers } from "../../application/useUsers";
import { statusActions, type ManagedUser, type StatusAction } from "../../domain/user";
import { CreateUserDialog } from "../components/CreateUserDialog";
import { EditUserDialog } from "../components/EditUserDialog";
import { NativeSelect } from "../components/NativeSelect";
import { RoleMatrix } from "../components/RoleMatrix";

const STATUS_ICON = { active: Unlock, locked: Lock, disabled: Ban } as const;

export default function UsersPage() {
  const { user: me } = useSession();
  const [q, setQ] = useState("");
  const [status, setStatus] = useState<UserStatus | "">("");
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<ManagedUser | null>(null);
  const [pending, setPending] = useState<{ user: ManagedUser; action: StatusAction } | null>(null);
  const query = useMemo(() => ({ q: q.trim() || undefined, status: status || undefined }), [q, status]);
  const list = useUsers(query);
  const update = useUpdateUser();

  const columns: DataTableColumn<ManagedUser>[] = [
    {
      id: "name",
      header: "User",
      cell: (u) => (
        <div className="flex flex-col">
          <span className="font-medium text-neutral-900">
            {u.name}
            {u.id === me?.id ? <span className="ml-2 text-caption font-normal text-neutral-500">(bạn)</span> : null}
          </span>
          <span className="text-small text-neutral-500">{u.email}</span>
        </div>
      ),
    },
    { id: "roles", header: "Vai trò", cell: (u) => u.roles.map((r) => ROLE_LABELS[r]).join(", ") },
    { id: "status", header: "Trạng thái", cell: (u) => <StatusBadge kind="user" status={u.status} />, width: 120 },
    {
      id: "last_login",
      header: "Đăng nhập gần nhất",
      cell: (u) =>
        u.lastLoginAt ? <span title={formatDateTime(u.lastLoginAt)}>{formatRelative(u.lastLoginAt)}</span> : <span className="text-neutral-500">Chưa đăng nhập</span>,
      width: 170,
    },
  ];

  const rowActions = (u: ManagedUser): RowAction<ManagedUser>[] => [
    { label: "Sửa tên & vai trò", icon: Pencil, onSelect: setEditing },
    ...statusActions(u, me?.id ?? null).map((a) => ({
      label: a.label,
      icon: STATUS_ICON[a.to],
      tone: a.tone === "danger" ? ("danger" as const) : ("default" as const),
      onSelect: () => setPending({ user: u, action: a }),
    })),
  ];

  const filtered = Boolean(query.q || query.status);

  return (
    <>
      <PageHeader
        title="Người dùng & vai trò"
        description="Tạo tài khoản, gán vai trò, khóa user. Khóa hoặc vô hiệu hóa sẽ thu hồi mọi phiên đăng nhập ngay."
        actions={
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Plus />
            Tạo user
          </Button>
        }
      />
      <Tabs defaultValue="users">
        <TabsList>
          <TabsTrigger value="users">Người dùng</TabsTrigger>
          <TabsTrigger value="roles">Vai trò & quyền</TabsTrigger>
        </TabsList>
        <TabsContent value="users" className="flex flex-col gap-4 pt-4">
          <div className="flex items-center gap-2">
            <Input
              type="search"
              aria-label="Tìm theo tên hoặc email"
              placeholder="Tìm theo tên hoặc email"
              className="max-w-xs"
              value={q}
              onChange={(e) => setQ(e.target.value)}
            />
            <NativeSelect aria-label="Trạng thái" className="w-44" value={status} onChange={(e) => setStatus(e.target.value as UserStatus | "")}>
              <option value="">Mọi trạng thái</option>
              <option value="active">Hoạt động</option>
              <option value="locked">Tạm khóa</option>
              <option value="disabled">Vô hiệu</option>
            </NativeSelect>
          </div>
          <DataTable
            aria-label="Danh sách user"
            columns={columns}
            rows={list.users}
            getRowId={(u) => u.id}
            isLoading={list.isLoading}
            error={list.error}
            onRetry={() => void list.refetch()}
            rowActions={rowActions}
            empty={
              filtered ? (
                <EmptyState icon={Users} message="Không có user khớp bộ lọc." />
              ) : (
                <EmptyState
                  icon={Users}
                  message="Chưa có user nào."
                  action={
                    <Button variant="secondary" onClick={() => setCreating(true)}>
                      Tạo user
                    </Button>
                  }
                />
              )
            }
            footer={
              list.hasNextPage ? (
                <Button variant="secondary" className="self-center" loading={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>
                  Tải thêm
                </Button>
              ) : null
            }
          />
        </TabsContent>
        <TabsContent value="roles" className="pt-4">
          <RoleMatrix />
        </TabsContent>
      </Tabs>

      <CreateUserDialog open={creating} onOpenChange={setCreating} />
      {editing ? <EditUserDialog key={editing.id} user={editing} isSelf={editing.id === me?.id} onClose={() => setEditing(null)} /> : null}
      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(o) => !o && setPending(null)}
        title={pending?.action.confirmTitle ?? ""}
        description={pending?.action.description}
        confirmLabel={pending?.action.label ?? ""}
        tone={pending?.action.tone ?? "primary"}
        onConfirm={async () => {
          if (!pending) return;
          await update.mutateAsync({ id: pending.user.id, input: { status: pending.action.to } });
          toast.success(`${pending.action.label}: ${pending.user.name}`);
        }}
      />
    </>
  );
}
