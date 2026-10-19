import { useState } from "react";
import { History } from "lucide-react";
import { formatDateTime } from "@/shared/lib/format";
import { DataTable, EmptyState, type DataTableColumn } from "@/shared/ui/patterns";
import { Sheet, SheetBody, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { useBrandVersions } from "../../application/useBrands";
import type { BrandVersion } from "../../domain/types";

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[140px_1fr] gap-2 border-b border-neutral-100 py-2">
      <dt className="text-small text-neutral-500">{label}</dt>
      <dd className="text-body text-neutral-900">{value || "—"}</dd>
    </div>
  );
}

/** Version history (read-only for everyone; each save creates a version). */
export function VersionsTab({ brandId, currentVersionNo }: { brandId: string; currentVersionNo: number }) {
  const q = useBrandVersions(brandId);
  const [selected, setSelected] = useState<BrandVersion | null>(null);
  const columns: DataTableColumn<BrandVersion>[] = [
    {
      id: "no",
      header: "Phiên bản",
      cell: (v) => (
        <span className="font-medium text-neutral-900">
          Phiên bản {v.versionNo}
          {v.versionNo === currentVersionNo ? <span className="ml-2 text-small font-normal text-neutral-500">(hiện tại)</span> : null}
        </span>
      ),
    },
    { id: "at", header: "Thời điểm lưu", cell: (v) => formatDateTime(v.createdAt) },
    { id: "colors", header: "Màu", cell: (v) => v.profile.colors.map((c) => c.hex).join(", ") || "—" },
    { id: "forbidden", header: "Từ cấm", align: "right", cell: (v) => v.profile.forbiddenWords.length },
  ];
  const p = selected?.profile;
  return (
    <>
      <DataTable
        aria-label="Lịch sử phiên bản"
        columns={columns}
        rows={q.data?.items}
        getRowId={(v) => v.id}
        isLoading={q.isLoading}
        error={q.error}
        onRetry={() => void q.refetch()}
        onRowClick={setSelected}
        empty={<EmptyState icon={History} message="Chưa có phiên bản nào." />}
      />
      <Sheet open={!!selected} onOpenChange={(o) => !o && setSelected(null)}>
        <SheetContent aria-describedby="version-desc">
          <SheetHeader>
            <SheetTitle>Phiên bản {selected?.versionNo}</SheetTitle>
            <SheetDescription id="version-desc" className="text-small text-neutral-500">
              Lưu lúc {selected ? formatDateTime(selected.createdAt) : ""}. Phiên bản cũ chỉ để xem; job đã chạy giữ nguyên phiên bản đã dùng.
            </SheetDescription>
          </SheetHeader>
          <SheetBody>
            {p ? (
              <dl>
                <Row label="Mô tả" value={p.description} />
                <Row label="Màu" value={p.colors.map((c) => `${c.name} ${c.hex}`).join(", ")} />
                <Row label="Font" value={[p.typography.heading, p.typography.body].filter(Boolean).join(" / ")} />
                <Row label="Tone of voice" value={p.toneOfVoice.join(", ")} />
                <Row label="Từ nên dùng" value={p.preferredWords.join(", ")} />
                <Row label="Từ cấm" value={p.forbiddenWords.join(", ")} />
                <Row label="CTA mặc định" value={p.defaultCtas.join(", ")} />
                <Row label="Persona" value={p.audiencePersonas.map((x) => x.name).join(", ")} />
                <Row label="Ghi chú" value={p.notes} />
              </dl>
            ) : null}
          </SheetBody>
        </SheetContent>
      </Sheet>
    </>
  );
}
