import { useState } from "react";
import { Wallet } from "lucide-react";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { Skeleton } from "@/shared/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { DataTable, EmptyState, PageHeader, type DataTableColumn } from "@/shared/ui/patterns";
import { formatDate, formatNumber, formatUsd } from "@/shared/lib/format";
import { useUsageSummary } from "../../application/useUsageSummary";
import {
  COST_SOURCE_LABELS,
  currentMonthRange,
  GROUP_BY_LABELS,
  GROUP_BYS,
  type GroupBy,
  type UsageRow,
  type UsageTotals,
} from "../../domain/budget";
import { UsageBarChart } from "../components/UsageBarChart";
import { UsageSubnav } from "../components/UsageSubnav";

const TOTAL_KEY = "__total__";

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="flex flex-col gap-0.5 rounded border border-neutral-200 bg-neutral-0 px-4 py-3">
      <span className="text-small text-neutral-500">{label}</span>
      <span className="text-section tabular text-neutral-900">{value}</span>
      {hint ? <span className="text-caption text-neutral-500">{hint}</span> : null}
    </div>
  );
}

export default function UsagePage() {
  const [range, setRange] = useState(currentMonthRange);
  const [groupBy, setGroupBy] = useState<GroupBy>("day");
  const invalidRange = range.to < range.from;
  const q = useUsageSummary({ from: range.from, to: range.to, groupBy }, { enabled: !invalidRange });
  const data = q.data;

  const labelOf = (r: UsageRow) => (groupBy === "day" && r.key ? formatDate(`${r.key}T00:00:00`) : r.label || "—");
  const money = (pick: (t: UsageTotals) => number) => (r: UsageRow) => formatUsd(pick(r));
  const columns: DataTableColumn<UsageRow>[] = [
    {
      id: "label",
      header: GROUP_BY_LABELS[groupBy],
      cell: (r) => (r.key === TOTAL_KEY ? <span className="font-semibold">Tổng</span> : labelOf(r)),
    },
    { id: "provider_reported", header: COST_SOURCE_LABELS.provider_reported, align: "right", cell: money((t) => t.providerReported) },
    { id: "calculated", header: COST_SOURCE_LABELS.calculated, align: "right", cell: money((t) => t.calculated) },
    { id: "estimated", header: COST_SOURCE_LABELS.estimated, align: "right", cell: money((t) => t.estimated) },
    {
      id: "total",
      header: "Tổng chi phí",
      align: "right",
      cell: (r) => <span className={r.key === TOTAL_KEY ? "font-semibold" : "font-medium"}>{formatUsd(r.total)}</span>,
    },
    { id: "count", header: "Lượt gọi", align: "right", cell: (r) => formatNumber(r.count) },
  ];
  const tableRows = data && data.rows.length > 0 ? [...data.rows, { ...data.totals, key: TOTAL_KEY, label: "Tổng" }] : data?.rows;

  return (
    <>
      <PageHeader title="Chi phí & ngân sách" description="Chi phí AI thực tế theo bản ghi usage, tách theo nguồn số liệu." />
      <UsageSubnav />

      <div className="mb-4 flex flex-wrap items-end gap-4">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="usage-from">Từ ngày</Label>
          <Input id="usage-from" type="date" className="w-40" value={range.from} max={range.to} onChange={(e) => e.target.value && setRange((r) => ({ ...r, from: e.target.value }))} />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="usage-to">Đến ngày</Label>
          <Input id="usage-to" type="date" className="w-40" value={range.to} min={range.from} onChange={(e) => e.target.value && setRange((r) => ({ ...r, to: e.target.value }))} />
        </div>
        <Tabs value={groupBy} onValueChange={(v) => setGroupBy(v as GroupBy)} className="ml-auto">
          <TabsList aria-label="Nhóm theo">
            {GROUP_BYS.map((g) => (
              <TabsTrigger key={g} value={g}>
                {GROUP_BY_LABELS[g]}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>

      {invalidRange ? <p className="mb-4 text-small text-danger">Ngày kết thúc phải sau ngày bắt đầu.</p> : null}

      <section aria-label="Tổng quan" className="mb-6 grid grid-cols-4 gap-3">
        {q.isLoading || !data ? (
          Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-20" />)
        ) : (
          <>
            <Stat label="Tổng chi phí" value={formatUsd(data.totals.total)} hint={`${formatNumber(data.totals.count)} lượt gọi provider`} />
            <Stat label={COST_SOURCE_LABELS.provider_reported} value={formatUsd(data.totals.providerReported)} />
            <Stat label={COST_SOURCE_LABELS.calculated} value={formatUsd(data.totals.calculated)} />
            <Stat label={COST_SOURCE_LABELS.estimated} value={formatUsd(data.totals.estimated)} hint={data.totals.estimated > 0 ? "Cần đối soát với hóa đơn nhà cung cấp" : undefined} />
          </>
        )}
      </section>

      {data && data.rows.length > 0 && !q.error ? (
        <section className="mb-6 rounded border border-neutral-200 bg-neutral-0 p-4">
          <UsageBarChart rows={data.rows} groupBy={groupBy} />
        </section>
      ) : null}

      <DataTable
        aria-label="Bảng chi phí"
        columns={columns}
        rows={tableRows}
        getRowId={(r) => r.key ?? "none"}
        isLoading={q.isLoading}
        error={q.error}
        onRetry={() => void q.refetch()}
        empty={<EmptyState icon={Wallet} message="Chưa có chi phí AI trong khoảng thời gian này." />}
      />
    </>
  );
}
