import type { HTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

export function Kbd({ className, ...props }: HTMLAttributes<HTMLElement>) {
  return (
    <kbd
      className={cn("inline-flex h-5 min-w-5 items-center justify-center rounded-sm border border-neutral-200 bg-neutral-50 px-1 font-mono text-caption text-neutral-500", className)}
      {...props}
    />
  );
}
