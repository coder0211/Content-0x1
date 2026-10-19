import { forwardRef, type HTMLAttributes, type TdHTMLAttributes, type ThHTMLAttributes } from "react";
import { cn } from "@/shared/lib/cn";

/** SPEC-05 §3 Table: header neutral-50 caption, rows 40px, hover neutral-100, numeric cols right-aligned. */
export const Table = forwardRef<HTMLTableElement, HTMLAttributes<HTMLTableElement>>(({ className, ...props }, ref) => (
  <div className="w-full overflow-auto rounded-lg border border-neutral-200 bg-neutral-0">
    <table ref={ref} className={cn("w-full caption-bottom border-collapse text-small", className)} {...props} />
  </div>
));
Table.displayName = "Table";

export const TableHeader = forwardRef<HTMLTableSectionElement, HTMLAttributes<HTMLTableSectionElement>>(
  ({ className, ...props }, ref) => <thead ref={ref} className={cn("bg-neutral-50", className)} {...props} />,
);
TableHeader.displayName = "TableHeader";

export const TableBody = forwardRef<HTMLTableSectionElement, HTMLAttributes<HTMLTableSectionElement>>(
  ({ className, ...props }, ref) => <tbody ref={ref} className={cn(className)} {...props} />,
);
TableBody.displayName = "TableBody";

export const TableRow = forwardRef<HTMLTableRowElement, HTMLAttributes<HTMLTableRowElement>>(
  ({ className, ...props }, ref) => (
    <tr
      ref={ref}
      className={cn(
        "group h-10 border-b border-neutral-200 transition-colors duration-150 last:border-b-0 hover:bg-neutral-100 data-[state=selected]:bg-accent-50",
        className,
      )}
      {...props}
    />
  ),
);
TableRow.displayName = "TableRow";

type Align = { align?: "left" | "right" | "center" };
const alignClass = (a: Align["align"]) => (a === "right" ? "text-right tabular" : a === "center" ? "text-center" : "text-left");

export const TableHead = forwardRef<HTMLTableCellElement, ThHTMLAttributes<HTMLTableCellElement> & Align>(
  ({ className, align, ...props }, ref) => (
    <th
      ref={ref}
      className={cn(
        "h-9 border-b border-neutral-200 px-3 text-caption text-neutral-500",
        alignClass(align),
        className,
      )}
      {...props}
    />
  ),
);
TableHead.displayName = "TableHead";

export const TableCell = forwardRef<HTMLTableCellElement, TdHTMLAttributes<HTMLTableCellElement> & Align>(
  ({ className, align, ...props }, ref) => (
    <td ref={ref} className={cn("px-3 py-2 align-middle text-neutral-700", alignClass(align), className)} {...props} />
  ),
);
TableCell.displayName = "TableCell";
