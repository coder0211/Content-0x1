import { useState } from "react";
import { useParams, useSearchParams } from "react-router";
import { useBrand } from "@/modules/brand";
import type { CampaignStatus } from "@/shared/kernel";
import { isApiError } from "@/shared/kernel";
import { formatDate } from "@/shared/lib/format";
import { useSession } from "@/shared/lib/session";
import { Button } from "@/shared/ui/button";
import { ConfirmDialog, ErrorState, PageHeader, StatusBadge } from "@/shared/ui/patterns";
import { Skeleton } from "@/shared/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { toast } from "@/shared/ui/toast";
import { useCampaign, useUpdateCampaign } from "../../application/useCampaigns";
import { canEditCampaign, transitionLabel } from "../../domain/rules";
import type { Campaign } from "../../domain/types";
import { AssetsTab } from "../components/AssetsTab";
import { OverviewTab } from "../components/OverviewTab";

const CONFIRM: Partial<Record<CampaignStatus, string>> = {
  completed: "Campaign hoàn thành sẽ không nhận yêu cầu sinh nội dung mới.",
  archived: "Campaign lưu trữ chỉ còn xem được, không sửa hay sinh nội dung mới.",
};

function StatusActions({ campaign }: { campaign: Campaign }) {
  const update = useUpdateCampaign(campaign.id);
  const [confirm, setConfirm] = useState<CampaignStatus | null>(null);
  const move = async (to: CampaignStatus) => {
    try {
      await update.mutateAsync({ status: to });
      toast.success("Đã đổi trạng thái");
      setConfirm(null);
    } catch (e) {
      if (isApiError(e) && e.code === "INVALID_STATE_TRANSITION") toast.error("Không thể chuyển sang trạng thái này.");
      else toast.error(e);
    }
  };
  const forward = campaign.nextStatuses.filter((s) => s !== "archived");
  return (
    <div className="flex items-center gap-2">
      {campaign.nextStatuses.includes("archived") ? (
        <Button variant="ghost" onClick={() => setConfirm("archived")}>
          Lưu trữ campaign
        </Button>
      ) : null}
      {forward.map((s, i) => (
        <Button
          key={s}
          variant={i === forward.length - 1 ? "primary" : "secondary"}
          loading={update.isPending && update.variables?.status === s}
          onClick={() => (CONFIRM[s] ? setConfirm(s) : void move(s))}
        >
          {transitionLabel(campaign.status, s)}
        </Button>
      ))}
      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={confirm ? transitionLabel(campaign.status, confirm) : ""}
        description={confirm ? CONFIRM[confirm] : undefined}
        confirmLabel={confirm ? transitionLabel(campaign.status, confirm) : ""}
        tone={confirm === "archived" ? "danger" : "primary"}
        onConfirm={() => (confirm ? move(confirm) : undefined)}
      />
    </div>
  );
}

export default function CampaignDetailPage() {
  const { campaignId = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const q = useCampaign(campaignId);
  const brand = useBrand(q.data?.brandId);
  const { user } = useSession();

  if (q.isLoading) {
    return (
      <div className="flex flex-col gap-4" aria-busy>
        <Skeleton className="h-7 w-72" />
        <Skeleton className="h-8 w-96" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  if (q.error || !q.data) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  const c = q.data;
  const canEdit = canEditCampaign(c, user);
  const tab = params.get("tab") ?? "overview";

  return (
    <div>
      <PageHeader
        title={c.name}
        meta={<StatusBadge kind="campaign" status={c.status} />}
        description={[brand.data?.name, c.brief.deadline ? `Deadline ${formatDate(c.brief.deadline)}` : null].filter(Boolean).join(" · ")}
        actions={canEdit ? <StatusActions campaign={c} /> : undefined}
      />
      <Tabs value={tab} onValueChange={(v) => setParams(v === "overview" ? {} : { tab: v }, { replace: true })}>
        <TabsList>
          <TabsTrigger value="overview">Tổng quan</TabsTrigger>
          <TabsTrigger value="content">Nội dung</TabsTrigger>
          <TabsTrigger value="images">Hình ảnh</TabsTrigger>
          <TabsTrigger value="videos">Video</TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="pt-4">
          <OverviewTab campaign={c} canEdit={canEdit} />
        </TabsContent>
        <TabsContent value="content" className="pt-4">
          <AssetsTab campaign={c} type="content" />
        </TabsContent>
        <TabsContent value="images" className="pt-4">
          <AssetsTab campaign={c} type="image" />
        </TabsContent>
        <TabsContent value="videos" className="pt-4">
          <AssetsTab campaign={c} type="video" />
        </TabsContent>
      </Tabs>
    </div>
  );
}
