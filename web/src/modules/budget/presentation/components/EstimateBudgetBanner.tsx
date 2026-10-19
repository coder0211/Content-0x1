import { Banner } from "@/shared/ui/banner";
import { formatUsd } from "@/shared/lib/format";
import type { Estimate } from "../../domain/budget";

/**
 * Over-budget banner for a studio's generate area (SPEC-05 §5 "Vượt ngân sách").
 * Renders nothing when the estimate fits.
 */
export function EstimateBudgetBanner({ estimate, className }: { estimate: Estimate | undefined; className?: string }) {
  if (!estimate) return null;
  if (estimate.blocked) {
    return (
      <Banner tone="danger" title="Không đủ ngân sách" className={className}>
        Dự toán {formatUsd(estimate.total, { estimate: true })} vượt ngân sách còn lại. Liên hệ Admin để tăng hạn mức.
      </Banner>
    );
  }
  if (estimate.requiresOverride) {
    return (
      <Banner tone="warning" className={className}>
        Dự toán {formatUsd(estimate.total)} vượt hạn mức mỗi job {formatUsd(estimate.limits.perJobLimit)}. Giảm khối lượng hoặc xin Marketing Lead duyệt.
      </Banner>
    );
  }
  return null;
}
