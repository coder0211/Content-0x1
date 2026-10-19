import { useState } from "react";
import { ImagePlus, MessageSquareText, X } from "lucide-react";
import { isApiError, type ApiErrorDetail } from "@/shared/kernel";
import { describeError, describeErrorKind } from "@/shared/lib/errors";
import { formatUsd } from "@/shared/lib/format";
import { useCan } from "@/shared/lib/session";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Label } from "@/shared/ui/label";
import { Textarea } from "@/shared/ui/textarea";
import { toast } from "@/shared/ui/toast";
import { CostChip, FormField } from "@/shared/ui/patterns";
import { useGenerateImages, useImageEstimate, usePromptSuggestions } from "../../application/useImageStudio";
import {
  ASPECT_RATIOS,
  MAX_COUNT,
  MAX_PROMPT_LENGTH,
  MAX_REFERENCES,
  PRESETS,
  PRESET_DEFAULT_RATIO,
  PRESET_LABELS,
  QUALITIES,
  QUALITY_LABELS,
  capabilityErrors,
  defaultOptions,
  validateOptions,
  type FieldErrors,
  type GenerationOptions,
  type ReferenceImage,
  type StudioTarget,
} from "../../domain/image";
import { ReferencePicker } from "./ReferencePicker";
import { Field, Segmented } from "./Segmented";

export const PROMPT_FIELD_ID = "image-prompt";

const COUNTS = Array.from({ length: MAX_COUNT }, (_, i) => ({ value: i + 1, label: String(i + 1) }));

/** Field errors from a 400 (VALIDATION_FAILED) or 422 (CAPABILITY_NOT_SUPPORTED) response. */
function serverFieldErrors(error: unknown): FieldErrors | null {
  if (!isApiError(error)) return null;
  if (error.code === "CAPABILITY_NOT_SUPPORTED") return capabilityErrors(error.details);
  if (error.code === "VALIDATION_FAILED" || error.code === "UNSUPPORTED_MEDIA_TYPE" || error.code === "NOT_FOUND") {
    const out: FieldErrors = {};
    for (const d of error.details as ApiErrorDetail[]) {
      if (d.field === "prompt") out.prompt = d.issue === "too_long" ? `Mô tả tối đa ${MAX_PROMPT_LENGTH} ký tự.` : "Nhập mô tả ảnh.";
      else if (d.field === "count") out.count = "Số ảnh từ 1 đến 4.";
      else if (d.field === "reference_file_ids")
        out.reference_file_ids =
          error.code === "NOT_FOUND"
            ? "Không tìm thấy ảnh tham chiếu hoặc bạn không có quyền dùng ảnh này."
            : error.code === "UNSUPPORTED_MEDIA_TYPE"
              ? "Ảnh tham chiếu phải là PNG, JPEG hoặc WebP."
              : "Ảnh tham chiếu không hợp lệ.";
      else if (d.field === "campaign_id") out.target = "Chọn campaign hoặc brand.";
    }
    return Object.keys(out).length ? out : null;
  }
  return null;
}

/** Left column of Image Studio: prompt, preset, ratio, quality, count, references, estimate, generate. */
export function GenerationForm({ target, onSubmitted }: { target: StudioTarget | null; onSubmitted?: () => void }) {
  const [o, setO] = useState<GenerationOptions>(() => defaultOptions());
  const [refs, setRefs] = useState<ReferenceImage[]>([]);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [formError, setFormError] = useState<unknown>(null);
  const canOverride = useCan("budget.manage");
  const generate = useGenerateImages();
  const suggest = usePromptSuggestions();

  const estimate = useImageEstimate(
    { campaignId: target?.campaignId, operation: "generate", aspectRatio: o.aspectRatio, quality: o.quality, count: o.count },
    target != null,
  );
  const est = estimate.data;
  const overBudget = est?.requiresOverride ?? false;

  const set = <K extends keyof GenerationOptions>(k: K, v: GenerationOptions[K]) => {
    setO((prev) => ({ ...prev, [k]: v }));
    setErrors((e) => ({ ...e, [k === "aspectRatio" ? "aspect_ratio" : k]: undefined }));
  };

  const submit = () => {
    const opts = { ...o, referenceFileIds: refs.map((r) => r.fileId) };
    const v = validateOptions(opts, target != null);
    setErrors(v);
    setFormError(null);
    if (Object.keys(v).length || !target) return;
    generate.mutate(
      { target, options: opts },
      {
        onSuccess: () => {
          toast.success(`Đã gửi yêu cầu tạo ${opts.count} ảnh`, { description: "Bạn có thể rời trang, job vẫn tiếp tục chạy." });
          onSubmitted?.();
        },
        onError: (err) => {
          const fe = serverFieldErrors(err);
          if (fe) setErrors(fe);
          else setFormError(err);
        },
      },
    );
  };

  const capabilityError = isApiError(estimate.error) && estimate.error.code === "CAPABILITY_NOT_SUPPORTED" ? capabilityErrors(estimate.error.details) : {};
  const shown: FieldErrors = { ...capabilityError, ...errors };

  return (
    <form
      noValidate
      aria-label="Tạo ảnh"
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <Field id="image-preset" label="Mục đích">
        <Segmented
          label="Mục đích"
          columns={2}
          value={o.preset}
          options={PRESETS.map((p) => ({ value: p, label: PRESET_LABELS[p] }))}
          onChange={(p) => {
            setO((prev) => ({ ...prev, preset: p, aspectRatio: PRESET_DEFAULT_RATIO[p] }));
            setErrors((e) => ({ ...e, aspect_ratio: undefined }));
          }}
        />
      </Field>

      <FormField label="Mô tả ảnh" required id={PROMPT_FIELD_ID} error={shown.prompt} helper="Mô tả chủ thể, bối cảnh, màu sắc và chữ cần có trên ảnh.">
        <Textarea
          rows={6}
          maxLength={MAX_PROMPT_LENGTH}
          value={o.prompt}
          onChange={(e) => set("prompt", e.target.value)}
          placeholder="VD: Poster khai giảng khóa IELTS, học viên trẻ trong lớp học sáng, tông xanh dương của brand"
        />
      </FormField>

      <div className="-mt-2 flex flex-col gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="self-start"
          loading={suggest.starting || suggest.status === "queued" || suggest.status === "running"}
          disabled={!target}
          onClick={() => suggest.start({ campaignId: target?.campaignId, preset: o.preset })}
        >
          <MessageSquareText aria-hidden />
          Đề xuất prompt từ brief
        </Button>
        {suggest.status === "failed" ? (
          <p role="alert" className="text-small text-danger">
            {describeErrorKind(suggest.errorKind)}
          </p>
        ) : null}
        {suggest.prompts.length > 0 ? (
          <ul aria-label="Prompt đề xuất" className="flex flex-col gap-2">
            {suggest.prompts.map((p, i) => (
              <li key={i} className="rounded border border-neutral-200 bg-neutral-50 p-2">
                <p className="text-small text-neutral-700">{p}</p>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="mt-1"
                  aria-label={`Dùng prompt ${i + 1}`}
                  onClick={() => {
                    set("prompt", p);
                    suggest.dismiss();
                  }}
                >
                  Dùng prompt này
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
      </div>

      {o.preset === "poster" ? (
        <Banner tone="info">Poster có chữ: AI dễ sai chính tả. Bạn cần tự kiểm tra chữ trước khi gửi duyệt.</Banner>
      ) : null}

      <Field id="image-ratio" label="Tỉ lệ khung hình" error={shown.aspect_ratio}>
        <Segmented
          label="Tỉ lệ khung hình"
          value={o.aspectRatio}
          invalid={!!shown.aspect_ratio}
          options={ASPECT_RATIOS.map((r) => ({ value: r, label: r }))}
          onChange={(r) => set("aspectRatio", r)}
        />
      </Field>

      <Field id="image-quality" label="Chất lượng" error={shown.quality}>
        <Segmented
          label="Chất lượng"
          value={o.quality}
          invalid={!!shown.quality}
          options={QUALITIES.map((q) => ({ value: q, label: QUALITY_LABELS[q] }))}
          onChange={(q) => set("quality", q)}
        />
      </Field>

      <Field id="image-count" label="Số ảnh" error={shown.count}>
        <Segmented label="Số ảnh" value={o.count} invalid={!!shown.count} options={COUNTS} onChange={(n) => set("count", n)} />
      </Field>

      <Field id="image-refs" label="Ảnh tham chiếu" error={shown.reference_file_ids} helper="Logo, ảnh sản phẩm hoặc ảnh mẫu (tùy chọn).">
        <div className="flex flex-wrap items-center gap-2">
          {refs.map((r) => (
            <div key={r.fileId} className="relative size-14 overflow-hidden rounded border border-neutral-200 bg-neutral-100">
              {r.thumbnailUrl ? <img src={r.thumbnailUrl} alt={r.label} className="size-full object-contain" /> : null}
              <button
                type="button"
                aria-label={`Bỏ ảnh ${r.label}`}
                onClick={() => setRefs((x) => x.filter((y) => y.fileId !== r.fileId))}
                className="absolute right-0.5 top-0.5 flex size-4 items-center justify-center rounded-sm bg-neutral-0 text-neutral-700 shadow-sm"
              >
                <X className="size-3" aria-hidden />
              </button>
            </div>
          ))}
          {refs.length < MAX_REFERENCES ? (
            <Button type="button" variant="secondary" size="sm" disabled={!target} onClick={() => setPickerOpen(true)}>
              <ImagePlus aria-hidden />
              Chọn ảnh tham chiếu
            </Button>
          ) : null}
        </div>
      </Field>
      <ReferencePicker
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        target={target}
        value={refs}
        onChange={(r) => {
          setRefs(r);
          setErrors((e) => ({ ...e, reference_file_ids: undefined }));
        }}
      />

      {overBudget && est ? (
        <Banner tone="warning">
          Dự toán {formatUsd(est.total.amount)} vượt hạn mức mỗi job {formatUsd(est.perJobLimit.amount)}. Giảm số ảnh hoặc chất lượng
          {canOverride ? ", hoặc chọn duyệt vượt hạn mức." : ", hoặc xin Marketing Lead duyệt."}
        </Banner>
      ) : null}
      {overBudget && canOverride ? (
        <div className="flex items-center gap-2">
          <Checkbox id="image-override" checked={o.budgetOverride} onCheckedChange={(c) => set("budgetOverride", c === true)} />
          <Label htmlFor="image-override" className="text-small font-normal">
            Duyệt vượt hạn mức cho job này
          </Label>
        </div>
      ) : null}

      {shown.target ? (
        <p role="alert" className="text-small text-danger">
          {shown.target}
        </p>
      ) : null}
      {formError ? <Banner tone="danger">{describeError(formError).message}</Banner> : null}

      <div className="flex items-center gap-2">
        <Button type="submit" variant="primary" loading={generate.isPending} disabled={!target || (overBudget && !(canOverride && o.budgetOverride))}>
          Tạo {o.count} ảnh
        </Button>
        {est ? <CostChip amount={est.total.amount} estimate source={`Dự toán theo bảng giá ${est.pricingVersion}`} /> : null}
      </div>
    </form>
  );
}
