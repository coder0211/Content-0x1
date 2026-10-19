import { useState, type ReactNode } from "react";
import { Image, Megaphone, PenLine, RefreshCw, Trash2 } from "lucide-react";
import type { StatusKind } from "@/shared/kernel";
import { ApiError } from "@/shared/kernel";
import { formatDate, formatDuration, formatFileSize, formatUsd, formatVnd } from "@/shared/lib/format";
import { Badge } from "@/shared/ui/badge";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import {
  AIProvenance,
  ConfirmDialog,
  CostChip,
  DataTable,
  EmptyState,
  ErrorState,
  FormField,
  JobProgress,
  NoPermission,
  PageHeader,
  STATUS_MAP,
  StatusBadge,
  type DataTableColumn,
} from "@/shared/ui/patterns";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Sheet, SheetBody, SheetContent, SheetFooter, SheetHeader, SheetTitle, SheetTrigger } from "@/shared/ui/sheet";
import { Skeleton } from "@/shared/ui/skeleton";
import { Switch } from "@/shared/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { Textarea } from "@/shared/ui/textarea";
import { toast } from "@/shared/ui/toast";
import { fontSize, palette, radius, shadow } from "@/shared/ui/tokens";

const TEXT_CLASS: Record<keyof typeof fontSize, string> = {
  page: "text-page",
  section: "text-section",
  body: "text-body",
  small: "text-small",
  caption: "text-caption",
  editor: "text-editor",
};

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="border-t border-neutral-200 py-6">
      <h2 className="mb-4 text-section text-neutral-900">{title}</h2>
      {children}
    </section>
  );
}

function Swatch({ name, hex }: { name: string; hex: string }) {
  return (
    <div className="flex w-28 flex-col gap-1">
      <div className="h-10 rounded border border-neutral-200" style={{ background: hex }} />
      <span className="text-caption text-neutral-900">{name}</span>
      <span className="font-mono text-caption font-normal text-neutral-500">{hex}</span>
    </div>
  );
}

interface DemoRow {
  id: string;
  name: string;
  status: "draft" | "pending_review" | "approved";
  cost: number;
}

const ROWS: DemoRow[] = [
  { id: "1", name: "Khóa giao tiếp T11 — bài Facebook A", status: "approved", cost: 0.02 },
  { id: "2", name: "Poster ưu đãi 20%", status: "pending_review", cost: 0.32 },
  { id: "3", name: "Video TikTok 20s", status: "draft", cost: 3.2 },
];

const COLUMNS: DataTableColumn<DemoRow>[] = [
  { id: "name", header: "Tên", cell: (r) => <span className="font-medium text-neutral-900">{r.name}</span> },
  { id: "status", header: "Trạng thái", cell: (r) => <StatusBadge kind="asset" status={r.status} /> },
  { id: "cost", header: "Chi phí", align: "right", cell: (r) => formatUsd(r.cost) },
];

/** Living style guide for PO review (ACS-004). Public route: /_design. */
export default function DesignPage() {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const sampleError = new ApiError({ code: "SERVICE_UNAVAILABLE", message: "x", status: 503, requestId: "9f1c2a7e-0b1d-4c55-9d1e-3c7f5b2e8a10" });

  return (
    <div className="mx-auto max-w-[1200px] p-6">
      <PageHeader title="Design system" description="Token và component theo SPEC-05. Mọi màn hình chỉ dùng những gì có ở trang này." />

      <Section title="Màu">
        {Object.entries(palette).map(([group, values]) => (
          <div key={group} className="mb-4">
            <p className="mb-2 text-caption text-neutral-500">{group}</p>
            <div className="flex flex-wrap gap-3">
              {Object.entries(values).map(([k, hex]) => (
                <Swatch key={k} name={`${group}-${k}`} hex={hex} />
              ))}
            </div>
          </div>
        ))}
      </Section>

      <Section title="Chữ">
        <div className="flex flex-col gap-3">
          {Object.entries(fontSize).map(([k, [size, { lineHeight }]]) => (
            <div key={k} className="flex items-baseline gap-6">
              <span className="w-40 font-mono text-caption font-normal text-neutral-500">
                text-{k} · {size}/{lineHeight}
              </span>
              <span className={TEXT_CLASS[k as keyof typeof fontSize]}>Tự tin giao tiếp tiếng Anh sau 12 tuần</span>
            </div>
          ))}
          <div className="flex items-baseline gap-6">
            <span className="w-40 font-mono text-caption font-normal text-neutral-500">số liệu (tabular)</span>
            <span className="tabular">
              {formatUsd(0.42)} · {formatUsd(0.004)} · {formatVnd(2990000)} · {formatDate(new Date(2026, 9, 16))} · {formatDuration(20)} ·{" "}
              {formatFileSize(13_002_342)}
            </span>
          </div>
          <div className="flex items-baseline gap-6">
            <span className="w-40 font-mono text-caption font-normal text-neutral-500">font-mono</span>
            <span className="font-mono text-small">Poster quảng bá khóa học, phong cách trẻ trung</span>
          </div>
        </div>
      </Section>

      <Section title="Bo góc và bóng đổ">
        <div className="flex flex-wrap gap-6">
          {Object.entries(radius).map(([k, v]) => (
            <div key={k} className="flex flex-col items-center gap-2">
              <div className="size-16 border border-neutral-300 bg-neutral-0" style={{ borderRadius: v }} />
              <span className="text-caption text-neutral-500">radius-{k} {v}</span>
            </div>
          ))}
          {Object.entries(shadow).map(([k, v]) => (
            <div key={k} className="flex flex-col items-center gap-2">
              <div className="size-16 rounded-lg border border-neutral-200 bg-neutral-0" style={{ boxShadow: v }} />
              <span className="text-caption text-neutral-500">shadow-{k}</span>
            </div>
          ))}
        </div>
      </Section>

      <Section title="Nút">
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="primary">
            <PenLine />
            Viết 3 phương án
          </Button>
          <Button variant="secondary">Lưu phiên bản</Button>
          <Button variant="ghost">
            <RefreshCw />
            Viết lại
          </Button>
          <Button variant="danger">
            <Trash2 />
            Xóa asset
          </Button>
          <Button variant="primary" loading>
            Đang gửi duyệt
          </Button>
          <Button variant="secondary" disabled>
            Tải xuống
          </Button>
          <Button size="sm">Nhỏ (28px)</Button>
          <Button size="lg">Lớn (36px)</Button>
          <span className="flex items-center gap-2">
            <Button variant="primary">
              <Image />
              Tạo 4 ảnh
            </Button>
            <CostChip amount={0.32} estimate source="Ước tính theo bảng giá v3" />
          </span>
        </div>
      </Section>

      <Section title="Form">
        <div className="grid max-w-3xl grid-cols-2 gap-4">
          <FormField label="Tên campaign" required helper="Tối đa 120 ký tự.">
            <Input placeholder="Khóa giao tiếp T11" />
          </FormField>
          <FormField label="Đối tượng" required error="Trường này là bắt buộc." errorCode="required">
            <Input />
          </FormField>
          <FormField label="Nền tảng">
            <Select defaultValue="facebook">
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="facebook">Facebook</SelectItem>
                <SelectItem value="tiktok">TikTok</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label="Ghi chú">
            <Textarea placeholder="Yêu cầu thêm cho bài viết" />
          </FormField>
          <label className="flex items-center gap-2 text-body">
            <Checkbox defaultChecked /> Đã kiểm tra chữ trên poster
          </label>
          <label className="flex items-center gap-2 text-body">
            <Switch defaultChecked /> Bật phụ đề
          </label>
        </div>
      </Section>

      <Section title="Trạng thái">
        <div className="flex flex-col gap-3">
          {(Object.keys(STATUS_MAP) as StatusKind[]).map((kind) => (
            <div key={kind} className="flex flex-wrap items-center gap-2">
              <span className="w-20 text-caption text-neutral-500">{kind}</span>
              {Object.keys(STATUS_MAP[kind]).map((s) => (
                <StatusBadge key={s} kind={kind} status={s as never} />
              ))}
            </div>
          ))}
          <div className="flex items-center gap-2">
            <span className="w-20 text-caption text-neutral-500">badge</span>
            <Badge>12</Badge>
            <Badge tone="info">Mới</Badge>
            <CostChip amount={0.4} estimate={false} />
            <CostChip amount={0.004} estimate />
          </div>
        </div>
      </Section>

      <Section title="Tiến trình job">
        <div className="grid max-w-xl gap-6">
          <JobProgress status="running" progress={60} label="Đang sinh cảnh 3/5" etaSeconds={120} onCancel={() => undefined} />
          <JobProgress status="queued" onCancel={() => undefined} />
          <JobProgress status="failed" errorKind="ContentPolicy" onRetry={() => undefined} />
          <JobProgress status="succeeded" label="Đã tạo 3 phương án" />
          <AIProvenance model="claude-sonnet-5-5" brandVersion={3} createdAt={new Date(Date.now() - 5 * 60_000)} />
        </div>
      </Section>

      <Section title="Bảng">
        <DataTable
          aria-label="Bảng mẫu"
          columns={COLUMNS}
          rows={ROWS}
          getRowId={(r) => r.id}
          rowActions={() => [
            { label: "Mở", onSelect: () => undefined },
            { label: "Xóa", tone: "danger", icon: Trash2, onSelect: () => undefined },
          ]}
        />
        <div className="mt-4">
          <DataTable aria-label="Đang tải" columns={COLUMNS} rows={undefined} isLoading skeletonRows={2} getRowId={(r) => r.id} />
        </div>
      </Section>

      <Section title="Trạng thái màn hình (SPEC-05 §5)">
        <div className="grid gap-4">
          <div className="flex flex-col gap-2">
            <Skeleton className="h-5 w-64" />
            <Skeleton className="h-4 w-96" />
          </div>
          <EmptyState icon={Megaphone} message="Chưa có campaign nào." action={<Button variant="primary">Tạo campaign</Button>} />
          <ErrorState error={sampleError} onRetry={() => undefined} />
          <NoPermission className="rounded-lg border border-neutral-200" />
          <Banner tone="warning" title="Dự toán $3.20 vượt hạn mức mỗi job $2.00.">
            Giảm số cảnh hoặc xin Marketing Lead duyệt.
          </Banner>
        </div>
      </Section>

      <Section title="Tabs, dialog, drawer, toast">
        <Tabs defaultValue="a">
          <TabsList>
            <TabsTrigger value="a">Phương án A</TabsTrigger>
            <TabsTrigger value="b">Phương án B</TabsTrigger>
            <TabsTrigger value="c">Phương án C</TabsTrigger>
          </TabsList>
          <TabsContent value="a">
            <p className="max-w-editor text-editor">Bạn ngại nói tiếng Anh vì sợ sai? 12 tuần luyện cùng giáo viên bản ngữ…</p>
          </TabsContent>
          <TabsContent value="b">Phương án B</TabsContent>
          <TabsContent value="c">Phương án C</TabsContent>
        </Tabs>
        <div className="mt-4 flex flex-wrap gap-3">
          <Dialog>
            <DialogTrigger asChild>
              <Button>Mở dialog</Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Đổi tên asset</DialogTitle>
                <DialogDescription>Tên mới hiển thị trong thư viện.</DialogDescription>
              </DialogHeader>
              <Input defaultValue="Poster ưu đãi 20%" />
              <DialogFooter>
                <Button variant="primary">Lưu tên</Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
          <Sheet>
            <SheetTrigger asChild>
              <Button>Mở drawer</Button>
            </SheetTrigger>
            <SheetContent>
              <SheetHeader>
                <SheetTitle>Sửa brief</SheetTitle>
              </SheetHeader>
              <SheetBody>
                <FormField label="Thông điệp chính">
                  <Textarea />
                </FormField>
              </SheetBody>
              <SheetFooter>
                <Button variant="primary">Lưu brief</Button>
              </SheetFooter>
            </SheetContent>
          </Sheet>
          <Button variant="danger" onClick={() => setConfirmOpen(true)}>
            Xóa asset
          </Button>
          <ConfirmDialog
            open={confirmOpen}
            onOpenChange={setConfirmOpen}
            title="Xóa asset này?"
            description="Asset sẽ chuyển vào lưu trữ và được ghi audit."
            confirmLabel="Xóa asset"
            tone="danger"
            onConfirm={() => new Promise((r) => setTimeout(r, 600))}
          />
          <Button onClick={() => toast.success("Đã lưu phiên bản 4", { action: { label: "Xem", onClick: () => undefined } })}>
            Toast thành công
          </Button>
          <Button onClick={() => toast.error(new ApiError({ code: "BUDGET_EXCEEDED", message: "x", status: 429, requestId: "req-123" }))}>
            Toast lỗi
          </Button>
        </div>
      </Section>
    </div>
  );
}
