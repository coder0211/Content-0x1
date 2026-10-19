import { cn } from "@/shared/lib/cn";

/** Initials avatar — the only place round shapes are allowed besides status dots. */
export function Avatar({ name, className }: { name: string; className?: string }) {
  const initials = name
    .split(/\s+/)
    .filter(Boolean)
    .slice(-2)
    .map((p) => p[0]?.toUpperCase())
    .join("");
  return (
    <span
      aria-hidden
      className={cn(
        "inline-flex size-7 shrink-0 items-center justify-center rounded-full bg-neutral-200 text-caption text-neutral-700",
        className,
      )}
    >
      {initials}
    </span>
  );
}
