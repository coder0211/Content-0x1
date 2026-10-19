import type { ReactNode } from "react";
import { Link } from "react-router";
import { Clapperboard, Image, ListChecks, PenLine, Workflow } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import { formatMoney, formatNumber, formatRelative } from "@/shared/lib/format";
import { useSession } from "@/shared/lib/session";
import { buttonVariants } from "@/shared/ui/button";
import { Skeleton } from "@/shared/ui/skeleton";
import { EmptyState, ErrorState, PageHeader, StatusBadge } from "@/shared/ui/patterns";
import { useDashboardSummary } from "../../application/useDashboardSummary";
import { budgetTone, jobTypeLabel, todoItems, type DashboardSummary } from "../../domain/summary";

/** / — 4 metrics in one row + "Cần bạn xử lý" / "Job gần đây" (SPEC-05 §4, FR-DSH-01/02). */
export default function DashboardPage() {
  const { can } = useSession();
  const summary = useDashboardSummary();
  const canGenerate = can("generate.run");

  return (
    <>
      <PageHeader
        title="Tổng quan"
        actions={
          canGenerate ? (
            <div className="flex items-center gap-2" role="group" aria-label="Lối tắt">
              <Link to="/content" className={buttonVariants({ variant: "secondary", size: "sm" })}>
                  <PenLine />
                  Tạo nội dung
                </Link>
              <Link to="/images" className={buttonVariants({ variant: "secondary", size: "sm" })}>
                  <Image />
                  Tạo ảnh
                </Link>
              <Link to="/videos" className={buttonVariants({ variant: "secondary", size: "sm" })}>
                  <Clapperboard />
                  Tạo video
                </Link>
            </div>
          ) : null
        }
      />
      {summary.error ? (
        <ErrorState error={summary.error} onRetry={() => void summary.refetch()} />
      ) : summary.isPending ? (
        <div aria-busy aria-label="Đang tải tổng quan" className="flex flex-col gap-6">
          <div className="grid grid-cols-4 gap-4">
            {Array.from({ length: 4 }, (_, i) => (
              <Skeleton key={i} className="h-20" />
            ))}
          </div>
          <div className="grid grid-cols-2 gap-6">
            <Skeleton className="h-48" />
            <Skeleton className="h-48" />
          </div>
        </div>
      ) : (
        <DashboardBody data={summary.data} />
      )}
    </>
  );
}

function DashboardBody({ data }: { data: DashboardSummary }) {
  const { campaigns, jobs, reviews, cost } = data;
  const tone = budgetTone(cost?.monthUsedPct ?? null);
  const todo = todoItems(data);

  return (
    <div className="flex flex-col gap-6">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4" role="group" aria-label="Số liệu chính">
        {campaigns ? <Metric label="Campaign đang chạy" value={formatNumber(campaigns.activeCount)} to="/campaigns" /> : null}
        <Metric
          label={jobs.scope === "all" ? "Job đang chạy (toàn hệ thống)" : "Job của bạn đang chạy"}
          value={formatNumber(jobs.running + jobs.queued)}
          sub={jobs.failed24h > 0 ? <span className="text-danger">{jobs.failed24h} lỗi trong 24 giờ</span> : "Không có lỗi trong 24 giờ"}
          to="/jobs"
        />
        <Metric
          label={reviews.pendingReview !== null ? "Chờ duyệt" : "Bạn đang chờ duyệt"}
          value={formatNumber(reviews.pendingReview ?? reviews.myPendingReview)}
          to={reviews.pendingReview !== null ? "/review" : "/library?status=pending_review"}
        />
        {cost ? (
          <Metric
            label="Chi phí tháng này"
            value={formatMoney({ amount: cost.month, currency: cost.currency })}
            sub={
              <>
                Hôm nay {formatMoney({ amount: cost.today, currency: cost.currency })}
                {cost.monthUsedPct !== null && cost.monthlyLimit !== null ? (
                  <span className={cn(tone === "danger" && "text-danger", tone === "warning" && "text-warning")}>
                    {" "}
                    · {formatNumber(Math.round(cost.monthUsedPct))}% hạn mức
                  </span>
                ) : null}
              </>
            }
            to="/usage"
          />
        ) : null}
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Panel title="Cần bạn xử lý">
          {todo.length === 0 ? (
            <EmptyState icon={ListChecks} message="Không có việc nào đang chờ bạn." />
          ) : (
            <ul className="divide-y divide-neutral-200">
              {todo.map((t) => (
                <li key={t.to + t.label}>
                  <Link to={t.to} className="flex items-center justify-between gap-3 px-4 py-2.5 text-body text-neutral-900 hover:bg-neutral-50">
                    <span>{t.label}</span>
                    <span className="tabular font-medium">{formatNumber(t.count)}</span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
          {campaigns && campaigns.recent.length > 0 ? (
            <div className="border-t border-neutral-200 px-4 py-3">
              <h3 className="mb-2 text-caption font-medium uppercase text-neutral-500">Campaign đang chạy</h3>
              <ul className="flex flex-col gap-1.5">
                {campaigns.recent.map((c) => (
                  <li key={c.id} className="flex items-center justify-between gap-3 text-body">
                    <Link to="/campaigns" className="truncate text-neutral-900 hover:underline">
                      {c.name}
                    </Link>
                    <span className="flex shrink-0 items-center gap-2 text-small text-neutral-500">
                      {formatRelative(c.updatedAt)}
                      <StatusBadge kind="campaign" status={c.status} />
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </Panel>

        <Panel title="Job gần đây">
          {jobs.recent.length === 0 ? (
            <EmptyState icon={Workflow} message="Không có job đang chạy hoặc lỗi gần đây." />
          ) : (
            <ul className="divide-y divide-neutral-200">
              {jobs.recent.map((j) => (
                <li key={j.id} className="flex items-center justify-between gap-3 px-4 py-2.5">
                  <span className="min-w-0">
                    <span className="block truncate text-body text-neutral-900">{jobTypeLabel(j.jobType)}</span>
                    <span className="block text-small text-neutral-500">
                      {formatRelative(j.createdAt)}
                      {j.status === "running" ? ` · ${j.progress}%` : ""}
                      {j.errorCode ? <span className="font-mono"> · {j.errorCode}</span> : null}
                    </span>
                  </span>
                  <StatusBadge kind="job" status={j.status} />
                </li>
              ))}
            </ul>
          )}
          <div className="border-t border-neutral-200 px-4 py-2">
            <Link to="/jobs" className="text-small text-accent-700 hover:underline">
              Xem tất cả job
            </Link>
          </div>
        </Panel>
      </div>
    </div>
  );
}

function Metric({ label, value, sub, to }: { label: string; value: string; sub?: ReactNode; to: string }) {
  return (
    <Link to={to} className="flex flex-col gap-1 rounded-lg border border-neutral-200 bg-neutral-0 p-4 hover:border-neutral-300">
      <span className="text-small text-neutral-500">{label}</span>
      <span className="text-page tabular text-neutral-900">{value}</span>
      {sub ? <span className="text-small text-neutral-500">{sub}</span> : null}
    </Link>
  );
}

function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section aria-label={title} className="flex flex-col rounded-lg border border-neutral-200 bg-neutral-0">
      <h2 className="border-b border-neutral-200 px-4 py-3 text-body font-medium text-neutral-900">{title}</h2>
      {children}
    </section>
  );
}
