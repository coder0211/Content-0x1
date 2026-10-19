import { forwardRef, type InputHTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

export const controlClass =
  "w-full rounded border border-neutral-200 bg-neutral-0 px-3 text-body text-neutral-900 transition-colors duration-150 placeholder:text-neutral-400 hover:border-neutral-300 focus-visible:border-accent-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/20 disabled:cursor-not-allowed disabled:bg-neutral-100 disabled:opacity-70 aria-[invalid=true]:border-danger";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  /** `md` 32px default, `sm` 28px for toolbars/tables, `lg` 36px for primary forms. */
  inputSize?: "sm" | "md" | "lg";
}

export const Input = forwardRef<HTMLInputElement, InputProps>(({ className, inputSize = "md", ...props }, ref) => (
  <input
    ref={ref}
    className={cn(controlClass, inputSize === "sm" ? "h-7" : inputSize === "lg" ? "h-9" : "h-8", className)}
    {...props}
  />
));
Input.displayName = "Input";
