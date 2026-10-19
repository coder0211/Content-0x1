import { SwatchBook } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/brand. */
export default function BrandsPage() {
  return (
    <>
      <PageHeader title="Brand" />
      <EmptyState icon={SwatchBook} message="Màn hình này đang được xây dựng." />
    </>
  );
}
