import { CheckCheck } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/review. */
export default function ReviewQueuePage() {
  return (
    <>
      <PageHeader title="Hàng chờ duyệt" />
      <EmptyState icon={CheckCheck} message="Màn hình này đang được xây dựng." />
    </>
  );
}
