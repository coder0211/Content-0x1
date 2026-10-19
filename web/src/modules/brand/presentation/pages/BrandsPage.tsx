import { useState } from "react";
import { useNavigate } from "react-router";
import { Plus, SwatchBook } from "lucide-react";
import { isApiError } from "@/shared/kernel";
import { formatDate } from "@/shared/lib/format";
import { Can, useCan } from "@/shared/lib/session";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { DataTable, EmptyState, FormField, PageHeader, type DataTableColumn } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";
import { useBrands, useCreateBrand } from "../../application/useBrands";
import { emptyProfile, issueMessage } from "../../domain/rules";
import type { Brand } from "../../domain/types";

function CreateBrandDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const create = useCreateBrand();
  const navigate = useNavigate();
  const submit = async () => {
    if (!name.trim()) return setError("required");
    try {
      const b = await create.mutateAsync({ name: name.trim(), profile: emptyProfile() });
      toast.success("Đã tạo brand");
      onOpenChange(false);
      setName("");
      void navigate(`/brands/${b.id}`);
    } catch (e) {
      if (isApiError(e) && e.fieldErrors().name) setError(e.fieldErrors().name!);
      else toast.error(e);
    }
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <DialogHeader>
            <DialogTitle>Tạo brand</DialogTitle>
            <DialogDescription>Sau khi tạo, bạn nhập màu, tone of voice và từ cấm ở trang hồ sơ.</DialogDescription>
          </DialogHeader>
          <FormField label="Tên brand" required error={issueMessage(error ?? undefined)}>
            <Input autoFocus value={name} maxLength={200} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <DialogFooter>
            <Button type="button" onClick={() => onOpenChange(false)}>
              Hủy
            </Button>
            <Button type="submit" variant="primary" loading={create.isPending}>
              Tạo brand
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export default function BrandsPage() {
  const [q, setQ] = useState("");
  const brands = useBrands({ q: q.trim() || undefined, limit: 50 });
  const canManage = useCan("brand.manage");
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();

  const columns: DataTableColumn<Brand>[] = [
    { id: "name", header: "Brand", cell: (b) => <span className="font-medium text-neutral-900">{b.name}</span> },
    {
      id: "colors",
      header: "Màu",
      cell: (b) => (
        <span className="flex items-center gap-1">
          {b.currentVersion.profile.colors.slice(0, 6).map((c) => (
            <span key={c.name + c.hex} title={`${c.name} ${c.hex}`} className="size-4 rounded-sm border border-neutral-200" style={{ backgroundColor: c.hex }} />
          ))}
          {b.currentVersion.profile.colors.length === 0 ? "—" : null}
        </span>
      ),
    },
    { id: "version", header: "Phiên bản", align: "right", cell: (b) => b.currentVersion.versionNo },
    { id: "updated", header: "Cập nhật", cell: (b) => formatDate(b.updatedAt) },
  ];

  return (
    <div>
      <PageHeader
        title="Brand"
        description="Hồ sơ thương hiệu, sản phẩm và tài nguyên dùng khi sinh nội dung."
        actions={
          <Can permission="brand.manage">
            <Button variant="primary" onClick={() => setOpen(true)}>
              <Plus /> Tạo brand
            </Button>
          </Can>
        }
      />
      <div className="mb-4 max-w-xs">
        <Input type="search" aria-label="Tìm brand" placeholder="Tìm theo tên" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <DataTable
        aria-label="Danh sách brand"
        columns={columns}
        rows={brands.data?.items}
        getRowId={(b) => b.id}
        isLoading={brands.isLoading}
        error={brands.error}
        onRetry={() => void brands.refetch()}
        onRowClick={(b) => void navigate(`/brands/${b.id}`)}
        empty={
          <EmptyState
            icon={SwatchBook}
            message={q ? "Không có brand nào khớp." : "Chưa có brand nào."}
            action={
              canManage && !q ? (
                <Button size="sm" onClick={() => setOpen(true)}>
                  Tạo brand
                </Button>
              ) : undefined
            }
          />
        }
      />
      <CreateBrandDialog open={open} onOpenChange={setOpen} />
    </div>
  );
}
