import { useMemo, useState } from "react";
import { ScrollText } from "lucide-react";
import { formatDateTime } from "@/shared/lib/format";
import { useSession } from "@/shared/lib/session";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { DataTable, EmptyState, PageHeader, type DataTableColumn } from "@/shared/ui/patterns";
import { Sheet, SheetBody, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { useAuditLogs } from "../../application/useAuditLogs";
import { useUsers } from "../../application/useUsers";
import { RESOURCE_TYPES, changedKeys, formatJson, isDateRangeValid, toAuditQuery, type AuditEntry, type AuditFilter } from "../../domain/audit";
import { NativeSelect } from "../components/NativeSelect";

/** /settings/audit — read-only audit log with filters + before/after detail (ACS-094). */
export default function AuditLogPage() {
  const { can } = useSession();
  const [filter, setFilter] = useState<AuditFilter>({});
  const [open, setOpen] = useState<AuditEntry | null>(null);
  const rangeOk = isDateRangeValid(filter);
  const query = useMemo(() => toAuditQuery(rangeOk ? filter : { ...filter, fromDate: undefined, toDate: undefined }), [filter, rangeOk]);
  const list = useAuditLogs(query);
  const canListUsers = can("user.manage");
  const users = useUsers({}, { enabled: canListUsers });

  const set = (patch: Partial<AuditFilter>) => setFilter((f) => ({ ...f, ...patch }));
  const filtered = Object.values(filter).some(Boolean);

  const columns: DataTableColumn<AuditEntry>[] = [
    { id: "time", header: "Thời gian", cell: (e) => <span className="tabular">{formatDateTime(e.createdAt)}</span>, width: 170 },
    { id: "actor", header: "Người thực hiện", cell: (e) => e.actorEmail || <span className="text-neutral-500">Hệ thống</span> },
    { id: "action", header: "Hành động", cell: (e) => <code className="font-mono text-small">{e.action}</code> },
    {
      id: "resource",
      header: "Đối tượng",
      cell: (e) => (
        <span className="text-small">
          {e.resourceType}
          <span className="text-neutral-500"> · {e.resourceId}</span>
        </span>
      ),
    },
    { id: "ip", header: "IP", cell: (e) => <span className="tabular text-small text-neutral-500">{e.ip || "—"}</span>, width: 130 },
  ];

  return (
    <>
      <PageHeader title="Nhật ký hệ thống" description="Mọi thay đổi quan trọng: đăng nhập, phân quyền, duyệt asset, cấu hình provider, ngân sách. Chỉ đọc." />
      <div className="mb-4 flex flex-wrap items-end gap-2" role="group" aria-label="Bộ lọc nhật ký">
        {canListUsers ? (
          <NativeSelect aria-label="Người thực hiện" className="w-52" value={filter.actorId ?? ""} onChange={(e) => set({ actorId: e.target.value || undefined })}>
            <option value="">Mọi người</option>
            {(users.users ?? []).map((u) => (
              <option key={u.id} value={u.id}>
                {u.name} ({u.email})
              </option>
            ))}
          </NativeSelect>
        ) : null}
        <NativeSelect aria-label="Loại đối tượng" className="w-40" value={filter.resourceType ?? ""} onChange={(e) => set({ resourceType: e.target.value || undefined })}>
          <option value="">Mọi đối tượng</option>
          {RESOURCE_TYPES.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </NativeSelect>
        <Input
          aria-label="Hành động"
          placeholder="Hành động, vd. asset.approve"
          className="h-8 w-56"
          value={filter.action ?? ""}
          onChange={(e) => set({ action: e.target.value || undefined })}
        />
        <Input aria-label="Từ ngày" type="date" className="h-8 w-40" value={filter.fromDate ?? ""} onChange={(e) => set({ fromDate: e.target.value || undefined })} />
        <Input aria-label="Đến ngày" type="date" className="h-8 w-40" value={filter.toDate ?? ""} onChange={(e) => set({ toDate: e.target.value || undefined })} />
        {filtered ? (
          <Button size="sm" variant="ghost" onClick={() => setFilter({})}>
            Xóa bộ lọc
          </Button>
        ) : null}
      </div>
      {!rangeOk ? (
        <p role="alert" className="mb-3 text-small text-danger">
          "Từ ngày" phải trước hoặc bằng "Đến ngày". Bộ lọc ngày đang bị bỏ qua.
        </p>
      ) : null}
      <DataTable
        aria-label="Nhật ký hệ thống"
        columns={columns}
        rows={list.entries}
        getRowId={(e) => String(e.id)}
        isLoading={list.isLoading}
        error={list.error}
        onRetry={() => void list.refetch()}
        onRowClick={setOpen}
        selectedId={open ? String(open.id) : null}
        empty={<EmptyState icon={ScrollText} message={filtered ? "Không có bản ghi khớp bộ lọc." : "Chưa có bản ghi nào."} />}
        footer={
          list.hasNextPage ? (
            <Button variant="secondary" className="self-center" loading={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>
              Tải thêm
            </Button>
          ) : null
        }
      />
      <Sheet open={open !== null} onOpenChange={(o) => !o && setOpen(null)}>
        <SheetContent>{open ? <AuditDetail entry={open} /> : null}</SheetContent>
      </Sheet>
    </>
  );
}

function AuditDetail({ entry }: { entry: AuditEntry }) {
  const changed = changedKeys(entry.before, entry.after);
  return (
    <>
      <SheetHeader>
        <SheetTitle>
          <code className="font-mono">{entry.action}</code>
        </SheetTitle>
        <SheetDescription className="text-small text-neutral-500">
          {formatDateTime(entry.createdAt)} · {entry.actorEmail || "Hệ thống"}
        </SheetDescription>
      </SheetHeader>
      <SheetBody className="flex flex-col gap-4">
        <dl className="grid grid-cols-[120px_1fr] gap-x-3 gap-y-1 text-small">
          <dt className="text-neutral-500">Đối tượng</dt>
          <dd className="break-all text-neutral-900">
            {entry.resourceType} · {entry.resourceId}
          </dd>
          <dt className="text-neutral-500">IP</dt>
          <dd className="text-neutral-900">{entry.ip || "—"}</dd>
          <dt className="text-neutral-500">Request ID</dt>
          <dd className="break-all font-mono text-neutral-900">{entry.requestId || "—"}</dd>
          <dt className="text-neutral-500">Trường thay đổi</dt>
          <dd className="text-neutral-900">{changed.length ? changed.join(", ") : "—"}</dd>
        </dl>
        <JsonBlock label="Trước" value={entry.before} />
        <JsonBlock label="Sau" value={entry.after} />
      </SheetBody>
    </>
  );
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <section className="flex flex-col gap-1">
      <h3 className="text-caption font-medium uppercase text-neutral-500">{label}</h3>
      <pre className="max-h-64 overflow-auto rounded border border-neutral-200 bg-neutral-50 p-3 font-mono text-small text-neutral-900">{formatJson(value)}</pre>
    </section>
  );
}
