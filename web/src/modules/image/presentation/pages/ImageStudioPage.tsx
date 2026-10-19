import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useSearchParams } from "react-router";
import { Megaphone } from "lucide-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Label } from "@/shared/ui/label";
import { toast } from "@/shared/ui/toast";
import { EmptyState, ErrorState, PageHeader } from "@/shared/ui/patterns";
import { flattenBatches, useImageBatches, useRetryImageJob, useStudioTargets } from "../../application/useImageStudio";
import { isBatchActive, type ImageBatch, type StudioTarget } from "../../domain/image";
import { Gallery, GallerySkeleton, type Selection } from "../components/Gallery";
import { GenerationForm, PROMPT_FIELD_ID } from "../components/GenerationForm";
import { Inspector } from "../components/Inspector";
import { Lightbox } from "../components/Lightbox";

const NO_CAMPAIGN = "__none__";

function TargetPicker({
  campaignId,
  brandId,
  showBrand,
  onCampaign,
  onBrand,
}: {
  campaignId: string | null;
  brandId: string | null;
  showBrand: boolean;
  onCampaign: (id: string | null) => void;
  onBrand: (id: string) => void;
}) {
  const { campaigns, brands } = useStudioTargets();
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="image-campaign">Campaign</Label>
        <Select
          value={campaignId ?? (showBrand ? NO_CAMPAIGN : undefined)}
          onValueChange={(v) => onCampaign(v === NO_CAMPAIGN ? null : v)}
          disabled={campaigns.isPending}
        >
          <SelectTrigger id="image-campaign" aria-label="Campaign">
            <SelectValue placeholder={campaigns.isPending ? "Đang tải…" : "Chọn campaign"} />
          </SelectTrigger>
          <SelectContent>
            {(campaigns.data ?? []).map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.name}
              </SelectItem>
            ))}
            <SelectItem value={NO_CAMPAIGN}>Không thuộc campaign</SelectItem>
          </SelectContent>
        </Select>
        {campaigns.isError ? <p className="text-small text-danger">Không tải được danh sách campaign.</p> : null}
      </div>
      {showBrand ? (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="image-brand">Brand</Label>
          <Select value={brandId ?? undefined} onValueChange={onBrand}>
            <SelectTrigger id="image-brand" aria-label="Brand">
              <SelectValue placeholder="Chọn brand" />
            </SelectTrigger>
            <SelectContent>
              {(brands.data ?? []).map((b) => (
                <SelectItem key={b.id} value={b.id}>
                  {b.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : null}
    </div>
  );
}

/** Image Studio (ACS-053): brief 320px | gallery canvas | inspector 300px. Accepts `?campaign_id=` and `?brand_id=`. */
export default function ImageStudioPage() {
  const [params, setParams] = useSearchParams();
  const campaignId = params.get("campaign_id");
  const brandId = params.get("brand_id");
  const target: StudioTarget | null = campaignId ? { campaignId } : brandId ? { brandId } : null;
  const [noCampaign, setNoCampaign] = useState(false);

  const batchesQ = useImageBatches(target ? (campaignId ?? undefined) : undefined);
  const batches = useMemo(() => {
    const all = flattenBatches(batchesQ.data?.pages);
    // Independent tasks: the API lists the user's own batches; keep those of the chosen brand.
    return campaignId ? all : all.filter((b) => b.campaignId == null && (!brandId || b.brandId === brandId));
  }, [batchesQ.data, campaignId, brandId]);
  const retry = useRetryImageJob();

  const [selection, setSelection] = useState<Selection | null>(null);
  const [lightbox, setLightbox] = useState<{ batchId: string; index: number } | null>(null);
  const selectedBatch = batches.find((b) => b.id === selection?.batchId) ?? null;
  const lightboxBatch = batches.find((b) => b.id === lightbox?.batchId) ?? null;

  // Toast when a job the user is watching finishes (SPEC-05 §5: job running → toast on completion).
  const prev = useRef(new Map<string, string>());
  useEffect(() => {
    for (const b of batches) {
      const before = prev.current.get(b.id);
      if ((before === "queued" || before === "running") && !isBatchActive(b)) {
        if (b.job.status === "succeeded") {
          const n = b.asset?.images.length ?? b.count;
          toast.success(`Đã tạo xong ${n} ảnh`, {
            action: b.asset?.images[0]
              ? { label: "Xem", onClick: () => setSelection({ batchId: b.id, fileId: b.asset!.images[0]!.fileId }) }
              : undefined,
          });
        } else if (b.job.status === "failed") {
          toast.warning("Tạo ảnh không thành công", { description: "Xem nguyên nhân trong lưới ảnh." });
        }
      }
      prev.current.set(b.id, b.job.status);
    }
  }, [batches]);

  const setCampaign = (id: string | null) => {
    setSelection(null);
    setNoCampaign(id == null);
    setParams(id ? { campaign_id: id } : {}, { replace: true });
  };
  const setBrand = (id: string) => {
    setSelection(null);
    setParams({ brand_id: id }, { replace: true });
  };

  const openSelected = () => {
    if (!selectedBatch?.asset || !selection) return;
    setLightbox({ batchId: selectedBatch.id, index: selectedBatch.asset.images.findIndex((i) => i.fileId === selection.fileId) });
  };

  let canvas: ReactNode;
  if (!target) {
    canvas = <EmptyState icon={Megaphone} message="Chọn campaign hoặc brand ở cột bên trái để bắt đầu tạo ảnh." />;
  } else if (batchesQ.isPending) {
    canvas = <GallerySkeleton />;
  } else if (batchesQ.isError) {
    canvas = <ErrorState error={batchesQ.error} onRetry={() => void batchesQ.refetch()} />;
  } else {
    canvas = (
      <Gallery
        batches={batches}
        selection={selection}
        onSelect={setSelection}
        onOpen={(b: ImageBatch, index) => {
          setSelection({ batchId: b.id, fileId: b.asset!.images[index]!.fileId });
          setLightbox({ batchId: b.id, index });
        }}
        onRetry={(b) => retry.mutate(b.job.id)}
        retryingId={retry.isPending ? (batches.find((b) => b.job.id === retry.variables)?.id ?? null) : null}
        hasMore={batchesQ.hasNextPage}
        loadingMore={batchesQ.isFetchingNextPage}
        onLoadMore={() => void batchesQ.fetchNextPage()}
        onStart={() => document.getElementById(PROMPT_FIELD_ID)?.focus()}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <PageHeader title="Image Studio" description="Tạo ảnh từ mô tả và ảnh tham chiếu, tạo biến thể, chỉnh sửa và gửi duyệt." />
      <div className="grid min-h-[calc(100vh-180px)] grid-cols-[320px_minmax(0,1fr)_300px] overflow-hidden rounded-lg border border-neutral-200">
        <aside aria-label="Tùy chọn tạo ảnh" className="flex flex-col gap-4 overflow-y-auto border-r border-neutral-200 bg-neutral-0 p-4">
          <TargetPicker
            campaignId={campaignId}
            brandId={brandId}
            showBrand={!campaignId && (noCampaign || !!brandId)}
            onCampaign={setCampaign}
            onBrand={setBrand}
          />
          <GenerationForm key={campaignId ?? brandId ?? "none"} target={target} />
        </aside>
        <main aria-label="Kết quả" className="overflow-y-auto bg-neutral-100 p-4">
          {canvas}
        </main>
        <aside aria-label="Chi tiết ảnh" className="overflow-y-auto border-l border-neutral-200 bg-neutral-0 p-4">
          <Inspector batch={selectedBatch} fileId={selection?.fileId ?? null} onOpen={openSelected} />
        </aside>
      </div>
      <Lightbox
        batch={lightboxBatch}
        index={lightbox?.index ?? 0}
        onIndexChange={(index) => setLightbox((l) => (l ? { ...l, index } : l))}
        onClose={() => setLightbox(null)}
      />
    </div>
  );
}
