import { useNavigate, useSearchParams } from "react-router";
import { Clapperboard } from "lucide-react";
import { formatDuration, formatRelative } from "@/shared/lib/format";
import { Button } from "@/shared/ui/button";
import { DataTable, EmptyState, PageHeader, StatusBadge, type DataTableColumn } from "@/shared/ui/patterns";
import { useVideoList } from "../../application/useVideo";
import { PRESETS, stageLabel, type VideoProject } from "../../domain/video";
import { CreateVideoForm } from "../components/CreateVideoForm";
import { VideoStepper } from "../components/VideoStepper";

const columns: DataTableColumn<VideoProject>[] = [
  { id: "title", header: "Video", cell: (v) => <span className="font-medium text-neutral-900">{v.title}</span> },
  { id: "stage", header: "Tiến độ", cell: (v) => <span className="text-neutral-700">{stageLabel(v.stage)}</span> },
  { id: "status", header: "Trạng thái", cell: (v) => <StatusBadge kind="asset" status={v.status} /> },
  { id: "format", header: "Khung", cell: (v) => PRESETS[v.settings.preset]?.aspectRatio ?? v.settings.aspectRatio },
  { id: "duration", header: "Thời lượng", align: "right", cell: (v) => formatDuration(v.totalDurationMs / 1000) },
  { id: "updated", header: "Cập nhật", align: "right", cell: (v) => formatRelative(v.updatedAt) },
];

/** /videos — bước 1 (Kịch bản) + danh sách video gần đây. Hỗ trợ ?campaign_id= và ?script_from_asset_id=. */
export default function VideoStudioPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const campaignId = params.get("campaign_id");
  const scriptFromAssetId = params.get("script_from_asset_id");
  const list = useVideoList(campaignId);
  const rows = list.data?.pages.flatMap((p) => p.items);

  return (
    <>
      <PageHeader title="Video Studio" actions={<VideoStepper stage={null} />} />
      <div className="grid grid-cols-[minmax(0,720px)_minmax(0,1fr)] gap-8">
        <section aria-label="Tạo video" className="flex flex-col gap-4">
          <h2 className="text-section font-semibold text-neutral-900">Tạo video mới</h2>
          <CreateVideoForm
            campaignId={campaignId}
            scriptFromAssetId={scriptFromAssetId}
            onCreated={(v) => void navigate(`/videos/${v.assetId}`)}
          />
        </section>
        <section aria-label="Video gần đây" className="flex min-w-0 flex-col gap-4">
          <h2 className="text-section font-semibold text-neutral-900">{campaignId ? "Video của campaign" : "Video gần đây"}</h2>
          <DataTable
            aria-label="Danh sách video"
            columns={columns}
            rows={rows}
            getRowId={(v) => v.id}
            isLoading={list.isLoading}
            error={list.error}
            onRetry={() => void list.refetch()}
            onRowClick={(v) => void navigate(`/videos/${v.id}`)}
            skeletonRows={4}
            empty={<EmptyState icon={Clapperboard} message="Chưa có video nào. Tạo video đầu tiên bằng biểu mẫu bên trái." />}
            footer={
              list.hasNextPage ? (
                <Button variant="ghost" size="sm" onClick={() => void list.fetchNextPage()} loading={list.isFetchingNextPage}>
                  Xem thêm
                </Button>
              ) : null
            }
          />
        </section>
      </div>
    </>
  );
}
