import { cn } from "@/shared/lib/cn";
import { formatDateTime, formatRelative } from "@/shared/lib/format";

export interface AIProvenanceProps {
  model: string;
  brandVersion?: number | null;
  createdAt: Date | string;
  /** Defaults to "Bản nháp AI". */
  label?: string;
  className?: string;
}

/** "Bản nháp AI · {model} · brand v{n} · {thời gian}" — no sparkle icon (SPEC-05 P3). */
export function AIProvenance({ model, brandVersion, createdAt, label = "Bản nháp AI", className }: AIProvenanceProps) {
  const parts = [label, model, brandVersion != null ? `brand v${brandVersion}` : null, formatRelative(createdAt)].filter(Boolean);
  return (
    <p className={cn("text-caption font-normal text-neutral-500", className)} title={formatDateTime(createdAt)}>
      {parts.join(" · ")}
    </p>
  );
}
