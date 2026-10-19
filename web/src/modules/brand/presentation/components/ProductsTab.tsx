import { useState } from "react";
import { Package, Plus } from "lucide-react";
import { formatMoney } from "@/shared/lib/format";
import { Badge } from "@/shared/ui/badge";
import { Button } from "@/shared/ui/button";
import { DataTable, EmptyState, StatusBadge, type DataTableColumn } from "@/shared/ui/patterns";
import { useProducts } from "../../application/useBrands";
import { offerStatus } from "../../domain/rules";
import type { Product } from "../../domain/types";
import { ProductSheet } from "./ProductSheet";

export function ProductsTab({ brandId, canManage }: { brandId: string; canManage: boolean }) {
  const q = useProducts(brandId);
  const [editing, setEditing] = useState<Product | null>(null);
  const [open, setOpen] = useState(false);

  const openSheet = (p: Product | null) => {
    setEditing(p);
    setOpen(true);
  };

  const columns: DataTableColumn<Product>[] = [
    { id: "name", header: "Sản phẩm", cell: (p) => <span className="font-medium text-neutral-900">{p.name}</span> },
    { id: "price", header: "Giá", align: "right", cell: (p) => (p.facts.price ? formatMoney(p.facts.price) : "—") },
    {
      id: "offers",
      header: "Ưu đãi",
      cell: (p) => {
        const expired = p.facts.offers.filter((o) => (o.status ?? offerStatus(o)) === "expired").length;
        const active = p.facts.offers.filter((o) => (o.status ?? offerStatus(o)) === "active").length;
        return (
          <span className="flex items-center gap-2">
            <span>{active} đang hiệu lực</span>
            {expired > 0 ? <Badge tone="danger">{expired} hết hạn</Badge> : null}
          </span>
        );
      },
    },
    { id: "proof", header: "Bằng chứng đã xác thực", align: "right", cell: (p) => p.facts.proof.filter((x) => x.verified).length },
    { id: "status", header: "Trạng thái", cell: (p) => <StatusBadge kind="brand" status={p.status} /> },
  ];

  return (
    <div className="flex flex-col gap-4">
      {canManage ? (
        <div className="flex justify-end">
          <Button size="sm" onClick={() => openSheet(null)}>
            <Plus /> Thêm sản phẩm
          </Button>
        </div>
      ) : null}
      <DataTable
        aria-label="Danh sách sản phẩm"
        columns={columns}
        rows={q.data}
        getRowId={(p) => p.id}
        isLoading={q.isLoading}
        error={q.error}
        onRetry={() => void q.refetch()}
        onRowClick={(p) => openSheet(p)}
        empty={
          <EmptyState
            icon={Package}
            message="Chưa có sản phẩm nào."
            action={
              canManage ? (
                <Button size="sm" onClick={() => openSheet(null)}>
                  Thêm sản phẩm
                </Button>
              ) : undefined
            }
          />
        }
      />
      <ProductSheet brandId={brandId} product={editing} open={open} onOpenChange={setOpen} readOnly={!canManage} />
    </div>
  );
}
