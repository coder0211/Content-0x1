import { useState, type FormEvent } from "react";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { FormField } from "@/shared/ui/patterns";
import { Textarea } from "@/shared/ui/textarea";
import { toast } from "@/shared/ui/toast";
import { useCreateModel, useUpdateModel } from "../../application/useProviders";
import { KIND_LABELS, parseCapabilities, type AiModel, type ModelKind, type Provider } from "../../domain/provider";
import { NativeSelect } from "./NativeSelect";

/** Create a model under `provider`, or edit `model` (capabilities, concurrency, default). */
export function ModelDialog({ provider, model, onClose }: { provider: Provider; model: AiModel | null; onClose: () => void }) {
  const [modelCode, setModelCode] = useState(model?.modelCode ?? "");
  const [kind, setKind] = useState<ModelKind>(model?.kind ?? provider.kinds[0] ?? "text");
  const [concurrency, setConcurrency] = useState(String(model?.concurrencyLimit ?? 2));
  const [isDefault, setIsDefault] = useState(model?.isDefault ?? false);
  const [caps, setCaps] = useState(JSON.stringify(model?.capabilities ?? {}, null, 2));
  const [errors, setErrors] = useState<{ modelCode?: string; concurrency?: string; caps?: string }>({});
  const create = useCreateModel();
  const update = useUpdateModel();

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const n = Number(concurrency);
    const parsed = parseCapabilities(caps);
    const next = {
      modelCode: !model && !modelCode.trim() ? "Nhập mã model." : undefined,
      concurrency: Number.isInteger(n) && n >= 1 ? undefined : "Giới hạn song song phải là số nguyên ≥ 1.",
      caps: parsed.ok ? undefined : parsed.error,
    };
    setErrors(next);
    if (next.modelCode || next.concurrency || !parsed.ok) return;
    if (model) {
      await update.mutateAsync({ id: model.id, input: { capabilities: parsed.value, concurrencyLimit: n, isDefault } });
      toast.success(`Đã lưu ${model.modelCode}`);
    } else {
      await create.mutateAsync({ providerId: provider.id, modelCode: modelCode.trim(), kind, capabilities: parsed.value, concurrencyLimit: n, isDefault });
      toast.success(`Đã thêm ${modelCode.trim()}`);
    }
    onClose();
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <form onSubmit={(e) => void submit(e).catch(() => undefined)} noValidate className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>{model ? `Sửa model ${model.modelCode}` : "Thêm model"}</DialogTitle>
            <DialogDescription>{provider.name}</DialogDescription>
          </DialogHeader>
          {model ? null : (
            <>
              <FormField label="Mã model" required error={errors.modelCode}>
                <Input value={modelCode} onChange={(e) => setModelCode(e.target.value)} className="font-mono" spellCheck={false} />
              </FormField>
              <FormField label="Loại">
                <NativeSelect value={kind} onChange={(e) => setKind(e.target.value as ModelKind)}>
                  {provider.kinds.map((k) => (
                    <option key={k} value={k}>
                      {KIND_LABELS[k]}
                    </option>
                  ))}
                </NativeSelect>
              </FormField>
            </>
          )}
          <FormField label="Giới hạn song song" required error={errors.concurrency} helper="Số request đồng thời tối đa tới provider cho model này.">
            <Input type="number" min={1} step={1} value={concurrency} onChange={(e) => setConcurrency(e.target.value)} className="w-32 tabular" />
          </FormField>
          <label className="flex items-center gap-2 text-body text-neutral-900">
            <Checkbox checked={isDefault} onCheckedChange={(c) => setIsDefault(c === true)} />
            Model mặc định cho loại {KIND_LABELS[model?.kind ?? kind].toLowerCase()}
          </label>
          <FormField label="Capabilities (JSON)" error={errors.caps}>
            <Textarea value={caps} onChange={(e) => setCaps(e.target.value)} rows={6} spellCheck={false} className="font-mono text-small" />
          </FormField>
          <DialogFooter>
            <Button type="button" variant="secondary" onClick={onClose}>
              Hủy
            </Button>
            <Button type="submit" variant="primary" loading={create.isPending || update.isPending}>
              {model ? "Lưu" : "Thêm model"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
