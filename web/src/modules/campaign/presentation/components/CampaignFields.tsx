import { Plus, Trash2 } from "lucide-react";
import { activeOffers, useBrandOptions, useProducts } from "@/modules/brand";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { controlClass, Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { FormField } from "@/shared/ui/patterns";
import { Skeleton } from "@/shared/ui/skeleton";
import { Textarea } from "@/shared/ui/textarea";
import { CHANNELS, FORMATS, GOALS, issueMessage, type FieldErrors } from "../../domain/rules";
import type { CampaignBrief, CampaignDraft, CampaignGoal } from "../../domain/types";

export function HeaderFields({
  value,
  onChange,
  errors,
  brandLocked,
}: {
  value: CampaignDraft;
  onChange: (d: CampaignDraft) => void;
  errors: FieldErrors;
  brandLocked?: boolean;
}) {
  const brands = useBrandOptions();
  const set = (p: Partial<CampaignDraft>) => onChange({ ...value, ...p });
  return (
    <div className="flex flex-col gap-4">
      <FormField label="Tên campaign" required error={issueMessage(errors.name)}>
        <Input value={value.name} maxLength={200} onChange={(e) => set({ name: e.target.value })} />
      </FormField>
      <div className="grid grid-cols-2 gap-4">
        <FormField label="Brand" required error={issueMessage(errors.brand_id)}>
          <select
            className={controlClass}
            value={value.brandId}
            disabled={brandLocked || brands.isLoading}
            onChange={(e) => set({ brandId: e.target.value, brief: { ...value.brief, productIds: [], verifiedOffer: "" } })}
          >
            <option value="">Chọn brand</option>
            {brands.options.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </FormField>
        <FormField label="Mục tiêu" required error={issueMessage(errors.goal)}>
          <select className={controlClass} value={value.goal} onChange={(e) => set({ goal: e.target.value as CampaignGoal | "" })}>
            <option value="">Chọn mục tiêu</option>
            {GOALS.map((g) => (
              <option key={g.value} value={g.value}>
                {g.label}
              </option>
            ))}
          </select>
        </FormField>
        <FormField label="Ngày bắt đầu" error={issueMessage(errors.start_date)}>
          <Input type="date" value={value.startDate} onChange={(e) => set({ startDate: e.target.value })} />
        </FormField>
        <FormField label="Ngày kết thúc" error={issueMessage(errors.end_date)}>
          <Input type="date" value={value.endDate} onChange={(e) => set({ endDate: e.target.value })} />
        </FormField>
      </div>
    </div>
  );
}

function toggle(list: string[], v: string, on: boolean): string[] {
  return on ? (list.includes(v) ? list : [...list, v]) : list.filter((x) => x !== v);
}

export function BriefFields({
  brandId,
  value,
  onChange,
  errors,
}: {
  brandId: string;
  value: CampaignBrief;
  onChange: (b: CampaignBrief) => void;
  errors: FieldErrors;
}) {
  const products = useProducts(brandId || undefined, "active");
  const set = (p: Partial<CampaignBrief>) => onChange({ ...value, ...p });
  const selected = (products.data ?? []).filter((p) => value.productIds.includes(p.id));
  const offers = selected.flatMap((p) => activeOffers(p.facts).map((o) => ({ product: p.name, text: o.text })));
  const offerKnown = !value.verifiedOffer || offers.some((o) => o.text === value.verifiedOffer);
  const formatError = Object.entries(errors).find(([k]) => k.startsWith("brief.formats["));

  return (
    <div className="flex flex-col gap-4">
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1.5 text-body font-medium text-neutral-900">Sản phẩm</legend>
        {products.isLoading ? (
          <Skeleton className="h-6 w-64" />
        ) : (products.data ?? []).length === 0 ? (
          <p className="text-small text-neutral-500">{brandId ? "Brand chưa có sản phẩm đang bán." : "Chọn brand trước."}</p>
        ) : (
          (products.data ?? []).map((p) => (
            <div key={p.id} className="flex items-center gap-2">
              <Checkbox
                id={`brief-product-${p.id}`}
                checked={value.productIds.includes(p.id)}
                onCheckedChange={(v) => set({ productIds: toggle(value.productIds, p.id, v === true), verifiedOffer: v === true ? value.verifiedOffer : "" })}
              />
              <Label htmlFor={`brief-product-${p.id}`} className="font-normal">
                {p.name}
              </Label>
            </div>
          ))
        )}
      </fieldset>
      <FormField label="Đối tượng mục tiêu" required error={issueMessage(errors["brief.target_audience"])}>
        <Textarea rows={2} value={value.targetAudience} onChange={(e) => set({ targetAudience: e.target.value })} />
      </FormField>
      <FormField label="Thông điệp chính" required error={issueMessage(errors["brief.key_message"])}>
        <Textarea rows={2} value={value.keyMessage} onChange={(e) => set({ keyMessage: e.target.value })} />
      </FormField>
      <FormField
        label="Ưu đãi đã xác thực"
        helper="Chỉ chọn ưu đãi còn hạn của sản phẩm đã chọn. AI không được tự tạo ưu đãi khác."
        error={issueMessage(errors["brief.verified_offer"])}
      >
        <select className={controlClass} value={value.verifiedOffer} onChange={(e) => set({ verifiedOffer: e.target.value })}>
          <option value="">Không dùng ưu đãi</option>
          {!offerKnown ? <option value={value.verifiedOffer}>{value.verifiedOffer} (không còn trong danh sách)</option> : null}
          {offers.map((o) => (
            <option key={`${o.product}-${o.text}`} value={o.text}>
              {o.text} — {o.product}
            </option>
          ))}
        </select>
      </FormField>
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1.5 text-body font-medium text-neutral-900">
          Kênh<span className="ml-0.5 text-danger" aria-hidden>*</span>
        </legend>
        <div className="flex flex-wrap gap-x-4 gap-y-2">
          {CHANNELS.map((c) => (
            <div key={c.value} className="flex items-center gap-2">
              <Checkbox id={`brief-channel-${c.value}`} checked={value.channels.includes(c.value)} onCheckedChange={(v) => set({ channels: toggle(value.channels, c.value, v === true) })} />
              <Label htmlFor={`brief-channel-${c.value}`} className="font-normal">
                {c.label}
              </Label>
            </div>
          ))}
        </div>
        {errors["brief.channels"] ? (
          <p role="alert" className="text-small text-danger">
            {issueMessage(errors["brief.channels"])}
          </p>
        ) : null}
      </fieldset>
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1.5 text-body font-medium text-neutral-900">
          Đầu ra cần sản xuất<span className="ml-0.5 text-danger" aria-hidden>*</span>
        </legend>
        {value.formats.map((f, i) => (
          <div key={i} className="flex items-center gap-2">
            <select
              aria-label={`Loại đầu ra ${i + 1}`}
              className={cn(controlClass, "w-56")}
              value={f.type}
              aria-invalid={errors[`brief.formats[${i}].type`] ? true : undefined}
              onChange={(e) => set({ formats: value.formats.map((x, j) => (j === i ? { ...x, type: e.target.value } : x)) })}
            >
              <option value="">Chọn loại</option>
              {FORMATS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            <Input
              aria-label={`Số lượng ${i + 1}`}
              type="number"
              min={1}
              max={50}
              className="w-24 tabular"
              value={Number.isFinite(f.count) ? f.count : ""}
              aria-invalid={errors[`brief.formats[${i}].count`] ? true : undefined}
              onChange={(e) => set({ formats: value.formats.map((x, j) => (j === i ? { ...x, count: e.target.valueAsNumber } : x)) })}
            />
            <Button type="button" size="icon" variant="ghost" aria-label={`Xóa đầu ra ${i + 1}`} onClick={() => set({ formats: value.formats.filter((_, j) => j !== i) })}>
              <Trash2 />
            </Button>
          </div>
        ))}
        <div>
          <Button type="button" size="sm" onClick={() => set({ formats: [...value.formats, { type: "", count: 1 }] })}>
            <Plus /> Thêm đầu ra
          </Button>
        </div>
        {errors["brief.formats"] || formatError ? (
          <p role="alert" className="text-small text-danger">
            {issueMessage(errors["brief.formats"] ?? formatError?.[1])}
          </p>
        ) : null}
      </fieldset>
      <FormField label="Deadline" required error={issueMessage(errors["brief.deadline"])}>
        <Input type="date" className="w-48" value={value.deadline} onChange={(e) => set({ deadline: e.target.value })} />
      </FormField>
      <FormField label="Ghi chú" error={issueMessage(errors["brief.notes"])}>
        <Textarea rows={3} value={value.notes} onChange={(e) => set({ notes: e.target.value })} />
      </FormField>
    </div>
  );
}
