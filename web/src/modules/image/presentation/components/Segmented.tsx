import type { KeyboardEvent, ReactNode } from "react";
import { cn } from "@/shared/lib/cn";

export interface SegmentedOption<T extends string | number> {
  value: T;
  label: string;
}

/**
 * Compact single-choice control (radiogroup of buttons) for short option lists:
 * preset, aspect ratio, quality, count. Arrow keys move the selection.
 */
export function Segmented<T extends string | number>({
  label,
  value,
  options,
  onChange,
  columns,
  invalid,
  describedBy,
  disabled,
}: {
  label: string;
  value: T;
  options: readonly SegmentedOption<T>[];
  onChange: (v: T) => void;
  columns?: number;
  invalid?: boolean;
  describedBy?: string;
  disabled?: boolean;
}) {
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = options.findIndex((o) => o.value === value);
    const step = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
    if (!step || i < 0) return;
    e.preventDefault();
    const next = options[(i + step + options.length) % options.length]!;
    onChange(next.value);
    const btn = e.currentTarget.querySelectorAll<HTMLButtonElement>("[role=radio]")[(i + step + options.length) % options.length];
    btn?.focus();
  };
  return (
    <div
      role="radiogroup"
      aria-label={label}
      aria-invalid={invalid || undefined}
      aria-describedby={describedBy}
      onKeyDown={onKeyDown}
      className={cn("grid gap-1", invalid && "rounded outline outline-1 outline-danger")}
      style={{ gridTemplateColumns: `repeat(${columns ?? options.length}, minmax(0, 1fr))` }}
    >
      {options.map((o) => {
        const checked = o.value === value;
        return (
          <button
            key={String(o.value)}
            type="button"
            role="radio"
            aria-checked={checked}
            tabIndex={checked ? 0 : -1}
            disabled={disabled}
            onClick={() => onChange(o.value)}
            className={cn(
              "h-8 truncate rounded border px-2 text-small transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50",
              checked
                ? "border-accent-600 bg-accent-50 font-medium text-accent-700"
                : "border-neutral-200 bg-neutral-0 text-neutral-700 hover:bg-neutral-50",
            )}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

/** Label + control + error/helper, for controls that are not a single form element. */
export function Field({
  label,
  required,
  error,
  helper,
  id,
  children,
}: {
  label: string;
  required?: boolean;
  error?: string | null;
  helper?: string;
  id: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <span id={`${id}-label`} className="text-body font-medium text-neutral-900">
        {label}
        {required ? (
          <span className="ml-0.5 text-danger" aria-hidden>
            *
          </span>
        ) : null}
      </span>
      {children}
      {error ? (
        <p id={`${id}-error`} role="alert" className="text-small text-danger">
          {error}
        </p>
      ) : helper ? (
        <p className="text-small text-neutral-500">{helper}</p>
      ) : null}
    </div>
  );
}
