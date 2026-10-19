import { useState } from "react";
import { Cpu, Pencil, Plus, Power, Star } from "lucide-react";
import { formatRelative } from "@/shared/lib/format";
import { useSession } from "@/shared/lib/session";
import { Button } from "@/shared/ui/button";
import { DataTable, EmptyState, PageHeader, StatusBadge, type DataTableColumn, type RowAction } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";
import { useModels, useProviders, useUpdateModel } from "../../application/useProviders";
import { KIND_LABELS, type AiModel, type Provider } from "../../domain/provider";
import { ModelDialog } from "../components/ModelDialog";
import { PricingPanel } from "../components/PricingPanel";
import { ProviderSheet } from "../components/ProviderSheet";

/** /settings/providers — providers, their models and pricing versions (ACS-090). Secrets are never shown. */
export default function ProvidersPage() {
  const { can } = useSession();
  const canManage = can("provider.manage");
  const providers = useProviders();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [editing, setEditing] = useState<Provider | null>(null);
  const selected = providers.data?.find((p) => p.id === selectedId) ?? providers.data?.[0] ?? null;

  const columns: DataTableColumn<Provider>[] = [
    {
      id: "name",
      header: "Provider",
      cell: (p) => (
        <div className="flex flex-col">
          <span className="font-medium text-neutral-900">{p.name}</span>
          <span className="font-mono text-small text-neutral-500">{p.code}</span>
        </div>
      ),
    },
    { id: "kinds", header: "Loại", cell: (p) => p.kinds.map((k) => KIND_LABELS[k]).join(", ") },
    {
      id: "secret",
      header: "Secret",
      cell: (p) => (
        <div className="flex flex-col">
          <span className="font-mono text-small text-neutral-900">{p.secretRef || "—"}</span>
          {p.secretConfigured === false ? <span className="text-small text-danger">Chưa cấu hình trên server</span> : null}
        </div>
      ),
    },
    { id: "status", header: "Trạng thái", cell: (p) => <StatusBadge kind="provider" status={p.status} />, width: 120 },
    { id: "updated", header: "Cập nhật", cell: (p) => (p.updatedAt ? formatRelative(p.updatedAt) : "—"), width: 130 },
  ];

  const rowActions = canManage ? (): RowAction<Provider>[] => [{ label: "Sửa cấu hình", icon: Pencil, onSelect: setEditing }] : undefined;

  return (
    <>
      <PageHeader
        title="Provider & model"
        description={
          canManage
            ? "Bật/tắt provider, cấu hình model và bảng giá. Khóa API chỉ được tham chiếu bằng tên secret, không lưu trong hệ thống."
            : "Chế độ chỉ xem. Cần quyền provider.manage để thay đổi."
        }
      />
      <div className="flex flex-col gap-6">
        <DataTable
          aria-label="Danh sách provider"
          columns={columns}
          rows={providers.data}
          getRowId={(p) => p.id}
          isLoading={providers.isLoading}
          error={providers.error}
          onRetry={() => void providers.refetch()}
          onRowClick={(p) => setSelectedId(p.id)}
          selectedId={selected?.id ?? null}
          rowActions={rowActions}
          empty={<EmptyState icon={Cpu} message="Chưa có provider nào được khai báo." />}
        />
        {selected ? <ModelsSection key={selected.id} provider={selected} canManage={canManage} /> : null}
      </div>
      {editing ? <ProviderSheet key={editing.id} provider={editing} onClose={() => setEditing(null)} /> : null}
    </>
  );
}

function ModelsSection({ provider, canManage }: { provider: Provider; canManage: boolean }) {
  const models = useModels(provider.id);
  const update = useUpdateModel();
  const [dialog, setDialog] = useState<{ model: AiModel | null } | null>(null);
  const [pricingFor, setPricingFor] = useState<string | null>(null);
  const pricingModel = models.data?.find((m) => m.id === pricingFor) ?? null;

  const columns: DataTableColumn<AiModel>[] = [
    {
      id: "code",
      header: "Model",
      cell: (m) => (
        <span className="font-mono text-small text-neutral-900">
          {m.modelCode}
          {m.isDefault ? <span className="ml-2 font-sans text-caption text-accent-700">Mặc định</span> : null}
        </span>
      ),
    },
    { id: "kind", header: "Loại", cell: (m) => KIND_LABELS[m.kind], width: 110 },
    { id: "concurrency", header: "Song song", cell: (m) => m.concurrencyLimit, align: "right", width: 100 },
    { id: "status", header: "Trạng thái", cell: (m) => <StatusBadge kind="provider" status={m.status} />, width: 120 },
  ];

  const rowActions = canManage
    ? (m: AiModel): RowAction<AiModel>[] => [
        { label: "Sửa", icon: Pencil, onSelect: () => setDialog({ model: m }) },
        {
          label: "Đặt làm mặc định",
          icon: Star,
          disabled: m.isDefault || m.status !== "active",
          onSelect: () => void update.mutateAsync({ id: m.id, input: { isDefault: true } }).then(() => toast.success(`${m.modelCode} là model mặc định`)),
        },
        {
          label: m.status === "active" ? "Tắt model" : "Bật model",
          icon: Power,
          tone: m.status === "active" ? "danger" : "default",
          onSelect: () => void update.mutateAsync({ id: m.id, input: { status: m.status === "active" ? "disabled" : "active" } }),
        },
      ]
    : undefined;

  return (
    <section aria-label={`Model của ${provider.name}`} className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <h2 className="text-section text-neutral-900">Model · {provider.name}</h2>
        {canManage ? (
          <Button size="sm" variant="secondary" onClick={() => setDialog({ model: null })}>
            <Plus />
            Thêm model
          </Button>
        ) : null}
      </div>
      <div className="flex gap-6">
        <div className="min-w-0 flex-1">
          <DataTable
            aria-label="Danh sách model"
            columns={columns}
            rows={models.data}
            getRowId={(m) => m.id}
            isLoading={models.isLoading}
            error={models.error}
            onRetry={() => void models.refetch()}
            onRowClick={(m) => setPricingFor(m.id)}
            selectedId={pricingFor}
            rowActions={rowActions}
            empty={<EmptyState icon={Cpu} message="Provider này chưa có model." />}
          />
        </div>
        <aside className="w-[380px] shrink-0" aria-label="Bảng giá">
          {pricingModel ? (
            <PricingPanel key={pricingModel.id} model={pricingModel} canManage={canManage} />
          ) : (
            <p className="text-small text-neutral-500">Chọn một model để xem bảng giá.</p>
          )}
        </aside>
      </div>
      {dialog ? <ModelDialog provider={provider} model={dialog.model} onClose={() => setDialog(null)} /> : null}
    </section>
  );
}
