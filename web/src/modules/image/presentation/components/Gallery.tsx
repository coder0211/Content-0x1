import { Image as ImageIcon } from "lucide-react";
import type { ID } from "@/shared/kernel";
import { cn } from "@/shared/lib/cn";
import { isRetryableErrorKind } from "@/shared/lib/errors";
import { formatRelative } from "@/shared/lib/format";
import { Button } from "@/shared/ui/button";
import { Skeleton } from "@/shared/ui/skeleton";
import { AIProvenance, CostChip, EmptyState, JobProgress, StatusBadge } from "@/shared/ui/patterns";
import {
  PRESET_LABELS,
  cssAspect,
  isBatchActive,
  operationLabel,
  progressLabel,
  type ImageBatch,
  type ImageFile,
} from "../../domain/image";

export interface Selection {
  batchId: ID;
  fileId: ID;
}

function MediaCard({
  batch,
  image,
  index,
  selected,
  onSelect,
  onOpen,
}: {
  batch: ImageBatch;
  image: ImageFile;
  index: number;
  selected: boolean;
  onSelect: () => void;
  onOpen: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      aria-label={`Ảnh ${index + 1} — ${batch.asset?.title ?? batch.prompt}`}
      onClick={onSelect}
      onDoubleClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === " " && selected) {
          e.preventDefault();
          onOpen();
        }
      }}
      className={cn(
        "flex items-center justify-center overflow-hidden rounded bg-neutral-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        selected ? "outline outline-2 outline-accent-600" : "outline outline-1 outline-neutral-200 hover:outline-neutral-300",
      )}
      style={{ aspectRatio: cssAspect(batch.aspectRatio) }}
    >
      <img src={image.thumbnailUrl} alt={batch.asset?.title ?? ""} loading="lazy" className="size-full object-contain" />
    </button>
  );
}

function BatchCard({
  batch,
  selection,
  onSelect,
  onOpen,
  onRetry,
  retrying,
}: {
  batch: ImageBatch;
  selection: Selection | null;
  onSelect: (s: Selection) => void;
  onOpen: (batch: ImageBatch, index: number) => void;
  onRetry: (batch: ImageBatch) => void;
  retrying: boolean;
}) {
  const cost = batch.job.costActual ?? batch.job.costEstimate;
  const images = batch.asset?.images ?? [];
  return (
    <section aria-label={`Đợt tạo ${formatRelative(batch.createdAt)}`} className="rounded-lg border border-neutral-200 bg-neutral-0 p-4">
      <header className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1">
        <h3 className="text-body font-medium text-neutral-900">{operationLabel(batch.operation)}</h3>
        <span className="text-small text-neutral-500">
          {PRESET_LABELS[batch.preset] ?? batch.preset} · {batch.aspectRatio} · {batch.count} ảnh · {formatRelative(batch.createdAt)}
        </span>
        <span className="ml-auto flex items-center gap-2">
          {batch.asset ? <StatusBadge kind="asset" status={batch.asset.status} /> : null}
          {cost ? <CostChip amount={cost.amount} estimate={cost.isEstimate} /> : null}
        </span>
      </header>
      <p className="mb-3 line-clamp-2 text-small text-neutral-700" title={batch.prompt}>
        {batch.prompt}
      </p>

      {isBatchActive(batch) ? (
        <JobProgress status={batch.job.status} progress={batch.job.progress} label={progressLabel(batch)} />
      ) : null}
      {batch.job.status === "failed" || batch.job.status === "cancelled" ? (
        <JobProgress
          status={batch.job.status}
          errorKind={batch.job.errorKind ?? batch.job.errorCode}
          errorDetail={batch.job.errorMessage ?? undefined}
          onRetry={batch.job.status === "failed" && (!batch.job.errorKind || isRetryableErrorKind(batch.job.errorKind)) ? () => onRetry(batch) : undefined}
          retrying={retrying}
        />
      ) : null}

      {images.length > 0 ? (
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
          {images.map((img, i) => (
            <MediaCard
              key={img.fileId}
              batch={batch}
              image={img}
              index={i}
              selected={selection?.batchId === batch.id && selection.fileId === img.fileId}
              onSelect={() => onSelect({ batchId: batch.id, fileId: img.fileId })}
              onOpen={() => onOpen(batch, i)}
            />
          ))}
        </div>
      ) : null}

      {batch.asset ? (
        <footer className="mt-3 flex flex-wrap items-center gap-3">
          <AIProvenance model={batch.modelCode} createdAt={batch.createdAt} />
          <span className="text-caption text-neutral-500">Phiên bản {batch.asset.versionNo}</span>
          {batch.asset.requiresTypographyCheck && (batch.asset.status === "draft" || batch.asset.status === "changes_requested") ? (
            <span className="text-caption text-warning">Cần kiểm tra chữ trước khi gửi duyệt</span>
          ) : null}
        </footer>
      ) : null}
    </section>
  );
}

export function GallerySkeleton() {
  return (
    <div className="flex flex-col gap-4" aria-busy="true" aria-label="Đang tải ảnh">
      {Array.from({ length: 2 }, (_, i) => (
        <div key={i} className="rounded-lg border border-neutral-200 bg-neutral-0 p-4">
          <Skeleton className="mb-3 h-5 w-64" />
          <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
            {Array.from({ length: 4 }, (_, j) => (
              <Skeleton key={j} className="aspect-square" />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

/** Centre canvas: batches newest first (SPEC-05 §4.3 Image Studio). */
export function Gallery({
  batches,
  selection,
  onSelect,
  onOpen,
  onRetry,
  retryingId,
  hasMore,
  loadingMore,
  onLoadMore,
  onStart,
}: {
  batches: ImageBatch[];
  selection: Selection | null;
  onSelect: (s: Selection) => void;
  onOpen: (batch: ImageBatch, index: number) => void;
  onRetry: (batch: ImageBatch) => void;
  retryingId: ID | null;
  hasMore: boolean;
  loadingMore: boolean;
  onLoadMore: () => void;
  onStart: () => void;
}) {
  if (batches.length === 0) {
    return (
      <EmptyState
        icon={ImageIcon}
        message="Chưa có ảnh nào ở đây."
        action={
          <Button variant="secondary" onClick={onStart}>
            Viết mô tả ảnh
          </Button>
        }
      />
    );
  }
  return (
    <div className="flex flex-col gap-4">
      {batches.map((b) => (
        <BatchCard
          key={b.id}
          batch={b}
          selection={selection}
          onSelect={onSelect}
          onOpen={onOpen}
          onRetry={onRetry}
          retrying={retryingId === b.id}
        />
      ))}
      {hasMore ? (
        <Button variant="secondary" className="self-center" loading={loadingMore} onClick={onLoadMore}>
          Tải thêm đợt tạo
        </Button>
      ) : null}
    </div>
  );
}
