import { forwardRef, type ButtonHTMLAttributes } from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { Loader2 } from "lucide-react";
import { cn } from "@/shared/lib/cn";

/** SPEC-05 §3: 4 variants. `primary` — at most ONE per area. Labels are verb + object. */
export const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        primary: "bg-accent-600 text-white hover:bg-accent-700 active:bg-accent-700",
        secondary: "border border-neutral-200 bg-neutral-0 text-neutral-900 hover:border-neutral-300 hover:bg-neutral-50",
        ghost: "text-neutral-700 hover:bg-neutral-100 hover:text-neutral-900",
        danger: "bg-danger text-white hover:bg-danger/90",
      },
      size: {
        sm: "h-7 px-2.5 text-small",
        md: "h-8 px-3 text-body",
        lg: "h-9 px-4 text-body",
        icon: "size-8",
        "icon-sm": "size-7",
      },
    },
    defaultVariants: { variant: "secondary", size: "md" },
  },
);

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> {
  asChild?: boolean;
  /** Shows a small inline spinner and disables the button. */
  loading?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, loading = false, disabled, children, ...props }, ref) => {
    const classes = cn(buttonVariants({ variant, size }), className);
    if (asChild) {
      // Radix Slot requires exactly one child element — never render the spinner slot here.
      return (
        <Slot ref={ref} className={classes} aria-busy={loading || undefined} {...props}>
          {children}
        </Slot>
      );
    }
    return (
      <button ref={ref} className={classes} disabled={disabled || loading} aria-busy={loading || undefined} {...props}>
        {loading ? <Loader2 className="animate-spin" aria-hidden /> : null}
        {children}
      </button>
    );
  },
);
Button.displayName = "Button";
