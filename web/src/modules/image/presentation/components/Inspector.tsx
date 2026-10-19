import { useEffect, useState, type ReactNode } from "react";
import { Maximize2 } from "lucide-react";
import { isApiError } from "@/shared/kernel";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Label } from "@/shared/ui/label";
import { Separator } from "@/shared/ui/separator";
import { Textarea } from "@/shared/ui/textarea";
import { toast } from "@/shared/ui/toast";
import { CostChip, FormField, StatusBadge } from "@/shared/ui/patterns";
import { useEditImage, useImageEstimate, useSubmitImageReview, useVaryImage } from "../../application/useImageStudio";
import { MAX_COUNT, PRESET_LABELS, canSubmitForReview, type ImageBatch, type Quality } from "../../domain/image";
import { Field, Segmented } from "./Segmented";

const COUNTS = Array.from({ length: MAX_COUNT }, (_, i) => ({ value: i + 1, label: String(i + 1) }));

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <h3 className="text-small font-medium text-neutral-900">{title}</h3>
      {children}
    </section>
  );
}

/** Right column: details of the selected image + variation / edit-by-prompt / submit for review. */
export function Inspector({ batch, fileId, onOpen }: { batch: ImageBatch | null; fileId: string | null; onOpen: () => void }) {
  const [count, setCount] = useState(2);
  const [editPrompt, setEditPrompt] = useState("");
  const [typographyChecked, setTypographyChecked] = useState(false);
  const [reviewError, setReviewError] = useState<string | null>(null);
  const vary = useVaryImage();
  const edit = useEditImage();
  const submit = useSubmitImageReview();

  useEffect(() => {
    setEditPrompt("");
    setTypographyChecked(false);
    setReviewError(null);
  }, [fileId]);

  const asset = batch?.asset ?? null;
  const image = asset?.images.find((i) => i.fileId === fileId) ?? null;
  const varyEstimate = useImageEstimate(
    { campaignId: batch?.campaignId ?? undefined, operation: "variation", aspectRatio: batch?.aspectRatio ?? "1:1", quality: (batch?.quality ?? "standard") as Quality, count },
    !!image,
  );
  const editEstimate = useImageEstimate(
    { campaignId: batch?.campaignId ?? undefined, operation: "edit", aspectRatio: batch?.aspectRatio ?? "1:1", quality: (batch?.quality ?? "standard") as Quality, count: 1 },
    !!image,
  );

  if (!batch || !asset || !image) {
    return <p className="text-small text-neutral-500">Chọn một ảnh trong lưới để xem chi tiết, tạo biến thể hoặc chỉnh sửa.</p>;
  }

  const isCurrent = asset.versionNo === asset.currentVersionNo;
  const reviewable = isCurrent && canSubmitForReview(asset.status);
  const onAccepted = (what: string) => () => toast.success(`Đã gửi yêu cầu ${what}`, { description: "Kết quả sẽ hiện ở đầu lưới ảnh." });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <button
          type="button"
          onClick={onOpen}
          aria-label="Xem ảnh cỡ lớn"
          className="flex max-h-56 items-center justify-center overflow-hidden rounded border border-neutral-200 bg-neutral-100"
        >
          <img src={image.thumbnailUrl} alt="" className="max-h-56 object-contain" />
        </button>
        <p className="text-body font-medium text-neutral-900">{asset.title}</p>
        <div className="flex flex-wrap items-center gap-2 text-small text-neutral-500">
          <StatusBadge kind="asset" status={asset.status} />
          <span>Phiên bản {asset.versionNo}</span>
          <span className="tabular">
            {image.width}×{image.height}px
          </span>
        </div>
        <p className="text-small text-neutral-500">
          {PRESET_LABELS[batch.preset] ?? batch.preset} · {batch.aspectRatio} · {batch.quality === "high" ? "Chất lượng cao" : "Chất lượng tiêu chuẩn"}
        </p>
        {image.parentFileId ? <p className="text-small text-neutral-500">Tạo từ một ảnh gốc của asset này.</p> : null}
        <Button variant="ghost" size="sm" className="self-start" onClick={onOpen}>
          <Maximize2 aria-hidden />
          Xem cỡ lớn
        </Button>
      </div>

      {!isCurrent ? (
        <p className="text-small text-neutral-500">
          Đây là phiên bản cũ (v{asset.versionNo}). Chọn ảnh ở phiên bản mới nhất (v{asset.currentVersionNo}) để tạo biến thể hoặc chỉnh sửa.
        </p>
      ) : (
        <>
          <Separator />
          <Section title="Tạo biến thể">
            <Field id="vary-count" label="Số biến thể">
              <Segmented label="Số biến thể" value={count} options={COUNTS} onChange={setCount} />
            </Field>
            <div className="flex items-center gap-2">
              <Button
                variant="secondary"
                loading={vary.isPending}
                onClick={() => vary.mutate({ assetId: asset.id, input: { fileId: image.fileId, count } }, { onSuccess: onAccepted("tạo biến thể") })}
              >
                Tạo {count} biến thể
              </Button>
              {varyEstimate.data ? <CostChip amount={varyEstimate.data.total.amount} estimate /> : null}
            </div>
          </Section>

          <Separator />
          <Section title="Chỉnh sửa bằng prompt">
            <FormField label="Yêu cầu chỉnh sửa" id="edit-prompt" helper="VD: Đổi nền sang màu xanh dương, giữ nguyên người.">
              <Textarea rows={3} value={editPrompt} onChange={(e) => setEditPrompt(e.target.value)} />
            </FormField>
            <div className="flex items-center gap-2">
              <Button
                variant="secondary"
                loading={edit.isPending}
                disabled={!editPrompt.trim()}
                onClick={() =>
                  edit.mutate(
                    { assetId: asset.id, input: { fileId: image.fileId, prompt: editPrompt } },
                    {
                      onSuccess: () => {
                        setEditPrompt("");
                        onAccepted("chỉnh sửa")();
                      },
                    },
                  )
                }
              >
                Chỉnh sửa ảnh
              </Button>
              {editEstimate.data ? <CostChip amount={editEstimate.data.total.amount} estimate /> : null}
            </div>
          </Section>
        </>
      )}

      {reviewable ? (
        <>
          <Separator />
          <Section title="Gửi duyệt">
            {asset.requiresTypographyCheck ? (
              <div className="flex items-start gap-2">
                <Checkbox
                  id="typography-checked"
                  checked={typographyChecked}
                  onCheckedChange={(c) => {
                    setTypographyChecked(c === true);
                    setReviewError(null);
                  }}
                  className="mt-0.5"
                />
                <Label htmlFor="typography-checked" className="text-small font-normal">
                  Tôi đã kiểm tra chữ trên poster (chính tả, dấu, số liệu)
                </Label>
              </div>
            ) : null}
            {reviewError ? (
              <p role="alert" className="text-small text-danger">
                {reviewError}
              </p>
            ) : null}
            <Button
              variant="primary"
              className="self-start"
              loading={submit.isPending}
              onClick={() => {
                if (asset.requiresTypographyCheck && !typographyChecked) {
                  setReviewError("Đánh dấu đã kiểm tra chữ trước khi gửi duyệt.");
                  return;
                }
                submit.mutate(
                  { assetId: asset.id, typographyChecked },
                  {
                    onSuccess: () => toast.success("Đã gửi duyệt asset"),
                    onError: (err) => {
                      if (isApiError(err) && err.details.some((d) => d.field === "checklist.typography_checked")) {
                        setReviewError("Đánh dấu đã kiểm tra chữ trước khi gửi duyệt.");
                      }
                    },
                  },
                );
              }}
            >
              Gửi duyệt
            </Button>
          </Section>
        </>
      ) : null}
    </div>
  );
}
