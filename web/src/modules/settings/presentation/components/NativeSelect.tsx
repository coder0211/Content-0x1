import { forwardRef, type SelectHTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";
import { controlClass } from "@/shared/ui/input";

/** Compact native select for filter bars (keyboard/AT friendly; token-styled like Input). */
export const NativeSelect = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(({ className, ...props }, ref) => (
  <select ref={ref} className={cn(controlClass, "h-8 pr-8", className)} {...props} />
));
NativeSelect.displayName = "NativeSelect";
