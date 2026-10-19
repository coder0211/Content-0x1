import { useState, type FormEvent } from "react";
import { Plus, Trash2 } from "lucide-react";
import { formatDate, formatNumber } from "@/shared/lib/format";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { ErrorState, FormField } from "@/shared/ui/patterns";
import { Skeleton } from "@/shared/ui/skeleton";
import { toast } from "@/shared/ui/toast";
import { useAddPricing, usePricing } from "../../application/useProviders";
import {
  UNIT_SUGGESTIONS,
  groupPricing,
  toNewPricingInput,
  validatePricingDraft,
  type AiModel,
  type PricingDraft,
  type PricingDraftErrors,
} from "../../domain/provider";
import { NativeSelect } from "./NativeSelect";

/** Pricing versions of one model. Old versions are read-only; a new version closes the current one. */
export function PricingPanel({ model, canManage }: { model: AiModel; canManage: boolean }) {
  const pricing = usePricing(model.id);
  const [adding, setAdding] = useState(false);

  if (pricing.error) return <ErrorState error={pricing.error} onRetry={() => void pricing.refetch()} />;
  if (pricing.isPending) return <Skeleton className="h-24 w-full" />;
  const versions = groupPricing(pricing.data);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <h3 className="text-body font-medium text-neutral-900">Bảng giá · {model.modelCode}</h3>
        {canManage && !adding ? (
          <Button size="sm" variant="secondary" onClick={() => setAdding(true)}>
            <Plus />
            Phiên bản giá mới
          </Button>
        ) : null}
      </div>
      {adding ? <NewPricingForm model={model} existing={versions.map((v) => v.pricingVersion)} onDone={() => setAdding(false)} /> : null}
      {versions.length === 0 ? (
        <p className="text-small text-neutral-500">Chưa có bảng giá. Chi phí của model này sẽ không được ước tính.</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {versions.map((v) => (
            <li key={v.pricingVersion} className="rounded border border-neutral-200 p-3">
              <p className="text-small text-neutral-900">
                <span className="font-medium">{v.pricingVersion}</span>
                <span className="text-neutral-500">
                  {" "}
                  · từ {formatDate(v.effectiveFrom)}
                  {v.effectiveTo ? ` đến ${formatDate(v.effectiveTo)}` : ""} · {v.currency}
                </span>
                {v.current ? <span className="ml-2 text-caption text-success">Đang áp dụng</span> : null}
              </p>
              <table className="mt-2 w-full text-small">
                <tbody>
                  {v.rows.map((r) => (
                    <tr key={r.id} className="border-t border-neutral-100">
                      <td className="py-1 font-mono text-neutral-700">{r.unit}</td>
                      <td className="py-1 text-right tabular text-neutral-900">{formatNumber(r.unitPrice)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function NewPricingForm({ model, existing, onDone }: { model: AiModel; existing: string[]; onDone: () => void }) {
  const [draft, setDraft] = useState<PricingDraft>(() => ({
    pricingVersion: "",
    effectiveFrom: new Date().toISOString().slice(0, 10),
    currency: "USD",
    rows: UNIT_SUGGESTIONS[model.kind].map((unit) => ({ unit, unitPrice: "" })),
  }));
  const [errors, setErrors] = useState<PricingDraftErrors>({});
  const add = useAddPricing(model.id);
  const set = (patch: Partial<PricingDraft>) => setDraft((d) => ({ ...d, ...patch }));
  const setRow = (i: number, patch: Partial<PricingDraft["rows"][number]>) =>
    set({ rows: draft.rows.map((r, j) => (j === i ? { ...r, ...patch } : r)) });

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const errs = validatePricingDraft(draft, existing);
    setErrors(errs);
    if (Object.keys(errs).length) return;
    await add.mutateAsync(toNewPricingInput(draft));
    toast.success(`Đã thêm bảng giá ${draft.pricingVersion.trim()}`);
    onDone();
  };

  return (
    <form onSubmit={(e) => void submit(e).catch(() => undefined)} noValidate className="flex flex-col gap-3 rounded border border-neutral-200 bg-neutral-50 p-3" aria-label="Phiên bản giá mới">
      <div className="flex flex-wrap gap-3">
        <FormField label="Mã phiên bản" required error={errors.pricing_version} className="w-48">
          <Input value={draft.pricingVersion} onChange={(e) => set({ pricingVersion: e.target.value })} placeholder="vd. 2026-11" />
        </FormField>
        <FormField label="Hiệu lực từ" required error={errors.effective_from} className="w-44">
          <Input type="date" value={draft.effectiveFrom} onChange={(e) => set({ effectiveFrom: e.target.value })} />
        </FormField>
        <FormField label="Tiền tệ" className="w-28">
          <NativeSelect value={draft.currency} onChange={(e) => set({ currency: e.target.value })}>
            <option value="USD">USD</option>
            <option value="VND">VND</option>
          </NativeSelect>
        </FormField>
      </div>
      <div className="flex flex-col gap-2" role="group" aria-label="Đơn giá">
        {draft.rows.map((r, i) => (
          <div key={i} className="flex items-center gap-2">
            <Input aria-label={`Đơn vị dòng ${i + 1}`} value={r.unit} onChange={(e) => setRow(i, { unit: e.target.value })} className="h-8 w-56 font-mono" />
            <Input
              aria-label={`Đơn giá dòng ${i + 1}`}
              inputMode="decimal"
              value={r.unitPrice}
              onChange={(e) => setRow(i, { unitPrice: e.target.value })}
              className="h-8 w-36 tabular"
              placeholder="0.000002"
            />
            <Button type="button" size="sm" variant="ghost" aria-label={`Xóa dòng ${i + 1}`} onClick={() => set({ rows: draft.rows.filter((_, j) => j !== i) })}>
              <Trash2 />
            </Button>
          </div>
        ))}
        <Button type="button" size="sm" variant="ghost" className="self-start" onClick={() => set({ rows: [...draft.rows, { unit: "", unitPrice: "" }] })}>
          <Plus />
          Thêm dòng
        </Button>
        {errors.rows ? (
          <p role="alert" className="text-small text-danger">
            {errors.rows}
          </p>
        ) : null}
      </div>
      <p className="text-small text-neutral-500">Bảng giá đang áp dụng sẽ kết thúc vào ngày hiệu lực của phiên bản mới. Bảng giá cũ không sửa được.</p>
      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" size="sm" onClick={onDone}>
          Hủy
        </Button>
        <Button type="submit" variant="primary" size="sm" loading={add.isPending}>
          Lưu bảng giá
        </Button>
      </div>
    </form>
  );
}
