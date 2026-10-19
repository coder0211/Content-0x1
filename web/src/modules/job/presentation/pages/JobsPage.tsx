import { useMemo } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { Activity, RotateCcw, X } from "lucide-react";
import type { JobStatus } from "@/shared/kernel";
import { formatDateTime, formatRelative } from "@/shared/lib/format";
import { useCan, useSession } from "@/shared/lib/session";
import { Badge } from "@/shared/ui/badge";
import { Button } from "@/shared/ui/button";
import { DataTable, EmptyState, PageHeader, StatusBadge, type DataTableColumn } from "@/shared/ui/patterns";
import { Tooltip } from "@/shared/ui/tooltip";
import { useJobActions, useJobs } from "../../application/useJobs";
import { canCancel, canRetry, JOB_TYPE_LABELS, type JobFilter, type JobSummary, type JobType } from "../../domain/job";
import { JobCostCell } from "../components/JobCostCell";
import { JobFilters } from "../components/JobFilters";
import { creatorText, providerText } from "../components/format";

function readFilter(sp: URLSearchParams): JobFilter {
  return {
    status: (sp.get("status") as JobStatus | null) ?? undefined,
    type: (sp.get("type") as JobType | null) ?? undefined,
    providerCode: sp.get("provider") ?? undefined,
    createdBy: sp.get("user") ?? undefined,
  };
}

/** Job monitor (ACS-091): list with filters; row → detail. */
export default function JobsPage() {
  const navigate = useNavigate();
  const [sp, setSp] = useSearchParams();
  const filter = useMemo(() => readFilter(sp), [sp]);
  const { user } = useSession();
  const canSeeAll = useCan("provider.view");
  const q = useJobs(filter);
  const { cancel, retry } = useJobActions();
  const rows = q.data?.pages.flatMap((p) => p.items);
  const filtered = !!(filter.status || filter.type || filter.providerCode || filter.createdBy);

  const setFilter = (f: JobFilter) => {
    const next = new URLSearchParams();
    if (f.status) next.set("status", f.status);
    if (f.type) next.set("type", f.type);
    if (f.providerCode) next.set("provider", f.providerCode);
    if (f.createdBy) next.set("user", f.createdBy);
    setSp(next, { replace: true });
  };

  const columns: DataTableColumn<JobSummary>[] = [
    {
      id: "type",
      header: "Loại",
      cell: (j) => (
        <span className="flex items-center gap-2">
          <span className="font-medium text-neutral-900">{JOB_TYPE_LABELS[j.type] ?? j.type}</span>
          {j.needsReconciliation ? <Badge tone="warning">Cần đối soát</Badge> : null}
        </span>
      ),
    },
    {
      id: "status",
      header: "Trạng thái",
      cell: (j) => (
        <span className="flex items-center gap-2">
          <StatusBadge kind="job" status={j.status} />
          {j.cancelRequested && (j.status === "running" || j.status === "queued") ? (
            <span className="text-caption text-neutral-500">Đang hủy</span>
          ) : null}
        </span>
      ),
    },
    { id: "progress", header: "Tiến độ", align: "right", width: 80, cell: (j) => `${j.progress}%` },
    { id: "provider", header: "Nhà cung cấp", cell: (j) => <span className="text-neutral-700">{providerText(j)}</span> },
    ...(canSeeAll
      ? [{ id: "user", header: "Người tạo", cell: (j: JobSummary) => creatorText(j, user?.id) } satisfies DataTableColumn<JobSummary>]
      : []),
    { id: "cost", header: "Chi phí", align: "right", cell: (j) => <JobCostCell cost={j.cost} /> },
    {
      id: "created",
      header: "Tạo lúc",
      cell: (j) => (
        <Tooltip content={formatDateTime(j.createdAt)}>
          <span className="text-neutral-500">{formatRelative(j.createdAt)}</span>
        </Tooltip>
      ),
    },
  ];

  return (
    <>
      <PageHeader title="Job" description="Theo dõi các tác vụ sinh nội dung, ảnh và video." />
      <JobFilters value={filter} onChange={setFilter} canSeeAll={canSeeAll} currentUserId={user?.id} />
      <DataTable
        aria-label="Danh sách job"
        columns={columns}
        rows={rows}
        getRowId={(j) => j.id}
        isLoading={q.isLoading}
        error={q.error}
        onRetry={() => void q.refetch()}
        onRowClick={(j) => void navigate(`/jobs/${j.id}`)}
        rowActions={(j) => [
          ...(canCancel(j) && (j.createdBy === user?.id || user?.roles.includes("admin"))
            ? [{ label: "Hủy job", icon: X, tone: "danger" as const, onSelect: () => cancel.mutate(j.id) }]
            : []),
          ...(canRetry(j) && (j.createdBy === user?.id || user?.roles.includes("admin"))
            ? [{ label: "Thử lại", icon: RotateCcw, onSelect: () => retry.mutate({ id: j.id }) }]
            : []),
        ]}
        empty={
          filtered ? (
            <EmptyState
              icon={Activity}
              message="Không có job nào khớp bộ lọc."
              action={
                <Button variant="secondary" onClick={() => setFilter({})}>
                  Xóa bộ lọc
                </Button>
              }
            />
          ) : (
            <EmptyState icon={Activity} message="Chưa có job nào. Job xuất hiện khi bạn sinh nội dung, ảnh hoặc video." />
          )
        }
        footer={
          q.hasNextPage ? (
            <div className="flex justify-center">
              <Button variant="secondary" onClick={() => void q.fetchNextPage()} loading={q.isFetchingNextPage}>
                Tải thêm
              </Button>
            </div>
          ) : null
        }
      />
    </>
  );
}
