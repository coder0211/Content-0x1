import { PenLine } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/content. */
export default function ContentStudioPage() {
  return (
    <>
      <PageHeader title="Content Studio" />
      <EmptyState icon={PenLine} message="Màn hình này đang được xây dựng." />
    </>
  );
}
