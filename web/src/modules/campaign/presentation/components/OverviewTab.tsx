import { useState, type ReactNode } from "react";
import { useProducts } from "@/modules/brand";
import { formatDate } from "@/shared/lib/format";
import { Button } from "@/shared/ui/button";
import { channelLabel, formatLabel, GOAL_LABELS } from "../../domain/rules";
import type { Campaign } from "../../domain/types";
import { EditCampaignSheet } from "./EditCampaignSheet";
import { MembersDialog } from "./MembersDialog";
import { PlanSection } from "./PlanSection";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[180px_1fr] gap-4 py-2">
      <dt className="text-body text-neutral-500">{label}</dt>
      <dd className="text-body text-neutral-900">{children || "—"}</dd>
    </div>
  );
}

/** Overview: brief + members + content plan checklist (SPEC-05 Campaign Detail). */
export function OverviewTab({ campaign, canEdit }: { campaign: Campaign; canEdit: boolean }) {
  const [editOpen, setEditOpen] = useState(false);
  const [membersOpen, setMembersOpen] = useState(false);
  const products = useProducts(campaign.brandId);
  const productNames = campaign.brief.productIds.map((id) => products.data?.find((p) => p.id === id)?.name ?? "Sản phẩm đã xóa");
  const b = campaign.brief;

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_320px] gap-8">
      <div className="flex flex-col gap-8">
        <section aria-labelledby="brief-heading">
          <div className="mb-2 flex items-center justify-between">
            <h2 id="brief-heading" className="text-section text-neutral-900">
              Brief
            </h2>
            {canEdit ? (
              <Button variant="ghost" onClick={() => setEditOpen(true)}>
                Sửa brief
              </Button>
            ) : null}
          </div>
          <dl className="divide-y divide-neutral-200 border-y border-neutral-200">
            <Row label="Mục tiêu">{GOAL_LABELS[campaign.goal]}</Row>
            <Row label="Sản phẩm">{products.isLoading ? "…" : productNames.join(", ")}</Row>
            <Row label="Đối tượng mục tiêu">{b.targetAudience}</Row>
            <Row label="Thông điệp chính">{b.keyMessage}</Row>
            <Row label="Ưu đãi đã xác thực">{b.verifiedOffer}</Row>
            <Row label="Kênh">{b.channels.map(channelLabel).join(", ")}</Row>
            <Row label="Đầu ra">
              {b.formats.length ? (
                <ul>
                  {b.formats.map((f) => (
                    <li key={f.type}>
                      <span className="tabular">{f.count}</span> × {formatLabel(f.type)}
                    </li>
                  ))}
                </ul>
              ) : null}
            </Row>
            <Row label="Deadline">{b.deadline ? formatDate(b.deadline) : null}</Row>
            <Row label="Thời gian chạy">
              {campaign.startDate || campaign.endDate ? `${campaign.startDate ? formatDate(campaign.startDate) : "…"} – ${campaign.endDate ? formatDate(campaign.endDate) : "…"}` : null}
            </Row>
            <Row label="Ghi chú">{b.notes ? <span className="whitespace-pre-line">{b.notes}</span> : null}</Row>
          </dl>
        </section>
        <PlanSection campaign={campaign} canEdit={canEdit} />
      </div>
      <aside aria-labelledby="members-heading" className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <h2 id="members-heading" className="text-section text-neutral-900">
            Thành viên
          </h2>
          {canEdit ? (
            <Button variant="ghost" size="sm" onClick={() => setMembersOpen(true)}>
              Sửa thành viên
            </Button>
          ) : null}
        </div>
        {campaign.members.length === 0 ? (
          <p className="text-body text-neutral-500">Chưa gán thành viên.</p>
        ) : (
          <ul className="flex flex-col gap-1">
            {campaign.members.map((m) => (
              <li key={m.id} className="text-body text-neutral-900">
                {m.name} <span className="text-small text-neutral-500">{m.email}</span>
              </li>
            ))}
          </ul>
        )}
      </aside>
      {canEdit ? (
        <>
          <EditCampaignSheet campaign={campaign} open={editOpen} onOpenChange={setEditOpen} />
          <MembersDialog campaign={campaign} open={membersOpen} onOpenChange={setMembersOpen} />
        </>
      ) : null}
    </div>
  );
}
