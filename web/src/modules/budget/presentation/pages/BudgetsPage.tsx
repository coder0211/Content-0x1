import { useState, type FormEvent } from "react";
import { Pencil, Wallet } from "lucide-react";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Skeleton } from "@/shared/ui/skeleton";
import { Switch } from "@/shared/ui/switch";
import { toast } from "@/shared/ui/toast";
import { DataTable, EmptyState, ErrorState, FormField, PageHeader, type DataTableColumn } from "@/shared/ui/patterns";
import { formatDate, formatDateTime, formatUsd } from "@/shared/lib/format";
import { useCan } from "@/shared/lib/session";
import { useBudgetSettings, useUpdateBudgetSettings } from "../../application/useBudgetSettings";
import { parseAmount, parseThresholds, type Budget, type BudgetSettings } from "../../domain/budget";
import { BudgetMeter } from "../components/BudgetMeter";
import { CampaignBudgetDialog } from "../components/CampaignBudgetDialog";
import { UsageSubnav } from "../components/UsageSubnav";

function period(b: Budget) {
  return `${formatDate(`${b.periodStart}T00:00:00`)} – ${formatDate(`${b.periodEnd}T00:00:00`)}`;
}

export default function BudgetsPage() {
  const q = useBudgetSettings();
  const canManage = useCan("budget.manage");
  const [editing, setEditing] = useState<Budget | null>(null);

  const columns: DataTableColumn<Budget>[] = [
    { id: "campaign", header: "Campaign", cell: (b) => b.campaignName ?? b.campaignId },
    { id: "period", header: "Kỳ ngân sách", cell: period },
    { id: "limit", header: "Hạn mức", align: "right", cell: (b) => formatUsd(b.limitAmount) },
    { id: "spent", header: "Đã chi", align: "right", cell: (b) => formatUsd(b.spentAmount) },
    { id: "reserved", header: "Đang giữ", align: "right", cell: (b) => formatUsd(b.reservedAmount) },
    { id: "remaining", header: "Còn lại", align: "right", cell: (b) => formatUsd(b.remainingAmount) },
    { id: "pct", header: "Đã dùng", align: "right", cell: (b) => `${Math.round(b.percentUsed)}%` },
    { id: "hard", header: "Hard stop", cell: (b) => (b.hardStop ? "Bật" : "Tắt") },
  ];

  return (
    <>
      <PageHeader title="Chi phí & ngân sách" description="Hạn mức chi phí AI: toàn hệ thống theo tháng, mỗi job và từng campaign." />
      <UsageSubnav />
      {q.error ? (
        <ErrorState error={q.error} onRetry={() => void q.refetch()} />
      ) : !q.data ? (
        <div className="flex flex-col gap-4" aria-busy>
          <Skeleton className="h-28" />
          <Skeleton className="h-64" />
        </div>
      ) : (
        <div className="flex flex-col gap-8">
          <GlobalSection settings={q.data} canManage={canManage} key={q.data.updatedAt} />
          <section aria-labelledby="campaign-budgets" className="flex flex-col gap-3">
            <h2 id="campaign-budgets" className="text-section text-neutral-900">
              Ngân sách campaign
            </h2>
            <DataTable
              aria-label="Ngân sách campaign"
              columns={columns}
              rows={q.data.campaigns}
              getRowId={(b) => b.id}
              rowActions={canManage ? () => [{ label: "Sửa ngân sách", icon: Pencil, onSelect: setEditing }] : undefined}
              empty={<EmptyState icon={Wallet} message="Chưa campaign nào có ngân sách riêng. Đặt ngân sách trong trang chi tiết campaign." />}
            />
          </section>
        </div>
      )}
      {editing && editing.campaignId ? (
        <CampaignBudgetDialog
          campaignId={editing.campaignId}
          campaignName={editing.campaignName}
          budget={editing}
          open
          onOpenChange={(o) => {
            if (!o) setEditing(null);
          }}
        />
      ) : null}
    </>
  );
}

function GlobalSection({ settings, canManage }: { settings: BudgetSettings; canManage: boolean }) {
  const g = settings.global;
  const [limit, setLimit] = useState(String(g.limitAmount));
  const [thresholds, setThresholds] = useState(g.alertThresholds.join(", "));
  const [hardStop, setHardStop] = useState(g.hardStop);
  const [perJob, setPerJob] = useState({
    text: String(settings.perJobLimits.text),
    image: String(settings.perJobLimits.image),
    video: String(settings.perJobLimits.video),
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const update = useUpdateBudgetSettings();

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const errs: Record<string, string> = {};
    const lim = parseAmount(limit);
    if (lim === null) errs.limit = "Nhập số tiền USD hợp lệ.";
    const th = parseThresholds(thresholds);
    if (!th.ok) errs.thresholds = th.error;
    const pj = { text: parseAmount(perJob.text), image: parseAmount(perJob.image), video: parseAmount(perJob.video) };
    (Object.keys(pj) as (keyof typeof pj)[]).forEach((k) => {
      if (pj[k] === null) errs[k] = "Nhập số tiền USD hợp lệ.";
    });
    setErrors(errs);
    if (Object.keys(errs).length > 0 || lim === null || !th.ok || pj.text === null || pj.image === null || pj.video === null) return;
    update.mutate(
      { global: { limitAmount: lim, alertThresholds: th.value, hardStop }, perJobLimits: { text: pj.text, image: pj.image, video: pj.video } },
      { onSuccess: () => toast.success("Đã lưu cài đặt ngân sách") },
    );
  };

  const exhausted = g.percentUsed >= 100;
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-6">
      <section aria-labelledby="global-budget" className="flex flex-col gap-3 rounded border border-neutral-200 bg-neutral-0 p-4">
        <div className="flex items-baseline justify-between gap-3">
          <h2 id="global-budget" className="text-section text-neutral-900">
            Ngân sách toàn hệ thống
          </h2>
          <span className="text-small text-neutral-500">Kỳ {period(g)}</span>
        </div>
        {exhausted && g.hardStop ? (
          <Banner tone="danger" title="Đã hết ngân sách tháng">
            Job AI mới đang bị chặn. Tăng hạn mức hoặc nhờ Admin duyệt vượt ngân sách.
          </Banner>
        ) : g.percentUsed >= 80 ? (
          <Banner tone="warning" title={`Đã dùng ${Math.round(g.percentUsed)}% ngân sách tháng`}>
            Cảnh báo đã gửi cho Marketing Lead.
          </Banner>
        ) : null}
        <BudgetMeter budget={g} />
        <div className="grid grid-cols-3 gap-4">
          <FormField label="Hạn mức tháng (USD)" required error={errors.limit}>
            <Input inputMode="decimal" value={limit} onChange={(e) => setLimit(e.target.value)} disabled={!canManage} />
          </FormField>
          <FormField label="Ngưỡng cảnh báo (%)" helper="Mỗi ngưỡng gửi thông báo 1 lần mỗi tháng" error={errors.thresholds}>
            <Input value={thresholds} onChange={(e) => setThresholds(e.target.value)} disabled={!canManage} />
          </FormField>
          <div className="flex flex-col gap-1.5">
            <span className="text-small font-medium text-neutral-700">Khi hết ngân sách</span>
            <label className="flex h-8 items-center gap-2 text-body text-neutral-700">
              <Switch checked={hardStop} onCheckedChange={setHardStop} disabled={!canManage} aria-label="Chặn job mới khi hết ngân sách tháng" />
              Chặn job mới (Admin có thể duyệt vượt)
            </label>
          </div>
        </div>
      </section>

      <section aria-labelledby="per-job" className="flex flex-col gap-3 rounded border border-neutral-200 bg-neutral-0 p-4">
        <h2 id="per-job" className="text-section text-neutral-900">
          Hạn mức mỗi job
        </h2>
        <p className="text-small text-neutral-500">Dự toán vượt hạn mức sẽ bị từ chối, trừ khi Marketing Lead hoặc Admin duyệt vượt.</p>
        <div className="grid grid-cols-3 gap-4">
          {(
            [
              ["text", "Nội dung (USD)"],
              ["image", "Ảnh (USD)"],
              ["video", "Video (USD)"],
            ] as const
          ).map(([k, label]) => (
            <FormField key={k} label={label} required error={errors[k]}>
              <Input inputMode="decimal" value={perJob[k]} onChange={(e) => setPerJob((p) => ({ ...p, [k]: e.target.value }))} disabled={!canManage} />
            </FormField>
          ))}
        </div>
      </section>

      <div className="flex items-center justify-between gap-3">
        <span className="text-small text-neutral-500">Cập nhật lần cuối {formatDateTime(settings.updatedAt)}</span>
        {canManage ? (
          <Button type="submit" variant="primary" disabled={update.isPending}>
            Lưu cài đặt ngân sách
          </Button>
        ) : (
          <span className="text-small text-neutral-500">Bạn chỉ có quyền xem. Cần quyền quản lý ngân sách để sửa.</span>
        )}
      </div>
    </form>
  );
}
