import type { HTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

/** Shape-matching placeholder (SPEC-05 §3 Loading). Keep it the size of the real content. */
export function Skeleton({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div aria-hidden className={cn("animate-pulse rounded bg-neutral-100", className)} {...props} />;
}
