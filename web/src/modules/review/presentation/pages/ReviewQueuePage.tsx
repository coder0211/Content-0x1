import { useEffect, useMemo } from "react";
import { useSearchParams } from "react-router";
import { CheckCheck } from "lucide-react";
import { formatRelative } from "@/shared/lib/format";
import { cn } from "@/shared/lib/cn";
import { controlClass } from "@/shared/ui/input";
import { ASSET_TYPE_LABELS, ASSET_TYPES, useReviewQueue, type AssetSummary, type AssetType, type ReviewQueueFilters } from "@/modules/library";
import { Button } from "@/shared/ui/button";
import { Skeleton } from "@/shared/ui/skeleton";
import { EmptyState, ErrorState, PageHeader } from "@/shared/ui/patterns";
import { ReviewPane } from "../components/ReviewPane";
import { nextSelection } from "../../domain/queue";

/** /review — queue on the left, review pane on the right; decide without leaving the page (ACS-081). */
export default function ReviewQueuePage() {
  const [params, setParams] = useSearchParams();
  const type = (params.get("type") as AssetType | null) || null;
  const filters: ReviewQueueFilters = useMemo(() => ({ type, campaignId: null }), [type]);
  const queue = useReviewQueue(filters);
  const items = useMemo(() => queue.data?.pages.flatMap((p) => p.items) ?? [], [queue.data]);
  const selected = params.get("asset");

  const select = (id: string | null) => {
    const next = new URLSearchParams(params);
    if (id) next.set("asset", id);
    else next.delete("asset");
    setParams(next, { replace: true });
  };

  // Select the oldest submission when nothing (or a vanished asset) is selected.
  useEffect(() => {
    if (queue.isPending || queue.error) return;
    if (!selected && items[0]) select(items[0].id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queue.isPending, queue.error, items, selected]);

  const onDecided = (id: string) => select(nextSelection(items.map((i) => i.id), id));

  return (
    <>
      <PageHeader
        title="Hàng chờ duyệt"
        description="Duyệt hoặc yêu cầu sửa ngay trên trang này. Asset gửi sớm nhất ở đầu danh sách."
        meta={!queue.isPending && !queue.error ? <span className="text-small text-neutral-500 tabular">{items.length}{queue.hasNextPage ? "+" : ""} asset</span> : null}
      />
      <div className="flex min-h-[560px] gap-6">
        <aside className="flex w-[360px] shrink-0 flex-col gap-3" aria-label="Danh sách chờ duyệt">
          <select
            aria-label="Loại asset"
            className={cn(controlClass, "h-8")}
            value={type ?? ""}
            onChange={(e) => {
              const next = new URLSearchParams();
              if (e.target.value) next.set("type", e.target.value);
              setParams(next, { replace: true });
            }}
          >
            <option value="">Tất cả loại</option>
            {ASSET_TYPES.map((t) => (
              <option key={t} value={t}>
                {ASSET_TYPE_LABELS[t]}
              </option>
            ))}
          </select>
          {queue.error ? (
            <ErrorState error={queue.error} onRetry={() => void queue.refetch()} />
          ) : queue.isPending ? (
            <div className="flex flex-col gap-2" aria-busy aria-label="Đang tải hàng chờ">
              {Array.from({ length: 5 }, (_, i) => (
                <Skeleton key={i} className="h-14" />
              ))}
            </div>
          ) : items.length === 0 ? (
            <EmptyState icon={CheckCheck} message={type ? "Không có asset loại này chờ duyệt." : "Không có asset nào chờ duyệt."} />
          ) : (
            <ul className="flex flex-col divide-y divide-neutral-200 rounded-lg border border-neutral-200 bg-neutral-0">
              {items.map((a) => (
                <QueueRow key={a.id} asset={a} active={a.id === selected} onSelect={() => select(a.id)} />
              ))}
            </ul>
          )}
          {queue.hasNextPage ? (
            <Button size="sm" variant="secondary" loading={queue.isFetchingNextPage} onClick={() => void queue.fetchNextPage()}>
              Tải thêm
            </Button>
          ) : null}
        </aside>
        <section className="min-w-0 flex-1" aria-label="Khung duyệt">
          {selected ? (
            <ReviewPane key={selected} assetId={selected} onDecided={onDecided} />
          ) : !queue.isPending && items.length === 0 ? null : (
            <p className="text-body text-neutral-500">Chọn một asset trong danh sách để duyệt.</p>
          )}
        </section>
      </div>
    </>
  );
}

function QueueRow({ asset, active, onSelect }: { asset: AssetSummary; active: boolean; onSelect: () => void }) {
  return (
    <li>
      <button
        type="button"
        aria-current={active || undefined}
        onClick={onSelect}
        className={cn(
          "flex w-full items-center gap-3 px-3 py-2 text-left transition-colors duration-150 hover:bg-neutral-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          active && "bg-accent-50 hover:bg-accent-50",
        )}
      >
        {asset.thumbnailUrl ? (
          <img src={asset.thumbnailUrl} alt="" className="size-10 shrink-0 rounded-sm border border-neutral-200 object-cover" />
        ) : (
          <span className="flex size-10 shrink-0 items-center justify-center rounded-sm bg-neutral-100 text-caption text-neutral-500">{ASSET_TYPE_LABELS[asset.type]}</span>
        )}
        <span className="min-w-0 flex-1">
          <span className="block truncate text-body font-medium text-neutral-900">{asset.title}</span>
          <span className="block truncate text-small text-neutral-500">
            {asset.createdByName} · v{asset.currentVersionNo}
            {asset.submittedAt ? ` · ${formatRelative(asset.submittedAt)}` : ""}
          </span>
        </span>
      </button>
    </li>
  );
}
