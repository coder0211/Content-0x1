import { Activity } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/job. */
export default function JobsPage() {
  return (
    <>
      <PageHeader title="Job" />
      <EmptyState icon={Activity} message="Màn hình này đang được xây dựng." />
    </>
  );
}
