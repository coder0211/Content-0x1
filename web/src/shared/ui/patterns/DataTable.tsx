import type { ReactNode } from "react";
import { MoreHorizontal, type LucideIcon } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import { Button } from "../button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "../dropdown-menu";
import { Skeleton } from "../skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "../table";
import { ErrorState } from "./ErrorState";

export interface DataTableColumn<T> {
  id: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  /** Numeric columns: "right" (also applies tabular-nums). */
  align?: "left" | "right" | "center";
  width?: number | string;
  className?: string;
}

export interface RowAction<T> {
  label: string;
  icon?: LucideIcon;
  onSelect: (row: T) => void;
  tone?: "default" | "danger";
  disabled?: boolean;
}

export interface DataTableProps<T> {
  columns: DataTableColumn<T>[];
  rows: T[] | undefined;
  getRowId: (row: T) => string;
  isLoading?: boolean;
  error?: unknown;
  onRetry?: () => void;
  /** Rendered when `rows` is empty — use <EmptyState/>. */
  empty?: ReactNode;
  onRowClick?: (row: T) => void;
  /** Hover-revealed "…" menu (SPEC-05 §3 Table). */
  rowActions?: (row: T) => RowAction<T>[];
  selectedId?: string | null;
  skeletonRows?: number;
  /** e.g. "Tải thêm" button for cursor pagination. */
  footer?: ReactNode;
  "aria-label"?: string;
}

/** Standard list table: loading skeleton, inline error, empty state, row click, row actions. */
export function DataTable<T>({
  columns,
  rows,
  getRowId,
  isLoading,
  error,
  onRetry,
  empty,
  onRowClick,
  rowActions,
  selectedId,
  skeletonRows = 5,
  footer,
  "aria-label": ariaLabel,
}: DataTableProps<T>) {
  if (error) return <ErrorState error={error} onRetry={onRetry} />;
  if (!isLoading && rows && rows.length === 0 && empty) return <>{empty}</>;

  return (
    <div className="flex flex-col gap-3">
      <Table aria-label={ariaLabel} aria-busy={isLoading || undefined}>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            {columns.map((c) => (
              <TableHead key={c.id} align={c.align} style={c.width ? { width: c.width } : undefined}>
                {c.header}
              </TableHead>
            ))}
            {rowActions ? <TableHead className="w-10" aria-label="Thao tác" /> : null}
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading || !rows
            ? Array.from({ length: skeletonRows }, (_, i) => (
                <TableRow key={`sk-${i}`} className="hover:bg-transparent">
                  {columns.map((c) => (
                    <TableCell key={c.id} align={c.align}>
                      <Skeleton className={cn("h-4", c.align === "right" ? "ml-auto w-12" : "w-3/4")} />
                    </TableCell>
                  ))}
                  {rowActions ? <TableCell /> : null}
                </TableRow>
              ))
            : rows.map((row) => {
                const id = getRowId(row);
                const actions = rowActions?.(row) ?? [];
                return (
                  <TableRow
                    key={id}
                    data-state={selectedId === id ? "selected" : undefined}
                    className={cn(onRowClick && "cursor-pointer")}
                    tabIndex={onRowClick ? 0 : undefined}
                    onClick={onRowClick ? () => onRowClick(row) : undefined}
                    onKeyDown={
                      onRowClick
                        ? (e) => {
                            if (e.key === "Enter") onRowClick(row);
                          }
                        : undefined
                    }
                  >
                    {columns.map((c) => (
                      <TableCell key={c.id} align={c.align} className={c.className}>
                        {c.cell(row)}
                      </TableCell>
                    ))}
                    {rowActions ? (
                      <TableCell className="w-10 px-1" onClick={(e) => e.stopPropagation()}>
                        {actions.length > 0 ? (
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button
                                variant="ghost"
                                size="icon-sm"
                                aria-label="Thao tác"
                                className="opacity-0 focus-visible:opacity-100 group-hover:opacity-100 data-[state=open]:opacity-100"
                              >
                                <MoreHorizontal />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end">
                              {actions.map((a) => (
                                <DropdownMenuItem key={a.label} tone={a.tone} disabled={a.disabled} onSelect={() => a.onSelect(row)}>
                                  {a.icon ? <a.icon /> : null}
                                  {a.label}
                                </DropdownMenuItem>
                              ))}
                            </DropdownMenuContent>
                          </DropdownMenu>
                        ) : null}
                      </TableCell>
                    ) : null}
                  </TableRow>
                );
              })}
        </TableBody>
      </Table>
      {footer}
    </div>
  );
}
