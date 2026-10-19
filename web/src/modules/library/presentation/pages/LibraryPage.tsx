import { Library } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/library. */
export default function LibraryPage() {
  return (
    <>
      <PageHeader title="Thư viện asset" />
      <EmptyState icon={Library} message="Màn hình này đang được xây dựng." />
    </>
  );
}
