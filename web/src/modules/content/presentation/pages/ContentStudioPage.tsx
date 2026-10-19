import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { PenLine } from "lucide-react";
import type { ID, Money } from "@/shared/kernel";
import { newIdempotencyKey } from "@/shared/lib/id";
import { EmptyState, ErrorState, JobProgress } from "@/shared/ui/patterns";
import { Skeleton } from "@/shared/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { toast } from "@/shared/ui/toast";
import { contentKeys } from "../../application/keys";
import { useGenerate } from "../../application/mutations";
import { useCampaign } from "../../application/queries";
import { useContentJob } from "../../application/useContentJob";
import { variantLabel, type GenerateInput } from "../../domain/content";
import { AssetWorkspace, type StartedJob } from "../components/AssetWorkspace";
import { BriefPanel } from "../components/BriefPanel";

interface ActiveJob {
  id: ID;
  kind: "generate" | StartedJob["kind"];
  assetId: ID | null;
  label: string;
}

const splitIds = (s: string | null): ID[] =>
  s ? s.split(",").filter(Boolean) : [];

/**
 * Content Studio (ACS-035): brief | variant tabs + editor | inspector.
 * Query params: `campaign_id` (brief from campaign), `assets` (open variants, comma separated),
 * `asset_id` (open one existing asset, e.g. from the library).
 */
export default function ContentStudioPage() {
  const [params, setParams] = useSearchParams();
  const qc = useQueryClient();
  const campaignId = params.get("campaign_id");
  const campaign = useCampaign(campaignId);
  const assetIds = splitIds(params.get("assets") ?? params.get("asset_id"));
  const [active, setActive] = useState<ID | null>(assetIds[0] ?? null);
  const [job, setJob] = useState<ActiveJob | null>(null);
  const [estimate, setEstimate] = useState<Money | null>(null);
  const lastGenerate = useRef<GenerateInput | null>(null);
  const handled = useRef<string | null>(null);

  const generate = useGenerate();
  const jobState = useContentJob(job?.id ?? null);
  const snap = jobState.data;

  const setAssets = (ids: ID[], select: ID | null) => {
    setParams(
      (p) => {
        const n = new URLSearchParams(p);
        n.delete("asset_id");
        if (ids.length) n.set("assets", ids.join(","));
        else n.delete("assets");
        return n;
      },
      { replace: true },
    );
    setActive(select ?? ids[0] ?? null);
  };

  useEffect(() => {
    if (!job || !snap || snap.id !== job.id || handled.current === snap.id)
      return;
    if (snap.status === "succeeded") {
      handled.current = snap.id;
      if (job.kind === "generate") {
        setAssets(snap.assetIds, snap.assetIds[0] ?? null);
        toast.success(`Đã viết ${snap.assetIds.length} phương án`);
      } else if (job.kind === "rewrite" && job.assetId) {
        void qc.invalidateQueries({ queryKey: contentKeys.asset(job.assetId) });
        toast.success("Đã tạo phiên bản mới");
      } else if (job.kind === "transform") {
        const created = snap.assetIds[0];
        if (created) {
          const ids = assetIds.includes(created)
            ? assetIds
            : [...assetIds, created];
          toast.success("Đã tạo asset mới từ bài gốc", {
            action: { label: "Xem", onClick: () => setAssets(ids, created) },
          });
          setAssets(ids, active);
        }
      }
      setJob(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [snap, job]);

  const startGenerate = (input: GenerateInput, key = newIdempotencyKey()) => {
    lastGenerate.current = input;
    handled.current = null;
    generate.mutate(
      { input, key },
      {
        onSuccess: (acc) => {
          setEstimate(acc.estimatedCost);
          setJob({
            id: acc.jobId,
            kind: "generate",
            assetId: null,
            label: "Đang viết các phương án",
          });
        },
      },
    );
  };

  const onJobStarted = (j: StartedJob) => {
    handled.current = null;
    setJob({ id: j.id, kind: j.kind, assetId: j.assetId, label: j.label });
  };

  if (campaignId && campaign.isLoading) {
    return <Skeleton className="h-96 w-full" />;
  }
  if (campaignId && campaign.error) {
    return (
      <ErrorState
        error={campaign.error}
        onRetry={() => void campaign.refetch()}
      />
    );
  }

  const failed =
    snap &&
    job &&
    snap.id === job.id &&
    (snap.status === "failed" || snap.status === "cancelled");
  const running = !!job && !failed;
  const selected =
    active && assetIds.includes(active) ? active : (assetIds[0] ?? null);

  return (
    <div className="-m-6 flex h-[calc(100vh-48px)] min-h-0">
      <aside
        aria-label="Brief & tùy chọn"
        className="w-[320px] shrink-0 border-r border-neutral-200 bg-neutral-0"
      >
        <BriefPanel
          campaign={campaign.data ?? null}
          pending={generate.isPending || (running && job?.kind === "generate")}
          error={generate.error}
          estimate={estimate}
          onSubmit={(input) => startGenerate(input)}
        />
      </aside>
      <div className="flex min-w-0 flex-1 flex-col bg-neutral-100">
        <div className="border-b border-neutral-200 bg-neutral-0 px-6 pt-4">
          <h1 className="text-page font-semibold text-neutral-900">
            Content Studio
          </h1>
          {job ? (
            <JobProgress
              className="mt-3 pb-3"
              status={snap?.id === job.id ? snap.status : "queued"}
              progress={snap?.id === job.id ? snap.progress : 0}
              label={job.label}
              errorKind={failed ? (snap?.errorKind ?? "Transient") : null}
              errorDetail={snap?.errorMessage ?? undefined}
              cancelling={jobState.isCancelling}
              onCancel={running ? () => void jobState.cancel() : undefined}
              onRetry={
                failed && job.kind === "generate" && lastGenerate.current
                  ? () => {
                      setJob(null);
                      startGenerate(lastGenerate.current!);
                    }
                  : undefined
              }
            />
          ) : null}
          {assetIds.length ? (
            <Tabs value={selected ?? undefined} onValueChange={setActive}>
              <TabsList aria-label="Phương án" className="border-b-0">
                {assetIds.map((id, i) => (
                  <TabsTrigger key={id} value={id}>
                    Phương án {variantLabel(i)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          ) : (
            <div className="h-4" />
          )}
        </div>
        {selected ? (
          <AssetWorkspace
            key={selected}
            assetId={selected}
            busy={running && job?.assetId === selected}
            onJobStarted={onJobStarted}
          />
        ) : (
          <div className="flex flex-1 items-center justify-center p-6">
            {running ? (
              <p className="text-body text-neutral-500">
                Các phương án sẽ hiện ở đây khi job hoàn tất. Bạn có thể rời
                trang, job vẫn chạy.
              </p>
            ) : (
              <EmptyState
                icon={PenLine}
                message="Chưa có phương án nào. Điền brief bên trái rồi bấm viết phương án."
              />
            )}
          </div>
        )}
      </div>
    </div>
  );
}
