import { useEffect, useMemo, useState } from "react";
import { Check, ImageOff, Upload } from "lucide-react";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { Skeleton } from "@/shared/ui/skeleton";
import { EmptyState, ErrorState } from "@/shared/ui/patterns";
import { cn } from "@/shared/lib/cn";
import { useImageReferences, useUploadReference } from "../../application/useImageStudio";
import { MAX_REFERENCES, validateReferenceFile, type ReferenceImage, type StudioTarget } from "../../domain/image";

function previewUrl(file: File): string {
  return typeof URL.createObjectURL === "function" ? URL.createObjectURL(file) : "";
}

function Thumb({ item, selected, onToggle, disabled }: { item: ReferenceImage; selected: boolean; onToggle: () => void; disabled: boolean }) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      aria-label={item.label || "Ảnh tham chiếu"}
      disabled={disabled && !selected}
      onClick={onToggle}
      className={cn(
        "relative flex aspect-square items-center justify-center overflow-hidden rounded border bg-neutral-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50",
        selected ? "border-2 border-accent-600" : "border-neutral-200 hover:border-neutral-300",
      )}
    >
      {item.thumbnailUrl ? (
        <img src={item.thumbnailUrl} alt="" className="size-full object-contain" />
      ) : (
        <ImageOff className="size-5 text-neutral-400" aria-hidden />
      )}
      {selected ? (
        <span className="absolute right-1 top-1 flex size-5 items-center justify-center rounded-sm bg-accent-600 text-white">
          <Check className="size-3" aria-hidden />
        </span>
      ) : null}
    </button>
  );
}

/** Reference picker: brand assets, images already generated (library), or a new upload (presigned flow). */
export function ReferencePicker({
  open,
  onOpenChange,
  target,
  value,
  onChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  target: StudioTarget | null;
  value: ReferenceImage[];
  onChange: (refs: ReferenceImage[]) => void;
}) {
  const refs = useImageReferences(open ? target : null);
  const upload = useUploadReference();
  const [draft, setDraft] = useState<ReferenceImage[]>(value);
  const [uploads, setUploads] = useState<ReferenceImage[]>([]);
  const [fileError, setFileError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(value);
      setFileError(null);
    }
  }, [open, value]);

  const brand = useMemo(() => refs.data?.filter((r) => r.source === "brand") ?? [], [refs.data]);
  const library = useMemo(() => refs.data?.filter((r) => r.source === "library") ?? [], [refs.data]);
  const full = draft.length >= MAX_REFERENCES;
  const isSelected = (id: string) => draft.some((d) => d.fileId === id);
  const toggle = (item: ReferenceImage) =>
    setDraft((d) => (d.some((x) => x.fileId === item.fileId) ? d.filter((x) => x.fileId !== item.fileId) : d.length >= MAX_REFERENCES ? d : [...d, item]));

  const onFile = (file: File | undefined) => {
    if (!file) return;
    const err = validateReferenceFile(file);
    setFileError(err);
    if (err) return;
    upload.mutate(file, {
      onSuccess: (fileId) => {
        const item: ReferenceImage = { fileId, source: "upload", label: file.name, thumbnailUrl: previewUrl(file) };
        setUploads((u) => [item, ...u]);
        setDraft((d) => (d.length >= MAX_REFERENCES ? d : [...d, item]));
      },
    });
  };

  const grid = (items: ReferenceImage[], empty: string) => {
    if (refs.isPending) {
      return (
        <div className="grid grid-cols-4 gap-2" aria-busy="true">
          {Array.from({ length: 4 }, (_, i) => (
            <Skeleton key={i} className="aspect-square" />
          ))}
        </div>
      );
    }
    if (refs.isError) return <ErrorState error={refs.error} onRetry={() => void refs.refetch()} />;
    if (items.length === 0) return <EmptyState icon={ImageOff} message={empty} />;
    return (
      <div className="grid max-h-80 grid-cols-4 gap-2 overflow-y-auto">
        {items.map((it) => (
          <Thumb key={it.fileId} item={it} selected={isSelected(it.fileId)} disabled={full} onToggle={() => toggle(it)} />
        ))}
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Chọn ảnh tham chiếu</DialogTitle>
          <DialogDescription>
            Đã chọn {draft.length}/{MAX_REFERENCES} ảnh. Chỉ nhận PNG, JPEG hoặc WebP.
          </DialogDescription>
        </DialogHeader>
        <Tabs defaultValue="brand">
          <TabsList>
            <TabsTrigger value="brand">Brand</TabsTrigger>
            <TabsTrigger value="library">Thư viện</TabsTrigger>
            <TabsTrigger value="upload">Tải lên</TabsTrigger>
          </TabsList>
          <TabsContent value="brand">{grid(brand, "Brand chưa có logo hay ảnh tham chiếu.")}</TabsContent>
          <TabsContent value="library">{grid(library, "Chưa có ảnh nào được tạo ở đây.")}</TabsContent>
          <TabsContent value="upload" className="flex flex-col gap-3">
            <label className="flex h-24 cursor-pointer flex-col items-center justify-center gap-1 rounded border border-dashed border-neutral-300 bg-neutral-50 text-small text-neutral-700 hover:bg-neutral-100">
              <Upload className="size-5 text-neutral-500" aria-hidden />
              <span>{upload.isPending ? "Đang tải ảnh lên…" : "Chọn tệp ảnh từ máy"}</span>
              <input
                type="file"
                aria-label="Chọn tệp ảnh"
                accept="image/png,image/jpeg,image/webp"
                className="sr-only"
                disabled={upload.isPending || full}
                onChange={(e) => {
                  onFile(e.target.files?.[0]);
                  e.target.value = "";
                }}
              />
            </label>
            {fileError ? (
              <p role="alert" className="text-small text-danger">
                {fileError}
              </p>
            ) : null}
            {uploads.length > 0 ? (
              <div className="grid grid-cols-4 gap-2">
                {uploads.map((it) => (
                  <Thumb key={it.fileId} item={it} selected={isSelected(it.fileId)} disabled={full} onToggle={() => toggle(it)} />
                ))}
              </div>
            ) : null}
          </TabsContent>
        </Tabs>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              onChange(draft);
              onOpenChange(false);
            }}
          >
            Dùng {draft.length} ảnh
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
