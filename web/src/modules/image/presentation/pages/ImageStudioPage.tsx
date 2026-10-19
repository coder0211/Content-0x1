import { Image } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/image. */
export default function ImageStudioPage() {
  return (
    <>
      <PageHeader title="Image Studio" />
      <EmptyState icon={Image} message="Màn hình này đang được xây dựng." />
    </>
  );
}
