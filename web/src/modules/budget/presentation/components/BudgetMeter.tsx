import { cn } from "@/shared/lib/cn";
import { formatUsd } from "@/shared/lib/format";
import { budgetTone, type Budget } from "../../domain/budget";

const TONE_BAR = { ok: "bg-accent-600", warning: "bg-warning", danger: "bg-danger" } as const;
const TONE_TEXT = { ok: "text-neutral-700", warning: "text-warning", danger: "text-danger" } as const;

/** Spent + reserved vs limit with threshold ticks. */
export function BudgetMeter({ budget, className }: { budget: Budget; className?: string }) {
  const tone = budgetTone(budget.percentUsed);
  const limit = budget.limitAmount;
  const spentPct = limit > 0 ? Math.min(100, (budget.spentAmount / limit) * 100) : 0;
  const reservedPct = limit > 0 ? Math.min(100 - spentPct, (budget.reservedAmount / limit) * 100) : 0;
  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <div className="flex items-baseline justify-between gap-3 text-small">
        <span className="text-neutral-600">
          Đã chi <span className="tabular text-neutral-900">{formatUsd(budget.spentAmount)}</span> · Đang giữ{" "}
          <span className="tabular text-neutral-900">{formatUsd(budget.reservedAmount)}</span> / {formatUsd(limit)}
        </span>
        <span className={cn("tabular font-medium", TONE_TEXT[tone])}>{Math.round(budget.percentUsed)}%</span>
      </div>
      <div
        role="meter"
        aria-label="Mức sử dụng ngân sách"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(Math.min(100, budget.percentUsed))}
        className="relative h-2 overflow-hidden rounded-sm bg-neutral-100"
      >
        <div className={cn("absolute inset-y-0 left-0", TONE_BAR[tone])} style={{ width: `${spentPct}%` }} />
        <div className={cn("absolute inset-y-0 opacity-40", TONE_BAR[tone])} style={{ left: `${spentPct}%`, width: `${reservedPct}%` }} />
        {budget.alertThresholds
          .filter((t) => t < 100)
          .map((t) => (
            <div key={t} aria-hidden className="absolute inset-y-0 w-px bg-neutral-0" style={{ left: `${t}%` }} />
          ))}
      </div>
    </div>
  );
}
