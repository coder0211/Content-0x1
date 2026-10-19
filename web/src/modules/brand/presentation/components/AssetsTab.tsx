import { useState } from "react";
import { FileUp, ImageIcon, Trash2 } from "lucide-react";
import { isApiError } from "@/shared/kernel";
import { cn } from "@/shared/lib/cn";
import { formatDate, formatFileSize } from "@/shared/lib/format";
import { Badge } from "@/shared/ui/badge";
import { Button } from "@/shared/ui/button";
import { controlClass, Input } from "@/shared/ui/input";
import { ConfirmDialog, DataTable, EmptyState, FormField, type DataTableColumn } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";
import { useAddBrandAsset, useBrandAssets, useDeleteBrandAsset } from "../../application/useBrands";
import { ASSET_TYPE_LABELS, ASSET_UPLOAD, issueMessage, UPLOADABLE_ASSET_TYPES } from "../../domain/rules";
import type { BrandAsset, BrandAssetType } from "../../domain/types";

function UploadForm({ brandId }: { brandId: string }) {
  const [type, setType] = useState<BrandAssetType>("logo");
  const [label, setLabel] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [source, setSource] = useState("");
  const [licenseType, setLicenseType] = useState("");
  const [expiresAt, setExpiresAt] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formKey, setFormKey] = useState(0);
  const add = useAddBrandAsset(brandId);
  const music = type === "music";

  const submit = async () => {
    const local: Record<string, string> = {};
    if (!file) local.file = "required";
    if (music && !source.trim()) local["license.source"] = "required";
    if (music && !licenseType.trim()) local["license.license_type"] = "required";
    setErrors(local);
    if (!file || Object.keys(local).length) return;
    try {
      await add.mutateAsync({
        file,
        assetType: type,
        label: label.trim(),
        license: music ? { source: source.trim(), licenseType: licenseType.trim(), expiresAt: expiresAt || undefined } : undefined,
      });
      toast.success("Đã tải lên tài nguyên");
      setLabel("");
      setFile(null);
      setSource("");
      setLicenseType("");
      setExpiresAt("");
      setFormKey((k) => k + 1);
    } catch (e) {
      if (isApiError(e) && e.code === "VALIDATION_FAILED") setErrors(e.fieldErrors());
      else toast.error(e);
    }
  };

  return (
    <form
      key={formKey}
      aria-label="Tải lên tài nguyên"
      className="flex flex-col gap-3 rounded-lg border border-neutral-200 p-4"
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      <div className="grid grid-cols-3 gap-3">
        <FormField label="Loại tài nguyên" required>
          <select className={cn(controlClass, "h-8")} value={type} onChange={(e) => setType(e.target.value as BrandAssetType)}>
            {UPLOADABLE_ASSET_TYPES.map((t) => (
              <option key={t} value={t}>
                {ASSET_TYPE_LABELS[t]}
              </option>
            ))}
          </select>
        </FormField>
        <FormField label="Nhãn">
          <Input value={label} onChange={(e) => setLabel(e.target.value)} maxLength={300} placeholder="Ví dụ: Logo nền trắng" />
        </FormField>
        <FormField label="File" required error={issueMessage(errors.file) ?? issueMessage(errors.file_id)}>
          <Input type="file" accept={ASSET_UPLOAD[type].accept} className="pt-1" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        </FormField>
      </div>
      {music ? (
        <div className="grid grid-cols-3 gap-3">
          <FormField label="Nguồn nhạc" required error={issueMessage(errors["license.source"]) ?? issueMessage(errors.license)}>
            <Input value={source} onChange={(e) => setSource(e.target.value)} placeholder="Ví dụ: Artlist" />
          </FormField>
          <FormField label="Loại giấy phép" required error={issueMessage(errors["license.license_type"])}>
            <Input value={licenseType} onChange={(e) => setLicenseType(e.target.value)} placeholder="Ví dụ: commercial" />
          </FormField>
          <FormField label="Hết hạn" helper="Để trống nếu không thời hạn." error={issueMessage(errors["license.expires_at"])}>
            <Input type="date" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} />
          </FormField>
        </div>
      ) : null}
      {music ? <p className="text-small text-neutral-500">Nhạc thiếu giấy phép hợp lệ sẽ không được dùng trong video.</p> : null}
      <Button type="submit" className="self-start" loading={add.isPending}>
        <FileUp /> Tải lên tài nguyên
      </Button>
    </form>
  );
}

/** Logo, reference images, documents and licensed music (ACS-023). */
export function AssetsTab({ brandId, canManage }: { brandId: string; canManage: boolean }) {
  const q = useBrandAssets(brandId);
  const del = useDeleteBrandAsset(brandId);
  const [toDelete, setToDelete] = useState<BrandAsset | null>(null);
  const columns: DataTableColumn<BrandAsset>[] = [
    { id: "type", header: "Loại", cell: (a) => ASSET_TYPE_LABELS[a.assetType] },
    {
      id: "label",
      header: "Nhãn / file",
      cell: (a) => (
        <span className="flex flex-col">
          <span className="text-neutral-900">{a.label || a.file?.originalName || "—"}</span>
          {a.file ? <span className="text-small text-neutral-500">{a.file.originalName}</span> : null}
        </span>
      ),
    },
    { id: "size", header: "Dung lượng", align: "right", cell: (a) => (a.file ? formatFileSize(a.file.sizeBytes) : "—") },
    {
      id: "license",
      header: "Giấy phép",
      cell: (a) =>
        a.assetType !== "music" ? (
          "—"
        ) : a.licenseValid ? (
          <Badge tone="success">Hợp lệ{a.license?.expiresAt ? ` đến ${formatDate(a.license.expiresAt)}` : ""}</Badge>
        ) : (
          <Badge tone="danger">Không dùng được</Badge>
        ),
    },
    { id: "at", header: "Ngày thêm", cell: (a) => formatDate(a.createdAt) },
  ];
  return (
    <div className="flex flex-col gap-4">
      {canManage ? <UploadForm brandId={brandId} /> : null}
      <DataTable
        aria-label="Tài nguyên brand"
        columns={columns}
        rows={q.data}
        getRowId={(a) => a.id}
        isLoading={q.isLoading}
        error={q.error}
        onRetry={() => void q.refetch()}
        rowActions={canManage ? (a) => [{ label: "Gỡ khỏi brand", icon: Trash2, tone: "danger", onSelect: () => setToDelete(a) }] : undefined}
        empty={<EmptyState icon={ImageIcon} message="Chưa có tài nguyên nào." />}
      />
      <ConfirmDialog
        open={!!toDelete}
        onOpenChange={(o) => !o && setToDelete(null)}
        title="Gỡ tài nguyên khỏi brand?"
        description="File vẫn được giữ trong kho lưu trữ theo chính sách lưu trữ."
        confirmLabel="Gỡ tài nguyên"
        tone="danger"
        onConfirm={async () => {
          if (toDelete) await del.mutateAsync(toDelete.id);
          setToDelete(null);
        }}
      />
    </div>
  );
}
