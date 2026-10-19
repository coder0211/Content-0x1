import { Clapperboard } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/video. */
export default function VideoStudioPage() {
  return (
    <>
      <PageHeader title="Video Studio" />
      <EmptyState icon={Clapperboard} message="Màn hình này đang được xây dựng." />
    </>
  );
}
