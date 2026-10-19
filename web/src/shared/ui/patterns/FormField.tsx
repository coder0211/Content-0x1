import { useId, type ReactElement, type ReactNode } from "react";
import { Slot } from "@radix-ui/react-slot";
import { cn } from "@/shared/lib/cn";
import { Label } from "../label";

export interface FormFieldProps {
  label: ReactNode;
  /** Single form control; receives `id`, `aria-invalid`, `aria-describedby`. */
  children: ReactElement;
  required?: boolean;
  helper?: ReactNode;
  error?: string | null;
  /** Server error code/issue shown next to the message, e.g. `required`. */
  errorCode?: string | null;
  id?: string;
  className?: string;
}

/** Label on top, `*` when required, helper below, error right under the field (SPEC-05 §3 Form). */
export function FormField({ label, children, required, helper, error, errorCode, id, className }: FormFieldProps) {
  const autoId = useId();
  const fieldId = id ?? autoId;
  const helperId = `${fieldId}-helper`;
  const errorId = `${fieldId}-error`;
  const describedBy = [helper ? helperId : null, error ? errorId : null].filter(Boolean).join(" ") || undefined;

  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <Label htmlFor={fieldId}>
        {label}
        {required ? (
          <span className="ml-0.5 text-danger" aria-hidden>
            *
          </span>
        ) : null}
      </Label>
      <Slot id={fieldId} aria-invalid={error ? true : undefined} aria-required={required || undefined} aria-describedby={describedBy}>
        {children}
      </Slot>
      {helper && !error ? (
        <p id={helperId} className="text-small text-neutral-500">
          {helper}
        </p>
      ) : null}
      {error ? (
        <p id={errorId} role="alert" className="text-small text-danger">
          {error}
          {errorCode ? <span className="ml-1 font-mono text-caption font-normal">({errorCode})</span> : null}
        </p>
      ) : null}
    </div>
  );
}
