import { formatUsd } from "@/shared/lib/format";
import { cn } from "@/shared/lib/cn";
import { Tooltip } from "../tooltip";

export interface CostChipProps {
  amount: number;
  /** `true` → "≈ $0.42" (dự toán); `false` → "$0.40" (chi phí thực tế). */
  estimate: boolean;
  /** Tooltip text; defaults to the cost source. E.g. "Ước tính theo bảng giá v3". */
  source?: string;
  className?: string;
}

/** Cost chip shown next to AI action buttons, in tables and media cards (SPEC-05 §3). */
export function CostChip({ amount, estimate, source, className }: CostChipProps) {
  const tip = source ?? (estimate ? "Dự toán theo bảng giá hiện hành" : "Chi phí thực tế do nhà cung cấp báo");
  return (
    <Tooltip content={tip}>
      <span
        tabIndex={0}
        aria-label={`${estimate ? "Dự toán" : "Chi phí thực tế"} ${formatUsd(amount)}`}
        className={cn(
          "inline-flex h-5 items-center rounded-sm border border-neutral-200 bg-neutral-50 px-1.5 text-caption tabular text-neutral-700",
          className,
        )}
      >
        {formatUsd(amount, { estimate })}
      </span>
    </Tooltip>
  );
}
