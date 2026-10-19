import { useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ArrowLeft, History, SearchX } from "lucide-react";
import { isApiError } from "@/shared/kernel";
import { describeErrorKind } from "@/shared/lib/errors";
import { formatDateTime, formatDurationMs } from "@/shared/lib/format";
import { useSession } from "@/shared/lib/session";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Skeleton } from "@/shared/ui/skeleton";
import { DataTable, EmptyState, ErrorState, JobProgress, PageHeader, StatusBadge, type DataTableColumn } from "@/shared/ui/patterns";
import { ConfirmDialog } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";
import { useJobHistory } from "../../application/useJobs";
import { useJobProgress } from "../../application/useJobProgress";
import { JOB_TYPE_LABELS, stepLabel, type JobHistoryEvent, type JobStep } from "../../domain/job";
import { JobCostCell } from "../components/JobCostCell";
import { creatorText, providerText } from "../components/format";

const EVENT_LABELS: Record<string, string> = {
  created: "Tạo job",
  status_changed: "Đổi trạng thái",
  step_status_changed: "Đổi trạng thái bước",
  retry_scheduled: "Hẹn thử lại",
  provider_error: "Lỗi nhà cung cấp",
  cancel_requested: "Yêu cầu hủy",
  retried: "Chạy lại",
  lease_expired: "Hết hạn lease",
  concurrency_wait: "Chờ slot đồng thời",
  reconciled: "Đối soát",
};

function eventSummary(e: JobHistoryEvent): string {
  const p = e.payload;
  const parts: string[] = [];
  if (typeof p.step_key === "string") parts.push(p.step_key);
  if (typeof p.from === "string" && typeof p.to === "string") parts.push(`${p.from} → ${p.to}`);
  else if (typeof p.to === "string") parts.push(`→ ${p.to}`);
  else if (typeof p.status === "string") parts.push(p.status);
  if (typeof p.error_kind === "string") parts.push(p.error_kind);
  if (typeof p.attempt === "number") parts.push(`lần ${p.attempt}`);
  if (typeof p.message === "string") parts.push(p.message);
  return parts.join(" · ");
}

function stepDuration(s: JobStep): string {
  if (!s.startedAt || !s.finishedAt) return "—";
  return formatDurationMs(Date.parse(s.finishedAt) - Date.parse(s.startedAt));
}

const stepColumns: DataTableColumn<JobStep>[] = [
  { id: "name", header: "Bước", cell: (s) => <span className="font-medium text-neutral-900">{stepLabel(s)}</span> },
  { id: "key", header: "Mã", cell: (s) => <code className="text-small text-neutral-500">{s.key}</code> },
  { id: "status", header: "Trạng thái", cell: (s) => <StatusBadge kind="step" status={s.status} /> },
  { id: "attempts", header: "Số lần chạy", align: "right", width: 100, cell: (s) => s.attemptCount },
  { id: "duration", header: "Thời gian", align: "right", width: 100, cell: stepDuration },
  {
    id: "error",
    header: "Lỗi",
    cell: (s) =>
      s.errorKind || s.errorMessage ? (
        <span className="text-small text-danger">
          {describeErrorKind(s.errorKind, s.errorMessage ?? undefined)}
          {s.errorMessage && s.errorKind !== "InvalidRequest" ? <span className="block text-neutral-500">{s.errorMessage}</span> : null}
        </span>
      ) : (
        <span className="text-neutral-400">—</span>
      ),
  },
];

const eventColumns: DataTableColumn<JobHistoryEvent>[] = [
  { id: "at", header: "Thời điểm", width: 160, cell: (e) => <span className="tabular text-neutral-500">{formatDateTime(e.createdAt)}</span> },
  { id: "type", header: "Sự kiện", cell: (e) => EVENT_LABELS[e.type] ?? e.type },
  { id: "detail", header: "Chi tiết", cell: (e) => <span className="text-neutral-700">{eventSummary(e) || "—"}</span> },
];

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-caption text-neutral-500">{label}</dt>
      <dd className="text-body text-neutral-900">{children}</dd>
    </div>
  );
}

/** Job detail (ACS-091): live progress, steps, events, errors, Hủy / Thử lại. */
export default function JobDetailPage() {
  const { jobId } = useParams<{ jobId: string }>();
  const { user } = useSession();
  const navigate = useNavigate();
  const p = useJobProgress(jobId, {
    onFinished: (j) => {
      if (j.status === "succeeded") toast.success(`${JOB_TYPE_LABELS[j.type] ?? j.type}: đã hoàn thành`);
    },
  });
  const history = useJobHistory(jobId);
  const [confirmCancel, setConfirmCancel] = useState(false);
  const job = p.job;

  if (p.error) {
    if (isApiError(p.error) && p.error.code === "NOT_FOUND") {
      return (
        <>
          <PageHeader title="Job" />
          <EmptyState
            icon={SearchX}
            message="Không tìm thấy job này hoặc bạn không có quyền xem."
            action={
              <Button variant="secondary" onClick={() => void navigate("/jobs")}>
                Về danh sách job
              </Button>
            }
          />
        </>
      );
    }
    return <ErrorState error={p.error} onRetry={p.refetch} />;
  }

  if (!job) {
    return (
      <div aria-busy className="flex flex-col gap-4">
        <Skeleton className="h-7 w-64" />
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  const isOwnerOrAdmin = job.createdBy === user?.id || !!user?.roles.includes("admin");
  const assetIds = Array.isArray(job.result?.asset_ids) ? (job.result.asset_ids as string[]) : [];

  return (
    <>
      <Link to="/jobs" className="mb-2 inline-flex items-center gap-1 text-small text-neutral-500 hover:text-neutral-900">
        <ArrowLeft className="size-4" aria-hidden />
        Danh sách job
      </Link>
      <PageHeader
        title={JOB_TYPE_LABELS[job.type] ?? job.type}
        meta={<StatusBadge kind="job" status={job.status} />}
        description={<span className="tabular">{job.id}</span>}
      />

      {job.needsReconciliation ? (
        <Banner tone="warning" className="mb-4" title="Cần đối soát">
          Không xác định được kết quả ở nhà cung cấp. Admin cần kiểm tra trước khi chạy lại để tránh tính phí hai lần.
        </Banner>
      ) : null}

      <section className="mb-6 rounded-lg border border-neutral-200 bg-neutral-0 p-4" aria-label="Tiến độ">
        <JobProgress
          status={job.status}
          progress={job.progress}
          label={p.label}
          errorKind={p.errorKind}
          errorDetail={p.errorDetail}
          onCancel={isOwnerOrAdmin && p.canCancel ? () => setConfirmCancel(true) : undefined}
          onRetry={isOwnerOrAdmin && p.canRetry ? () => void p.retry().catch(() => undefined) : undefined}
          cancelling={p.isCancelling}
          retrying={p.isRetrying}
        />
        {job.error && job.status !== "cancelled" ? (
          <p className="mt-2 text-small text-neutral-500">
            Mã lỗi <code>{job.error.code}</code>: {job.error.message}
          </p>
        ) : null}
        {p.mode === "polling" ? <p className="mt-2 text-caption text-neutral-500">Đang cập nhật mỗi 3 giây.</p> : null}
      </section>

      <dl className="mb-6 grid grid-cols-4 gap-4">
        <Field label="Nhà cung cấp">{providerText(job)}</Field>
        <Field label="Chi phí">
          <JobCostCell cost={job.cost} />
        </Field>
        <Field label="Người tạo">{creatorText(job.createdBy, user?.id)}</Field>
        <Field label="Tạo lúc">
          <span className="tabular">{formatDateTime(job.createdAt)}</span>
        </Field>
        <Field label="Bắt đầu">
          <span className="tabular">{job.startedAt ? formatDateTime(job.startedAt) : "—"}</span>
        </Field>
        <Field label="Kết thúc">
          <span className="tabular">{job.finishedAt ? formatDateTime(job.finishedAt) : "—"}</span>
        </Field>
        <Field label="Campaign">{job.campaignId ? <span className="tabular">{job.campaignId.slice(0, 8)}</span> : "—"}</Field>
        <Field label="Kết quả">{assetIds.length > 0 ? `${assetIds.length} asset` : "—"}</Field>
      </dl>

      <section className="mb-6" aria-labelledby="job-steps">
        <h2 id="job-steps" className="mb-2 text-section text-neutral-900">
          Các bước
        </h2>
        <DataTable
          aria-label="Các bước"
          columns={stepColumns}
          rows={job.steps}
          getRowId={(s) => s.id}
          empty={<EmptyState icon={History} message="Job này chưa có bước nào." />}
        />
      </section>

      <section aria-labelledby="job-events">
        <h2 id="job-events" className="mb-2 text-section text-neutral-900">
          Nhật ký sự kiện
        </h2>
        <DataTable
          aria-label="Nhật ký sự kiện"
          columns={eventColumns}
          rows={history.data}
          getRowId={(e) => String(e.id)}
          isLoading={history.isLoading}
          error={history.error}
          onRetry={() => void history.refetch()}
          empty={<EmptyState icon={History} message="Chưa có sự kiện nào." />}
        />
      </section>

      <ConfirmDialog
        open={confirmCancel}
        onOpenChange={setConfirmCancel}
        title="Hủy job này?"
        description="Bước đang chạy sẽ hoàn tất, các bước còn lại bị hủy. Chi phí đã phát sinh vẫn được tính."
        confirmLabel="Hủy job"
        cancelLabel="Giữ lại"
        tone="danger"
        onConfirm={async () => {
          await p.cancel();
          setConfirmCancel(false);
        }}
      />
    </>
  );
}
