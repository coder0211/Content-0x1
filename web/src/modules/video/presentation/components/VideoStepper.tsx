import { Check } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import type { VideoStage } from "../../domain/video";

const STEPS = ["Kịch bản", "Storyboard", "Sản xuất"] as const;

function current(stage: VideoStage | null): number {
  switch (stage) {
    case null:
      return 0;
    case "storyboarding":
      return 0;
    case "editing":
      return 1;
    default:
      return 2;
  }
}

/** Stepper 3 bước: Kịch bản → Storyboard → Sản xuất (SPEC-05 §4.3). */
export function VideoStepper({ stage }: { stage: VideoStage | null }) {
  const idx = current(stage);
  return (
    <ol aria-label="Các bước tạo video" className="flex items-center gap-2">
      {STEPS.map((label, i) => {
        const done = i < idx || (i === 2 && stage === "ready");
        const active = i === idx && !done;
        return (
          <li key={label} className="flex items-center gap-2" aria-current={active ? "step" : undefined}>
            <span
              className={cn(
                "inline-flex size-5 items-center justify-center rounded-sm border text-caption tabular",
                done ? "border-success-border bg-success-bg text-success" : active ? "border-accent-600 bg-accent-50 text-accent-700" : "border-neutral-200 text-neutral-500",
              )}
            >
              {done ? <Check className="size-3" aria-hidden /> : i + 1}
            </span>
            <span className={cn("text-small", active ? "font-medium text-neutral-900" : "text-neutral-500")}>{label}</span>
            {i < STEPS.length - 1 ? <span aria-hidden className="h-px w-6 bg-neutral-200" /> : null}
          </li>
        );
      })}
    </ol>
  );
}
