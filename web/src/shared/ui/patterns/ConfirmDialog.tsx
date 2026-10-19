import { useState, type ReactNode } from "react";
import * as AlertDialog from "@radix-ui/react-alert-dialog";
import { cn } from "@/shared/lib/cn";
import { Button } from "../button";
import { overlayClass } from "../dialog";

export interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: ReactNode;
  /** Verb + object: "Xóa asset", "Lưu thay đổi". Never "OK". */
  confirmLabel: string;
  cancelLabel?: string;
  tone?: "primary" | "danger";
  /** May return a promise; dialog stays open (button loading) until it resolves, and stays open on error. */
  onConfirm: () => unknown;
  confirmDisabled?: boolean;
  /** Extra content, e.g. a required comment field. */
  children?: ReactNode;
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  cancelLabel = "Hủy",
  tone = "primary",
  onConfirm,
  confirmDisabled,
  children,
}: ConfirmDialogProps) {
  const [pending, setPending] = useState(false);

  const handleConfirm = async () => {
    setPending(true);
    try {
      await onConfirm();
      onOpenChange(false);
    } catch {
      // Error surfaced by the caller (mutation toast); keep the dialog open for another try.
    } finally {
      setPending(false);
    }
  };

  return (
    <AlertDialog.Root open={open} onOpenChange={(o) => !pending && onOpenChange(o)}>
      <AlertDialog.Portal>
        <AlertDialog.Overlay className={overlayClass} />
        <AlertDialog.Content
          className={cn(
            "fixed left-1/2 top-1/2 z-50 grid w-full max-w-md -translate-x-1/2 -translate-y-1/2 gap-4 rounded-lg border border-neutral-200 bg-neutral-0 p-6 shadow-md duration-200 data-[state=open]:animate-in data-[state=open]:fade-in-0",
          )}
        >
          <div className="flex flex-col gap-1">
            <AlertDialog.Title className="text-section text-neutral-900">{title}</AlertDialog.Title>
            {description ? (
              <AlertDialog.Description className="text-body text-neutral-500">{description}</AlertDialog.Description>
            ) : (
              <AlertDialog.Description className="sr-only">{title}</AlertDialog.Description>
            )}
          </div>
          {children}
          <div className="flex justify-end gap-2">
            <AlertDialog.Cancel asChild>
              <Button variant="secondary" disabled={pending}>
                {cancelLabel}
              </Button>
            </AlertDialog.Cancel>
            <Button
              variant={tone === "danger" ? "danger" : "primary"}
              onClick={() => void handleConfirm()}
              loading={pending}
              disabled={confirmDisabled}
            >
              {confirmLabel}
            </Button>
          </div>
        </AlertDialog.Content>
      </AlertDialog.Portal>
    </AlertDialog.Root>
  );
}
