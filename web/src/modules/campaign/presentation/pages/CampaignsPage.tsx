import { useState } from "react";
import { useNavigate } from "react-router";
import { Megaphone, Plus } from "lucide-react";
import { useBrandOptions } from "@/modules/brand";
import type { CampaignStatus } from "@/shared/kernel";
import { formatDate } from "@/shared/lib/format";
import { Can, useCan } from "@/shared/lib/session";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { controlClass, Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { DataTable, EmptyState, PageHeader, StatusBadge, type DataTableColumn } from "@/shared/ui/patterns";
import { Tabs, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { useCampaigns } from "../../application/useCampaigns";
import { formatLabel, GOAL_LABELS } from "../../domain/rules";
import type { Campaign } from "../../domain/types";

const STATUS_TABS: { value: CampaignStatus | "all"; label: string }[] = [
  { value: "all", label: "Tất cả" },
  { value: "draft", label: "Nháp" },
  { value: "in_progress", label: "Đang thực hiện" },
  { value: "review", label: "Đang duyệt" },
  { value: "completed", label: "Hoàn thành" },
  { value: "archived", label: "Đã lưu trữ" },
];

export default function CampaignsPage() {
  const [status, setStatus] = useState<CampaignStatus | "all">("all");
  const [brandId, setBrandId] = useState("");
  const [q, setQ] = useState("");
  const [mine, setMine] = useState(false);
  const brands = useBrandOptions();
  const campaigns = useCampaigns({ status: status === "all" ? undefined : status, brandId: brandId || undefined, q: q.trim() || undefined, mine: mine || undefined });
  const canManage = useCan("campaign.manage");
  const navigate = useNavigate();
  const brandName = (id: string) => brands.options.find((o) => o.value === id)?.label ?? "—";
  const filtered = status !== "all" || !!brandId || !!q.trim() || mine;

  const columns: DataTableColumn<Campaign>[] = [
    { id: "name", header: "Campaign", cell: (c) => <span className="font-medium text-neutral-900">{c.name}</span> },
    { id: "brand", header: "Brand", cell: (c) => brandName(c.brandId) },
    { id: "goal", header: "Mục tiêu", cell: (c) => GOAL_LABELS[c.goal] ?? c.goal },
    { id: "status", header: "Trạng thái", cell: (c) => <StatusBadge kind="campaign" status={c.status} /> },
    {
      id: "outputs",
      header: "Đầu ra",
      cell: (c) => (
        <span className="text-neutral-700">
          {c.brief.formats.map((f) => `${f.count} ${formatLabel(f.type).toLowerCase()}`).join(", ") || "—"}
        </span>
      ),
    },
    { id: "deadline", header: "Deadline", cell: (c) => (c.brief.deadline ? formatDate(c.brief.deadline) : "—") },
    { id: "updated", header: "Cập nhật", cell: (c) => formatDate(c.updatedAt) },
  ];

  return (
    <div>
      <PageHeader
        title="Campaign"
        description="Mỗi campaign có một brief dùng chung cho nội dung, hình ảnh và video."
        actions={
          <Can permission="campaign.manage">
            <Button variant="primary" onClick={() => void navigate("/campaigns/new")}>
              <Plus /> Tạo campaign
            </Button>
          </Can>
        }
      />
      <Tabs value={status} onValueChange={(v) => setStatus(v as CampaignStatus | "all")}>
        <TabsList aria-label="Lọc theo trạng thái">
          {STATUS_TABS.map((t) => (
            <TabsTrigger key={t.value} value={t.value}>
              {t.label}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      <div className="my-4 flex items-center gap-3">
        <Input className="max-w-xs" type="search" aria-label="Tìm campaign" placeholder="Tìm theo tên" value={q} onChange={(e) => setQ(e.target.value)} />
        <select aria-label="Lọc theo brand" className={`${controlClass} w-56`} value={brandId} onChange={(e) => setBrandId(e.target.value)}>
          <option value="">Tất cả brand</option>
          {brands.options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
        <div className="flex items-center gap-2">
          <Checkbox id="campaign-mine" checked={mine} onCheckedChange={(v) => setMine(v === true)} />
          <Label htmlFor="campaign-mine" className="font-normal">
            Campaign của tôi
          </Label>
        </div>
      </div>
      <DataTable
        aria-label="Danh sách campaign"
        columns={columns}
        rows={campaigns.data?.items}
        getRowId={(c) => c.id}
        isLoading={campaigns.isLoading}
        error={campaigns.error}
        onRetry={() => void campaigns.refetch()}
        onRowClick={(c) => void navigate(`/campaigns/${c.id}`)}
        empty={
          <EmptyState
            icon={Megaphone}
            message={filtered ? "Không có campaign nào khớp bộ lọc." : "Chưa có campaign nào."}
            action={
              canManage && !filtered ? (
                <Button size="sm" onClick={() => void navigate("/campaigns/new")}>
                  Tạo campaign
                </Button>
              ) : undefined
            }
          />
        }
      />
    </div>
  );
}
