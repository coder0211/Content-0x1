import { useMemo } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { FileText, LayoutGrid, Library, List } from "lucide-react";
import { formatDate } from "@/shared/lib/format";
import { cn } from "@/shared/lib/cn";
import { Button } from "@/shared/ui/button";
import { Skeleton } from "@/shared/ui/skeleton";
import { DataTable, EmptyState, ErrorState, PageHeader, StatusBadge, type DataTableColumn } from "@/shared/ui/patterns";
import { useAssets, useCampaignOptions } from "../../application/useLibrary";
import {
  ASSET_TYPE_LABELS,
  EMPTY_FILTERS,
  filtersFromParams,
  filtersToParams,
  hasActiveFilters,
  type AssetFilters,
  type AssetSummary,
} from "../../domain/asset";
import { LibraryFilters } from "../components/LibraryFilters";

type View = "grid" | "list";

export default function LibraryPage() {
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const filters = useMemo(() => filtersFromParams(params), [params]);
  const view: View = params.get("view") === "grid" ? "grid" : params.get("view") === "list" ? "list" : filters.type && filters.type !== "content" ? "grid" : "list";

  const assets = useAssets(filters);
  const campaigns = useCampaignOptions();
  const campaignName = useMemo(() => new Map((campaigns.data ?? []).map((c) => [c.id, c.name])), [campaigns.data]);
  const rows = assets.data?.pages.flatMap((p) => p.items);

  const apply = (f: AssetFilters, v: View | null = params.get("view") as View | null) => {
    const next = filtersToParams(f);
    if (v) next.set("view", v);
    setParams(next, { replace: true });
  };
  const open = (a: AssetSummary) => navigate(`/library/${a.id}`);

  const empty = (
    <EmptyState
      icon={Library}
      message={hasActiveFilters(filters) ? "Không có asset nào khớp bộ lọc." : "Chưa có asset nào. Asset được tạo từ Content, Image hoặc Video Studio."}
      action={
        hasActiveFilters(filters) ? (
          <Button size="sm" onClick={() => apply(EMPTY_FILTERS)}>
            Xóa bộ lọc
          </Button>
        ) : undefined
      }
    />
  );

  const footer = assets.hasNextPage ? (
    <div className="flex justify-center">
      <Button size="sm" loading={assets.isFetchingNextPage} onClick={() => void assets.fetchNextPage()}>
        Tải thêm
      </Button>
    </div>
  ) : null;

  const columns: DataTableColumn<AssetSummary>[] = [
    {
      id: "title",
      header: "Tiêu đề",
      cell: (a) => (
        <div className="min-w-0">
          <p className="truncate font-medium text-neutral-900">{a.title}</p>
          {a.tags.length ? <p className="truncate text-caption font-normal text-neutral-500">{a.tags.join(", ")}</p> : null}
        </div>
      ),
    },
    { id: "type", header: "Loại", cell: (a) => ASSET_TYPE_LABELS[a.type], width: 80 },
    { id: "campaign", header: "Campaign", cell: (a) => (a.campaignId ? (campaignName.get(a.campaignId) ?? "—") : "—"), width: 220 },
    { id: "status", header: "Trạng thái", cell: (a) => <StatusBadge kind="asset" status={a.status} />, width: 130 },
    { id: "version", header: "Phiên bản", cell: (a) => `v${a.currentVersionNo}`, align: "right", width: 90 },
    { id: "creator", header: "Người tạo", cell: (a) => a.createdByName, width: 160 },
    { id: "created", header: "Ngày tạo", cell: (a) => formatDate(a.createdAt), align: "right", width: 110 },
  ];

  return (
    <>
      <PageHeader
        title="Thư viện"
        description="Tìm, xem phiên bản và tải asset đã duyệt."
        actions={
          <div className="flex items-center gap-1" role="group" aria-label="Kiểu hiển thị">
            <Button variant={view === "grid" ? "secondary" : "ghost"} size="icon" aria-label="Dạng lưới" aria-pressed={view === "grid"} onClick={() => apply(filters, "grid")}>
              <LayoutGrid />
            </Button>
            <Button variant={view === "list" ? "secondary" : "ghost"} size="icon" aria-label="Dạng danh sách" aria-pressed={view === "list"} onClick={() => apply(filters, "list")}>
              <List />
            </Button>
          </div>
        }
      />
      <div className="mb-4">
        <LibraryFilters value={filters} onChange={(f) => apply(f)} campaigns={campaigns.data ?? []} />
      </div>
      {view === "list" ? (
        <DataTable
          aria-label="Danh sách asset"
          columns={columns}
          rows={rows}
          getRowId={(a) => a.id}
          isLoading={assets.isPending}
          error={assets.error}
          onRetry={() => void assets.refetch()}
          onRowClick={open}
          empty={empty}
          footer={footer}
        />
      ) : assets.error ? (
        <ErrorState error={assets.error} onRetry={() => void assets.refetch()} />
      ) : assets.isPending ? (
        <div className="grid grid-cols-4 gap-4" aria-busy>
          {Array.from({ length: 8 }, (_, i) => (
            <Skeleton key={i} className="aspect-square" />
          ))}
        </div>
      ) : rows && rows.length === 0 ? (
        empty
      ) : (
        <div className="flex flex-col gap-4">
          <ul className="grid grid-cols-4 gap-4" aria-label="Lưới asset">
            {rows?.map((a) => (
              <li key={a.id}>
                <button
                  type="button"
                  onClick={() => open(a)}
                  className="flex w-full flex-col overflow-hidden rounded-lg border border-neutral-200 bg-neutral-0 text-left transition-colors duration-150 hover:border-neutral-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <div className="flex aspect-square items-center justify-center bg-neutral-100">
                    {a.thumbnailUrl ? (
                      <img src={a.thumbnailUrl} alt={a.title} className="size-full object-cover" />
                    ) : (
                      <FileText className="size-5 text-neutral-400" aria-hidden />
                    )}
                  </div>
                  <div className={cn("flex flex-col gap-1 p-3")}>
                    <p className="truncate text-body font-medium text-neutral-900">{a.title}</p>
                    <div className="flex items-center justify-between gap-2">
                      <StatusBadge kind="asset" status={a.status} />
                      <span className="text-caption font-normal text-neutral-500">
                        {ASSET_TYPE_LABELS[a.type]} · v{a.currentVersionNo}
                      </span>
                    </div>
                  </div>
                </button>
              </li>
            ))}
          </ul>
          {footer}
        </div>
      )}
    </>
  );
}
