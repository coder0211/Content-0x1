import { useRef, useState } from "react";
import { isApiError } from "@/shared/kernel";
import { describeError } from "@/shared/lib/errors";
import { formatNumber, formatUsd } from "@/shared/lib/format";
import { newIdempotencyKey } from "@/shared/lib/id";
import { useSession } from "@/shared/lib/session";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Skeleton } from "@/shared/ui/skeleton";
import { toast } from "@/shared/ui/toast";
import { CostChip, ErrorState } from "@/shared/ui/patterns";
import { useProduce, useVideoEstimate } from "../../application/useVideo";
import { produceBlockers, type StoryboardIssue, type VideoEstimate } from "../../domain/video";

const ITEM_LABELS: Record<string, string> = {
  scene_clips: "Sinh clip cảnh",
  voiceover: "Giọng đọc",
  subtitles_script: "Phụ đề",
};

function unitLabel(unit: string): string {
  if (unit.startsWith("video_second")) return "giây";
  if (unit === "char") return "ký tự";
  return unit;
}

export interface EstimatePanelProps {
  assetId: string;
  issues: StoryboardIssue[];
  dirty: boolean;
  jobActive: boolean;
  hasScenes: boolean;
}

/** Panel dự toán + nút "Sản xuất video" (ACS-064, ACS-071). */
export function EstimatePanel({ assetId, issues, dirty, jobActive, hasScenes }: EstimatePanelProps) {
  const { user } = useSession();
  const canOverride = !!user && (user.roles.includes("admin") || user.roles.includes("marketing_lead"));
  const [override, setOverride] = useState(false);
  const estimate = useVideoEstimate(assetId, hasScenes && !dirty && issues.length === 0 && !jobActive);
  const produce = useProduce(assetId);
  const keyRef = useRef<string | null>(null);

  const blockers = produceBlockers({ issues, dirty, estimate: estimate.data, canOverride, override, jobActive });
  if (!hasScenes) blockers.unshift("Storyboard chưa có cảnh nào.");

  function onProduce() {
    keyRef.current ??= newIdempotencyKey();
    produce.mutate(
      { budgetOverride: canOverride && override, key: keyRef.current },
      {
        onSuccess: () => {
          keyRef.current = null;
          toast.success("Đã bắt đầu sản xuất video");
        },
        onError: (err) => {
          if (isApiError(err) && err.status < 500) keyRef.current = null;
        },
      },
    );
  }

  return (
    <section aria-label="Dự toán" className="flex flex-col gap-3">
      <h2 className="text-section font-semibold text-neutral-900">Dự toán</h2>
      {issues.length > 0 || dirty ? (
        <p className="text-small text-neutral-600">{dirty ? "Lưu storyboard để xem dự toán." : "Sửa storyboard để xem dự toán."}</p>
      ) : estimate.isLoading ? (
        <div className="flex flex-col gap-2" aria-label="Đang tải dự toán">
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-2/3" />
          <Skeleton className="h-6 w-24" />
        </div>
      ) : estimate.error ? (
        <ErrorState error={estimate.error} onRetry={() => void estimate.refetch()} title="Không tải được dự toán" />
      ) : estimate.data ? (
        <EstimateBreakdown e={estimate.data} />
      ) : null}

      {estimate.data?.requiresOverride ? (
        <Banner tone="warning">
          Dự toán {formatUsd(estimate.data.total, { estimate: true })} vượt hạn mức mỗi job {formatUsd(estimate.data.perJobLimit)}. Giảm số cảnh
          hoặc thời lượng{canOverride ? ", hoặc chọn vượt hạn mức." : ", hoặc xin Marketing Lead duyệt."}
        </Banner>
      ) : null}
      {estimate.data?.requiresOverride && canOverride ? (
        <label className="flex items-center gap-2 text-small text-neutral-900">
          <Checkbox checked={override} onCheckedChange={(v) => setOverride(v === true)} aria-label="Vượt hạn mức (Lead duyệt)" />
          Vượt hạn mức (Lead duyệt, có ghi audit)
        </label>
      ) : null}

      {produce.error ? <ProduceError error={produce.error} /> : null}

      <Button variant="primary" onClick={onProduce} loading={produce.isPending} disabled={blockers.length > 0}>
        Sản xuất video
      </Button>
      {blockers.length > 0 ? (
        <ul aria-label="Lý do chưa sản xuất được" className="flex flex-col gap-1 text-small text-neutral-600">
          {blockers.map((b) => (
            <li key={b}>{b}</li>
          ))}
        </ul>
      ) : null}
    </section>
  );
}

function ProduceError({ error }: { error: unknown }) {
  if (isApiError(error) && error.code === "JOB_BUDGET_EXCEEDED") {
    return <Banner tone="warning">Dự toán vượt hạn mức mỗi job. Giảm số cảnh hoặc thời lượng, hoặc xin Marketing Lead duyệt.</Banner>;
  }
  return <Banner tone="danger">{describeError(error).message}</Banner>;
}

function EstimateBreakdown({ e }: { e: VideoEstimate }) {
  return (
    <div className="flex flex-col gap-2">
      <table className="w-full text-small">
        <tbody>
          {e.breakdown.map((b) => (
            <tr key={b.item + b.unit} className="border-b border-neutral-200 last:border-b-0">
              <td className="py-1.5 text-neutral-700">
                {ITEM_LABELS[b.item] ?? b.item}
                <span className="block text-caption text-neutral-500 tabular">
                  {formatNumber(b.units)} {unitLabel(b.unit)}
                </span>
              </td>
              <td className="py-1.5 text-right text-neutral-900 tabular">{formatUsd(b.amount, { estimate: true })}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="flex items-center justify-between">
        <span className="text-small font-medium text-neutral-900">Tổng dự toán</span>
        <CostChip amount={e.total} estimate />
      </div>
      <dl className="grid grid-cols-2 gap-x-2 gap-y-1 text-caption text-neutral-600">
        <dt>Hạn mức mỗi job</dt>
        <dd className="text-right tabular">{formatUsd(e.perJobLimit)}</dd>
        {e.campaignRemaining !== null ? (
          <>
            <dt>Ngân sách campaign còn</dt>
            <dd className="text-right tabular">{formatUsd(e.campaignRemaining)}</dd>
          </>
        ) : null}
        <dt>Ngân sách tháng còn</dt>
        <dd className="text-right tabular">{formatUsd(e.globalRemaining)}</dd>
      </dl>
    </div>
  );
}
