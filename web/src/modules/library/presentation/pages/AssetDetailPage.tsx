import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { Download, History, RotateCcw, Send, Trash2 } from "lucide-react";
import { isApiError } from "@/shared/kernel";
import { formatDateTime } from "@/shared/lib/format";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Label } from "@/shared/ui/label";
import { Skeleton } from "@/shared/ui/skeleton";
import { Textarea } from "@/shared/ui/textarea";
import { ConfirmDialog, ErrorState, PageHeader, StatusBadge } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";
import {
  useAsset,
  useAssetVersions,
  useDeleteAsset,
  useDownloadAsset,
  useRestoreVersion,
  useSubmitReview,
} from "../../application/useLibrary";
import { ASSET_TYPE_LABELS, requiresTypographyCheck, type AssetDetail } from "../../domain/asset";
import { AssetVersionContent } from "../components/AssetVersionContent";
import { ApprovalHistory, Field, Panel, ProvenancePanel, TagsRightsPanel } from "../components/AssetSidePanels";

export default function AssetDetailPage() {
  const { assetId = "" } = useParams();
  const asset = useAsset(assetId);

  if (asset.error) {
    return (
      <>
        <PageHeader title="Chi tiết asset" />
        <ErrorState error={asset.error} onRetry={() => void asset.refetch()} />
      </>
    );
  }
  if (!asset.data) {
    return (
      <div aria-busy className="flex flex-col gap-4">
        <Skeleton className="h-8 w-80" />
        <Skeleton className="h-96" />
      </div>
    );
  }
  return <AssetDetailView key={asset.data.id} asset={asset.data} />;
}

function AssetDetailView({ asset }: { asset: AssetDetail }) {
  const navigate = useNavigate();
  const versions = useAssetVersions(asset.id);
  const restore = useRestoreVersion(asset.id);
  const submit = useSubmitReview(asset.id);
  const download = useDownloadAsset(asset.id);
  const remove = useDeleteAsset(asset.id);

  const [viewNo, setViewNo] = useState<number | null>(null);
  const [submitOpen, setSubmitOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [comment, setComment] = useState("");
  const [typography, setTypography] = useState(false);

  const shown = (viewNo !== null && versions.data?.find((v) => v.versionNo === viewNo)) || asset.currentVersion;
  const isCurrent = shown.versionNo === asset.currentVersionNo;
  const needsTypography = requiresTypographyCheck(asset);
  const downloadForbidden = isApiError(download.error) && download.error.status === 403;

  const onDownload = () => {
    download.mutate(undefined, {
      onSuccess: (link) => {
        window.open(link.url, "_blank", "noopener");
      },
    });
  };

  return (
    <>
      <PageHeader
        title={asset.title}
        meta={<StatusBadge kind="asset" status={asset.status} />}
        description={`${ASSET_TYPE_LABELS[asset.type]}${asset.subtype ? ` · ${asset.subtype}` : ""} · phiên bản ${asset.currentVersionNo}`}
        actions={
          <>
            {asset.permissions.canEdit ? (
              <Button variant="ghost" onClick={() => setDeleteOpen(true)}>
                <Trash2 aria-hidden />
                Xóa asset
              </Button>
            ) : null}
            {asset.permissions.canDownload ? (
              <Button variant="secondary" onClick={onDownload} loading={download.isPending}>
                <Download aria-hidden />
                Tải xuống
              </Button>
            ) : null}
            {asset.permissions.canSubmit ? (
              <Button variant="primary" onClick={() => setSubmitOpen(true)}>
                <Send aria-hidden />
                Gửi duyệt
              </Button>
            ) : null}
          </>
        }
      />

      {download.error ? (
        <Banner tone={downloadForbidden ? "warning" : "danger"} className="mb-4">
          {downloadForbidden
            ? "Chỉ tải được asset đã duyệt. Gửi duyệt và chờ Reviewer duyệt trước khi tải."
            : "Không tạo được link tải. Thử lại sau ít phút."}
        </Banner>
      ) : null}
      {asset.status === "changes_requested" ? (
        <Banner tone="warning" className="mb-4">
          Reviewer yêu cầu sửa. Lưu phiên bản mới trong studio rồi gửi duyệt lại.
        </Banner>
      ) : null}

      <div className="grid grid-cols-[minmax(0,1fr)_300px] gap-6">
        <div className="flex min-w-0 flex-col gap-6">
          <section className="rounded-lg border border-neutral-200 bg-neutral-0 p-6" aria-label="Nội dung phiên bản">
            <div className="mb-4 flex items-center justify-between">
              <p className="text-small text-neutral-500">
                Phiên bản {shown.versionNo}
                {isCurrent ? " (hiện tại)" : ""} · {shown.createdByName} · {formatDateTime(shown.createdAt)}
              </p>
              {!isCurrent ? (
                <Button size="sm" variant="ghost" onClick={() => setViewNo(null)}>
                  Xem bản hiện tại
                </Button>
              ) : null}
            </div>
            <AssetVersionContent type={asset.type} title={asset.title} version={shown} />
          </section>

          <section aria-label="Phiên bản" className="flex flex-col gap-2">
            <h2 className="flex items-center gap-2 text-section text-neutral-900">
              <History className="size-4" aria-hidden />
              Phiên bản
            </h2>
            {versions.error ? (
              <ErrorState error={versions.error} onRetry={() => void versions.refetch()} />
            ) : !versions.data ? (
              <Skeleton className="h-24" />
            ) : (
              <ul className="divide-y divide-neutral-200 rounded-lg border border-neutral-200 bg-neutral-0">
                {versions.data.map((v) => (
                  <li key={v.id} className="flex items-center gap-3 px-4 py-2 text-body">
                    <span className="w-10 font-medium tabular">v{v.versionNo}</span>
                    <span className="min-w-0 flex-1 truncate text-neutral-700">
                      {v.changeNote ?? (v.generatedByJobId ? "Sinh bởi AI" : "Sửa thủ công")}
                    </span>
                    <span className="text-small text-neutral-500">
                      {v.createdByName} · {formatDateTime(v.createdAt)}
                    </span>
                    <Button size="sm" variant="ghost" onClick={() => setViewNo(v.versionNo)} aria-label={`Xem phiên bản ${v.versionNo}`}>
                      Xem
                    </Button>
                    {asset.permissions.canEdit && v.versionNo !== asset.currentVersionNo ? (
                      <Button
                        size="sm"
                        variant="secondary"
                        loading={restore.isPending && restore.variables === v.versionNo}
                        aria-label={`Khôi phục phiên bản ${v.versionNo}`}
                        onClick={() =>
                          restore.mutate(v.versionNo, {
                            onSuccess: (nv) => {
                              setViewNo(null);
                              toast.success(`Đã khôi phục thành phiên bản ${nv.versionNo}`);
                            },
                          })
                        }
                      >
                        <RotateCcw aria-hidden />
                        Khôi phục
                      </Button>
                    ) : null}
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>

        <aside className="flex flex-col gap-4" aria-label="Thông tin asset">
          <Panel title="Thông tin">
            <dl className="flex flex-col gap-1.5">
              <Field label="Người tạo">{asset.createdByName}</Field>
              <Field label="Ngày tạo">{formatDateTime(asset.createdAt)}</Field>
              <Field label="Cập nhật">{formatDateTime(asset.updatedAt)}</Field>
              {asset.campaignId ? (
                <Field label="Campaign">
                  <Link className="text-accent-700 hover:underline" to={`/campaigns/${asset.campaignId}`}>
                    Mở campaign
                  </Link>
                </Field>
              ) : null}
              {asset.sourceAssetId ? (
                <Field label="Chuyển thể từ">
                  <Link className="text-accent-700 hover:underline" to={`/library/${asset.sourceAssetId}`}>
                    Asset gốc
                  </Link>
                </Field>
              ) : null}
            </dl>
          </Panel>
          <ProvenancePanel provenance={shown.provenance} />
          <TagsRightsPanel asset={asset} />
          <ApprovalHistory approvals={asset.approvals} />
        </aside>
      </div>

      <ConfirmDialog
        open={submitOpen}
        onOpenChange={setSubmitOpen}
        title="Gửi duyệt asset"
        description="Reviewer sẽ nhận thông báo. Bạn không sửa được trạng thái cho đến khi có kết quả."
        confirmLabel="Gửi duyệt"
        confirmDisabled={needsTypography && !typography}
        onConfirm={async () => {
          await submit.mutateAsync({ comment, typographyChecked: needsTypography ? typography : undefined });
          toast.success("Đã gửi duyệt");
        }}
      >
        <div className="flex flex-col gap-3">
          {needsTypography ? (
            <div className="flex items-start gap-2">
              <Checkbox id="typography" checked={typography} onCheckedChange={(v) => setTypography(v === true)} />
              <Label htmlFor="typography" className="font-normal">
                Tôi đã kiểm tra chữ trên poster (chính tả, dấu, không bị cắt)
              </Label>
            </div>
          ) : null}
          <Textarea aria-label="Ghi chú cho reviewer" placeholder="Ghi chú cho reviewer (không bắt buộc)" value={comment} onChange={(e) => setComment(e.target.value)} />
        </div>
      </ConfirmDialog>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Xóa asset"
        description="Asset sẽ bị ẩn khỏi thư viện. Thao tác được ghi vào nhật ký."
        confirmLabel="Xóa asset"
        tone="danger"
        onConfirm={async () => {
          await remove.mutateAsync();
          toast.success("Đã xóa asset");
          navigate("/library");
        }}
      />
    </>
  );
}
