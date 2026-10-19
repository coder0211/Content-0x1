import { useState, type FormEvent } from "react";
import type { ActiveStatus } from "@/shared/kernel";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { FormField } from "@/shared/ui/patterns";
import { Sheet, SheetBody, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { Switch } from "@/shared/ui/switch";
import { Textarea } from "@/shared/ui/textarea";
import { toast } from "@/shared/ui/toast";
import { useUpdateProvider } from "../../application/useProviders";
import { parseConfigJson, validateSecretRef, type Provider } from "../../domain/provider";

/** Edit provider status / secret reference / config_json. Mount with `key` per provider. */
export function ProviderSheet({ provider, onClose }: { provider: Provider; onClose: () => void }) {
  const [status, setStatus] = useState<ActiveStatus>(provider.status);
  const [secretRef, setSecretRef] = useState(provider.secretRef);
  const [config, setConfig] = useState(JSON.stringify(provider.config, null, 2));
  const [errors, setErrors] = useState<{ secretRef?: string; config?: string }>({});
  const update = useUpdateProvider();

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const refErr = validateSecretRef(secretRef);
    const parsed = parseConfigJson(config);
    const next = { secretRef: refErr ?? undefined, config: parsed.ok ? undefined : parsed.error };
    setErrors(next);
    if (next.secretRef || next.config || !parsed.ok) return;
    await update.mutateAsync({
      id: provider.id,
      input: {
        status: status !== provider.status ? status : undefined,
        secretRef: secretRef.trim() !== provider.secretRef ? secretRef.trim() : undefined,
        config: parsed.value,
      },
    });
    toast.success(`Đã lưu ${provider.name}`);
    onClose();
  };

  return (
    <Sheet open onOpenChange={(o) => !o && onClose()}>
      <SheetContent>
        <form onSubmit={(e) => void submit(e).catch(() => undefined)} noValidate className="flex h-full flex-col">
          <SheetHeader>
            <SheetTitle>{provider.name}</SheetTitle>
            <SheetDescription className="text-small text-neutral-500">{provider.code}</SheetDescription>
          </SheetHeader>
          <SheetBody className="flex flex-col gap-4">
            <label className="flex items-center justify-between gap-3 text-body text-neutral-900">
              <span>
                Bật provider
                <span className="block text-small text-neutral-500">Tắt sẽ chặn job mới dùng provider này.</span>
              </span>
              <Switch checked={status === "active"} onCheckedChange={(c) => setStatus(c ? "active" : "disabled")} aria-label="Bật provider" />
            </label>
            <FormField
              label="Tên secret (secret_ref)"
              required
              error={errors.secretRef}
              helper="Chỉ nhập TÊN biến môi trường/secret, ví dụ PROVIDER_OPENAI_API_KEY. Không dán giá trị khóa."
            >
              <Input value={secretRef} onChange={(e) => setSecretRef(e.target.value)} autoComplete="off" spellCheck={false} className="font-mono" />
            </FormField>
            <FormField label="Cấu hình (JSON)" error={errors.config} helper="Không chứa khóa bí mật. Ví dụ base_url, timeout_seconds.">
              <Textarea value={config} onChange={(e) => setConfig(e.target.value)} rows={10} spellCheck={false} className="font-mono text-small" />
            </FormField>
          </SheetBody>
          <SheetFooter>
            <Button type="button" variant="secondary" onClick={onClose}>
              Hủy
            </Button>
            <Button type="submit" variant="primary" loading={update.isPending}>
              Lưu
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
