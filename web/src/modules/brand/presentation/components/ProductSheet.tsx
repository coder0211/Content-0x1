import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { isApiError } from "@/shared/kernel";
import { Badge } from "@/shared/ui/badge";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { cn } from "@/shared/lib/cn";
import { controlClass, Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { FormField } from "@/shared/ui/patterns";
import { Sheet, SheetBody, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { Textarea } from "@/shared/ui/textarea";
import { toast } from "@/shared/ui/toast";
import { useSaveProduct } from "../../application/useBrands";
import { emptyFacts, issueMessage, offerStatus } from "../../domain/rules";
import { PROOF_TYPES, type Product, type ProductFacts, type ProductStatus } from "../../domain/types";
import { LinesField } from "./LinesField";

const OFFER_BADGE = {
  active: { label: "Đang hiệu lực", tone: "success" },
  expired: { label: "Hết hạn", tone: "danger" },
  upcoming: { label: "Sắp áp dụng", tone: "info" },
} as const;

const PROOF_LABELS: Record<(typeof PROOF_TYPES)[number], string> = {
  testimonial: "Cảm nhận khách hàng",
  statistic: "Số liệu",
  award: "Giải thưởng",
  certification: "Chứng nhận",
  media: "Báo chí",
  other: "Khác",
};

interface Props {
  brandId: string;
  product: Product | null; // null = create
  open: boolean;
  onOpenChange: (open: boolean) => void;
  readOnly: boolean;
}

/** Create/edit a product and its verified facts (ACS-022). */
export function ProductSheet({ brandId, product, open, onOpenChange, readOnly }: Props) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [status, setStatus] = useState<ProductStatus>("active");
  const [facts, setFacts] = useState<ProductFacts>(emptyFacts());
  const [errors, setErrors] = useState<Record<string, string>>({});
  const save = useSaveProduct(brandId);

  useEffect(() => {
    if (!open) return;
    setName(product?.name ?? "");
    setDescription(product?.description ?? "");
    setStatus(product?.status ?? "active");
    setFacts(product?.facts ?? emptyFacts());
    setErrors({});
  }, [open, product]);

  const setF = <K extends keyof ProductFacts>(k: K, v: ProductFacts[K]) => setFacts((f) => ({ ...f, [k]: v }));
  const err = (f: string) => issueMessage(errors[f]);

  const submit = async () => {
    const local: Record<string, string> = {};
    if (!name.trim()) local.name = "required";
    facts.offers.forEach((o, i) => {
      if (!o.text.trim()) local[`facts.offers[${i}].text`] = "required";
      if (o.validFrom && o.validTo && o.validFrom > o.validTo) local[`facts.offers[${i}].valid_to`] = "before_valid_from";
    });
    facts.proof.forEach((p, i) => {
      if (!p.text.trim()) local[`facts.proof[${i}].text`] = "required";
    });
    setErrors(local);
    if (Object.keys(local).length) return;
    try {
      await save.mutateAsync({ id: product?.id, input: { name: name.trim(), description, facts, status } });
      toast.success(product ? "Đã lưu sản phẩm" : "Đã thêm sản phẩm");
      onOpenChange(false);
    } catch (e) {
      if (isApiError(e) && e.code === "VALIDATION_FAILED") setErrors(e.fieldErrors());
      else toast.error(e);
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent aria-describedby="product-sheet-desc">
        <SheetHeader>
          <SheetTitle>{product ? product.name : "Thêm sản phẩm"}</SheetTitle>
          <SheetDescription id="product-sheet-desc" className="text-small text-neutral-500">
            Giá và ưu đãi chỉ được lấy từ đây khi sinh nội dung. Ưu đãi hết hạn không được đưa vào prompt.
          </SheetDescription>
        </SheetHeader>
        <SheetBody>
          <form
            id="product-form"
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              void submit();
            }}
          >
            <fieldset disabled={readOnly} className="flex flex-col gap-4">
              <FormField label="Tên sản phẩm" required error={err("name")}>
                <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={200} />
              </FormField>
              <FormField label="Mô tả">
                <Textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} />
              </FormField>
              <FormField label="USP" helper="Mỗi dòng một điểm khác biệt.">
                <LinesField value={facts.usp} onChange={(v) => setF("usp", v)} />
              </FormField>

              <div className="grid grid-cols-3 gap-2">
                <FormField label="Giá" error={err("facts.price.amount")}>
                  <Input
                    type="number"
                    min={0}
                    inputMode="numeric"
                    value={facts.price?.amount ?? ""}
                    onChange={(e) =>
                      setF("price", e.target.value === "" ? null : { amount: Number(e.target.value), currency: facts.price?.currency ?? "VND", note: facts.price?.note ?? "" })
                    }
                  />
                </FormField>
                <FormField label="Tiền tệ" error={err("facts.price.currency")}>
                  <Input
                    value={facts.price?.currency ?? "VND"}
                    maxLength={3}
                    disabled={!facts.price}
                    onChange={(e) => facts.price && setF("price", { ...facts.price, currency: e.target.value.toUpperCase() })}
                  />
                </FormField>
                <FormField label="Ghi chú giá">
                  <Input value={facts.price?.note ?? ""} disabled={!facts.price} onChange={(e) => facts.price && setF("price", { ...facts.price, note: e.target.value })} />
                </FormField>
              </div>

              <section aria-labelledby="offers-h" className="flex flex-col gap-2">
                <h3 id="offers-h" className="text-section text-neutral-900">
                  Ưu đãi
                </h3>
                {facts.offers.map((o, i) => {
                  const st = o.status ?? offerStatus(o);
                  const badge = OFFER_BADGE[st];
                  const patch = (v: Partial<typeof o>) => setF("offers", facts.offers.map((x, j) => (j === i ? { ...x, ...v, status: undefined } : x)));
                  return (
                    <div key={i} className="flex flex-col gap-2 rounded border border-neutral-200 p-3">
                      <div className="flex items-center justify-between">
                        <Badge tone={badge.tone}>{badge.label}</Badge>
                        {!readOnly ? (
                          <Button type="button" variant="ghost" size="icon-sm" aria-label={`Xóa ưu đãi ${i + 1}`} onClick={() => setF("offers", facts.offers.filter((_, j) => j !== i))}>
                            <Trash2 />
                          </Button>
                        ) : null}
                      </div>
                      <FormField label={`Nội dung ưu đãi ${i + 1}`} required error={err(`facts.offers[${i}].text`)}>
                        <Input value={o.text} onChange={(e) => patch({ text: e.target.value })} />
                      </FormField>
                      <div className="grid grid-cols-2 gap-2">
                        <FormField label="Từ ngày" error={err(`facts.offers[${i}].valid_from`)}>
                          <Input type="date" value={o.validFrom ?? ""} onChange={(e) => patch({ validFrom: e.target.value || null })} />
                        </FormField>
                        <FormField label="Đến ngày" error={err(`facts.offers[${i}].valid_to`)}>
                          <Input type="date" value={o.validTo ?? ""} onChange={(e) => patch({ validTo: e.target.value || null })} />
                        </FormField>
                      </div>
                    </div>
                  );
                })}
                {!readOnly ? (
                  <Button type="button" size="sm" className="self-start" onClick={() => setF("offers", [...facts.offers, { text: "", validFrom: null, validTo: null }])}>
                    <Plus /> Thêm ưu đãi
                  </Button>
                ) : null}
              </section>

              <FormField label="Chính sách" helper="Mỗi dòng một chính sách, ví dụ: Hoàn phí trong 7 ngày.">
                <LinesField value={facts.policies} onChange={(v) => setF("policies", v)} rows={2} />
              </FormField>

              <section aria-labelledby="proof-h" className="flex flex-col gap-2">
                <h3 id="proof-h" className="text-section text-neutral-900">
                  Bằng chứng
                </h3>
                <p className="text-small text-neutral-500">Chỉ bằng chứng đã xác thực mới được dùng khi sinh nội dung.</p>
                {facts.proof.map((p, i) => {
                  const patch = (v: Partial<typeof p>) => setF("proof", facts.proof.map((x, j) => (j === i ? { ...x, ...v } : x)));
                  return (
                    <div key={i} className="flex flex-col gap-2 rounded border border-neutral-200 p-3">
                      <div className="flex items-center gap-2">
                        <select
                          aria-label={`Loại bằng chứng ${i + 1}`}
                          className={cn(controlClass, "h-8 w-44")}
                          value={p.type}
                          onChange={(e) => patch({ type: e.target.value as typeof p.type })}
                        >
                          <option value="">Chọn loại</option>
                          {PROOF_TYPES.map((t) => (
                            <option key={t} value={t}>
                              {PROOF_LABELS[t]}
                            </option>
                          ))}
                        </select>
                        <div className="flex items-center gap-2">
                          <Checkbox id={`proof-verified-${i}`} checked={p.verified} onCheckedChange={(v) => patch({ verified: v === true })} />
                          <Label htmlFor={`proof-verified-${i}`}>Đã xác thực</Label>
                        </div>
                        {!readOnly ? (
                          <Button type="button" variant="ghost" size="icon-sm" className="ml-auto" aria-label={`Xóa bằng chứng ${i + 1}`} onClick={() => setF("proof", facts.proof.filter((_, j) => j !== i))}>
                            <Trash2 />
                          </Button>
                        ) : null}
                      </div>
                      <FormField label={`Nội dung bằng chứng ${i + 1}`} required error={err(`facts.proof[${i}].text`)}>
                        <Textarea rows={2} value={p.text} onChange={(e) => patch({ text: e.target.value })} />
                      </FormField>
                      <FormField label="Nguồn">
                        <Input value={p.source} onChange={(e) => patch({ source: e.target.value })} />
                      </FormField>
                    </div>
                  );
                })}
                {!readOnly ? (
                  <Button type="button" size="sm" className="self-start" onClick={() => setF("proof", [...facts.proof, { type: "", text: "", source: "", verified: false }])}>
                    <Plus /> Thêm bằng chứng
                  </Button>
                ) : null}
              </section>

              {product ? (
                <div className="flex items-center gap-2">
                  <Checkbox id="product-archived" checked={status === "archived"} onCheckedChange={(v) => setStatus(v === true ? "archived" : "active")} />
                  <Label htmlFor="product-archived">Lưu trữ sản phẩm (không dùng khi sinh nội dung)</Label>
                </div>
              ) : null}
            </fieldset>
          </form>
        </SheetBody>
        <SheetFooter>
          <Button type="button" onClick={() => onOpenChange(false)}>
            {readOnly ? "Đóng" : "Hủy"}
          </Button>
          {!readOnly ? (
            <Button type="submit" form="product-form" variant="primary" loading={save.isPending}>
              {product ? "Lưu sản phẩm" : "Thêm sản phẩm"}
            </Button>
          ) : null}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
