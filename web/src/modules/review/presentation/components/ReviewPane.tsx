import { useState } from "react";
import { Link } from "react-router";
import { ExternalLink } from "lucide-react";
import { formatDateTime, formatRelative } from "@/shared/lib/format";
import { ASSET_TYPE_LABELS, AssetVersionContent, useAsset, useAssetVersions, versionText, type Approval } from "@/modules/library";
import { Button, buttonVariants } from "@/shared/ui/button";
import { Skeleton } from "@/shared/ui/skeleton";
import { ErrorState, StatusBadge } from "@/shared/ui/patterns";
import { DecisionForm } from "./DecisionForm";
import { TextDiff } from "./TextDiff";

const DECISION_LABEL: Record<Approval["decision"], string> = {
  submitted: "Gửi duyệt",
  approved: "Đã duyệt",
  changes_requested: "Yêu cầu sửa",
};

/** Right pane of the review queue: content/image/video, diff vs previous version, history, decision. */
export function ReviewPane({ assetId, onDecided }: { assetId: string; onDecided: (id: string) => void }) {
  const asset = useAsset(assetId);
  const versions = useAssetVersions(assetId);
  const [mode, setMode] = useState<"content" | "diff">("content");

  if (asset.error) return <ErrorState error={asset.error} onRetry={() => void asset.refetch()} />;
  if (asset.isPending) {
    return (
      <div className="flex flex-col gap-3" aria-busy aria-label="Đang tải asset">
        <Skeleton className="h-6 w-2/3" />
        <Skeleton className="h-4 w-1/3" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  const a = asset.data;
  const cur = a.currentVersion;
  const previous = versions.data?.find((v) => v.versionNo < cur.versionNo) ?? null;
  const canDiff = a.type === "content" || a.type === "video";
  const showDiff = canDiff && mode === "diff" && previous !== null;

  return (
    <article className="flex flex-col gap-5" aria-label={`Duyệt: ${a.title}`}>
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-section text-neutral-900">{a.title}</h2>
            <StatusBadge kind="asset" status={a.status} />
          </div>
          <p className="mt-1 text-small text-neutral-500">
            {ASSET_TYPE_LABELS[a.type]}
            {a.subtype ? ` · ${a.subtype}` : ""} · v{cur.versionNo} · {a.createdByName}
            {a.submittedAt ? ` · gửi ${formatRelative(a.submittedAt)}` : ""}
          </p>
        </div>
        <Link to={`/library/${a.id}`} className={buttonVariants({ variant: "ghost", size: "sm" })}>
            <ExternalLink />
            Mở chi tiết
          </Link>
      </header>

      {canDiff ? (
        <div className="flex items-center gap-1" role="group" aria-label="Chế độ xem">
          <Button size="sm" variant={mode === "content" ? "secondary" : "ghost"} aria-pressed={mode === "content"} onClick={() => setMode("content")}>
            Nội dung
          </Button>
          <Button
            size="sm"
            variant={mode === "diff" ? "secondary" : "ghost"}
            aria-pressed={mode === "diff"}
            disabled={previous === null}
            title={previous === null ? "Đây là phiên bản đầu tiên" : undefined}
            onClick={() => setMode("diff")}
          >
            {previous ? `So với v${previous.versionNo}` : "Không có bản trước"}
          </Button>
        </div>
      ) : null}

      <section className="rounded-lg border border-neutral-200 bg-neutral-0 p-4">
        {showDiff && previous ? (
          <TextDiff before={versionText(a.type, previous.content)} after={versionText(a.type, cur.content)} />
        ) : (
          <AssetVersionContent type={a.type} title={a.title} version={cur} />
        )}
        {cur.changeNote ? <p className="mt-3 text-small text-neutral-500">Ghi chú phiên bản: {cur.changeNote}</p> : null}
      </section>

      {a.subtype === "poster" ? (
        <p className="text-small text-neutral-700">
          Người tạo đã xác nhận kiểm tra chữ trên poster: {a.approvals.some((x) => x.decision === "submitted" && x.checklist.typography_checked === true) ? "có" : "chưa"}.
        </p>
      ) : null}

      <DecisionForm key={a.id} asset={a} onDecided={onDecided} />

      <section aria-label="Lịch sử duyệt" className="flex flex-col gap-2">
        <h3 className="text-caption font-medium uppercase text-neutral-500">Lịch sử duyệt</h3>
        {a.approvals.length === 0 ? (
          <p className="text-small text-neutral-500">Chưa có nhận xét.</p>
        ) : (
          <ol className="flex flex-col gap-2">
            {a.approvals.map((x) => (
              <li key={x.id} className="text-small">
                <p className="text-neutral-900">
                  <span className="font-medium">{x.reviewerName}</span> · {DECISION_LABEL[x.decision]} v{x.versionNo}
                  <span className="text-neutral-500"> · {formatDateTime(x.createdAt)}</span>
                </p>
                {x.comment ? <p className="whitespace-pre-line text-neutral-700">{x.comment}</p> : null}
              </li>
            ))}
          </ol>
        )}
      </section>
    </article>
  );
}
