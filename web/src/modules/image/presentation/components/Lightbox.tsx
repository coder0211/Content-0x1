import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/shared/ui/dialog";
import type { ImageBatch } from "../../domain/image";

/** 100% preview of a batch's images; ←/→ switch images (SPEC-05 §4.3). */
export function Lightbox({
  batch,
  index,
  onIndexChange,
  onClose,
}: {
  batch: ImageBatch | null;
  index: number;
  onIndexChange: (i: number) => void;
  onClose: () => void;
}) {
  const images = batch?.asset?.images ?? [];
  const img = images[index];
  const n = images.length;
  const go = (step: number) => n > 0 && onIndexChange((index + step + n) % n);
  return (
    <Dialog open={!!batch && !!img} onOpenChange={(o) => (o ? null : onClose())}>
      <DialogContent
        className="max-w-5xl"
        onKeyDown={(e) => {
          if (e.key === "ArrowLeft") go(-1);
          if (e.key === "ArrowRight") go(1);
        }}
      >
        <DialogTitle className="pr-8">{batch?.asset?.title ?? "Xem ảnh"}</DialogTitle>
        <DialogDescription>
          Ảnh {index + 1}/{n}
          {img ? ` · ${img.width}×${img.height}px` : ""}
        </DialogDescription>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="icon" aria-label="Ảnh trước" disabled={n < 2} onClick={() => go(-1)}>
            <ChevronLeft aria-hidden />
          </Button>
          <div className="flex min-w-0 flex-1 items-center justify-center bg-neutral-100">
            {img ? <img src={img.url} alt={batch?.asset?.title ?? ""} className="max-h-[75vh] max-w-full object-contain" /> : null}
          </div>
          <Button variant="ghost" size="icon" aria-label="Ảnh sau" disabled={n < 2} onClick={() => go(1)}>
            <ChevronRight aria-hidden />
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
