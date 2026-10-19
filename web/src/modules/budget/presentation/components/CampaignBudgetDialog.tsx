import { useState, type FormEvent } from "react";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { Switch } from "@/shared/ui/switch";
import { toast } from "@/shared/ui/toast";
import { FormField } from "@/shared/ui/patterns";
import { usePutCampaignBudget } from "../../application/useBudgetSettings";
import { parseAmount, parseThresholds, type Budget } from "../../domain/budget";

export interface CampaignBudgetDialogProps {
  campaignId: string;
  campaignName?: string | null;
  /** Existing budget, or null/undefined to create one. */
  budget?: Budget | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Create/edit a campaign budget (budget.manage). Exported for the campaign detail page. */
export function CampaignBudgetDialog({ campaignId, campaignName, budget, open, onOpenChange }: CampaignBudgetDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {open ? <CampaignBudgetForm campaignId={campaignId} campaignName={campaignName} budget={budget} onDone={() => onOpenChange(false)} /> : null}
      </DialogContent>
    </Dialog>
  );
}

function CampaignBudgetForm({ campaignId, campaignName, budget, onDone }: Omit<CampaignBudgetDialogProps, "open" | "onOpenChange"> & { onDone: () => void }) {
  const [limit, setLimit] = useState(budget ? String(budget.limitAmount) : "");
  const [start, setStart] = useState(budget?.periodStart ?? "");
  const [end, setEnd] = useState(budget?.periodEnd ?? "");
  const [thresholds, setThresholds] = useState((budget?.alertThresholds ?? [80, 100]).join(", "));
  const [hardStop, setHardStop] = useState(budget?.hardStop ?? true);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const put = usePutCampaignBudget();

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const errs: Record<string, string> = {};
    const amount = parseAmount(limit);
    if (amount === null) errs.limit = "Nhập số tiền USD hợp lệ.";
    const th = parseThresholds(thresholds);
    if (!th.ok) errs.thresholds = th.error;
    if (start && end && end < start) errs.end = "Ngày kết thúc phải sau ngày bắt đầu.";
    setErrors(errs);
    if (Object.keys(errs).length > 0 || amount === null || !th.ok) return;
    put.mutate(
      { campaignId, input: { limitAmount: amount, periodStart: start || undefined, periodEnd: end || undefined, alertThresholds: th.value, hardStop } },
      {
        onSuccess: () => {
          toast.success("Đã lưu ngân sách campaign");
          onDone();
        },
      },
    );
  };

  return (
    <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
      <DialogHeader>
        <DialogTitle>{budget ? "Sửa ngân sách campaign" : "Đặt ngân sách campaign"}</DialogTitle>
        {campaignName ? <DialogDescription>{campaignName}</DialogDescription> : null}
      </DialogHeader>
      <FormField label="Hạn mức (USD)" required error={errors.limit}>
        <Input inputMode="decimal" value={limit} onChange={(e) => setLimit(e.target.value)} />
      </FormField>
      <div className="grid grid-cols-2 gap-3">
        <FormField label="Từ ngày" helper="Mặc định: hôm nay">
          <Input type="date" value={start} onChange={(e) => setStart(e.target.value)} />
        </FormField>
        <FormField label="Đến ngày" error={errors.end}>
          <Input type="date" value={end} onChange={(e) => setEnd(e.target.value)} />
        </FormField>
      </div>
      <FormField label="Ngưỡng cảnh báo (%)" helper="Cách nhau bằng dấu phẩy, ví dụ 80, 100" error={errors.thresholds}>
        <Input value={thresholds} onChange={(e) => setThresholds(e.target.value)} />
      </FormField>
      <label className="flex items-center gap-2 text-body text-neutral-700">
        <Switch checked={hardStop} onCheckedChange={setHardStop} aria-label="Chặn job mới khi hết ngân sách campaign" />
        Chặn job mới khi hết ngân sách
      </label>
      <DialogFooter>
        <Button type="button" variant="secondary" onClick={onDone}>
          Hủy
        </Button>
        <Button type="submit" variant="primary" disabled={put.isPending}>
          Lưu ngân sách
        </Button>
      </DialogFooter>
    </form>
  );
}
