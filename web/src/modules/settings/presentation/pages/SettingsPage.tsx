import { Settings2 } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/settings. */
export default function SettingsPage() {
  return (
    <>
      <PageHeader title="Cài đặt" />
      <EmptyState icon={Settings2} message="Màn hình này đang được xây dựng." />
    </>
  );
}
