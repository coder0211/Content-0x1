import { Megaphone } from "lucide-react";
import { EmptyState, PageHeader } from "@/shared/ui/patterns";

/** Placeholder — replaced by the agent owning modules/campaign. */
export default function CampaignsPage() {
  return (
    <>
      <PageHeader title="Campaign" />
      <EmptyState icon={Megaphone} message="Màn hình này đang được xây dựng." />
    </>
  );
}
