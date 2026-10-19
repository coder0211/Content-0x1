import { Wallet } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/budget. */
export default function UsagePage() {
  return (
    <>
      <PageHeader title="Chi phí & ngân sách" />
      <EmptyState icon={Wallet} message="Màn hình này đang được xây dựng." />
    </>
  );
}
