import { useMemo } from "react";
import { cn } from "@/shared/lib/cn";
import { diffStats, diffWords } from "../../domain/diff";

/** Inline word diff: removed text struck through (danger), added text highlighted (success). */
export function TextDiff({ before, after, className }: { before: string; after: string; className?: string }) {
  const segments = useMemo(() => diffWords(before, after), [before, after]);
  const stats = diffStats(segments);
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <p className="text-small text-neutral-500">
        {stats.added === 0 && stats.removed === 0 ? "Nội dung không đổi so với phiên bản trước." : `+${stats.added} từ, −${stats.removed} từ so với phiên bản trước.`}
      </p>
      <p className="whitespace-pre-line text-editor text-neutral-900" aria-label="So sánh với phiên bản trước">
        {segments.map((s, i) =>
          s.kind === "same" ? (
            <span key={i}>{s.text}</span>
          ) : s.kind === "added" ? (
            <ins key={i} className="rounded-sm bg-success-bg text-success no-underline">
              {s.text}
            </ins>
          ) : (
            <del key={i} className="rounded-sm bg-danger-bg text-danger">
              {s.text}
            </del>
          ),
        )}
      </p>
    </div>
  );
}
