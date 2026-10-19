import { Link } from "react-router";
import { Clapperboard, Image as ImageIcon, PenLine, Plus, type LucideIcon } from "lucide-react";
import { formatDate } from "@/shared/lib/format";
import { useCan } from "@/shared/lib/session";
import { buttonVariants } from "@/shared/ui/button";
import { DataTable, EmptyState, StatusBadge, type DataTableColumn } from "@/shared/ui/patterns";
import { useCampaignAssets } from "../../application/useCampaigns";
import { acceptsGeneration, studioLink } from "../../domain/rules";
import type { Campaign, CampaignAsset, CampaignAssetType } from "../../domain/types";

const META: Record<CampaignAssetType, { icon: LucideIcon; noun: string }> = {
  content: { icon: PenLine, noun: "nội dung" },
  image: { icon: ImageIcon, noun: "hình ảnh" },
  video: { icon: Clapperboard, noun: "video" },
};

/** Content / Images / Videos tab: the campaign's assets + "Tạo mới" into the matching studio (ACS-026). */
export function AssetsTab({ campaign, type }: { campaign: Campaign; type: CampaignAssetType }) {
  const q = useCampaignAssets(campaign.id, type);
  const canGenerate = useCan("generate.run") && acceptsGeneration(campaign.status);
  const { icon, noun } = META[type];
  const href = studioLink(type, campaign.id);

  const columns: DataTableColumn<CampaignAsset>[] = [
    { id: "title", header: "Tiêu đề", cell: (a) => <span className="font-medium text-neutral-900">{a.title || "Chưa đặt tiêu đề"}</span> },
    { id: "status", header: "Trạng thái", cell: (a) => <StatusBadge kind="asset" status={a.status} /> },
    { id: "created", header: "Tạo lúc", cell: (a) => formatDate(a.createdAt) },
    { id: "updated", header: "Cập nhật", cell: (a) => formatDate(a.updatedAt) },
  ];

  return (
    <div className="flex flex-col gap-4">
      {canGenerate ? (
        <div className="flex justify-end">
          <Link to={href} className={buttonVariants({ size: "sm" })}>
            <Plus /> Tạo mới
          </Link>
        </div>
      ) : null}
      <DataTable
        aria-label={`Danh sách ${noun}`}
        columns={columns}
        rows={q.data?.items}
        getRowId={(a) => a.id}
        isLoading={q.isLoading}
        error={q.error}
        onRetry={() => void q.refetch()}
        empty={
          <EmptyState
            icon={icon}
            message={`Campaign chưa có ${noun} nào.`}
            action={
              canGenerate ? (
                <Link to={href} className={buttonVariants({ size: "sm" })}>
                  Tạo {noun}
                </Link>
              ) : undefined
            }
          />
        }
      />
    </div>
  );
}
