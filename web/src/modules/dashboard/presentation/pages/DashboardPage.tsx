import { LayoutDashboard } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/dashboard. */
export default function DashboardPage() {
  return (
    <>
      <PageHeader title="Tổng quan" />
      <EmptyState icon={LayoutDashboard} message="Màn hình này đang được xây dựng." />
    </>
  );
}
