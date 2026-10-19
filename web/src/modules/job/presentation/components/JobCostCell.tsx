import { CostChip } from "@/shared/ui/patterns";
import type { JobCost } from "../../domain/job";

/** Actual cost when known, else the estimate (≈). */
export function JobCostCell({ cost }: { cost: JobCost }) {
  if (cost.actual != null) return <CostChip amount={cost.actual} estimate={false} source={cost.source ?? undefined} />;
  if (cost.estimate != null) return <CostChip amount={cost.estimate} estimate />;
  return <span className="text-neutral-400">—</span>;
}
