import { toast as sonner, Toaster as SonnerToaster } from "sonner";
import { describeError } from "@/shared/lib/errors";

/** Mount once in app providers. Bottom-right; success auto-closes after 5s, errors stay (SPEC-05 §3). */
export function Toaster() {
  return (
    <SonnerToaster
      position="bottom-right"
      duration={5000}
      closeButton
      toastOptions={{
        unstyled: true,
        classNames: {
          toast:
            "flex w-[360px] items-start gap-2 rounded-lg border border-neutral-200 bg-neutral-0 p-3 text-body text-neutral-900 shadow-md",
          title: "font-medium",
          description: "text-small text-neutral-500",
          actionButton: "ml-auto shrink-0 text-body font-medium text-accent-600 hover:underline",
          closeButton: "order-last text-neutral-400 hover:text-neutral-900",
          error: "[&_[data-icon]]:text-danger",
          success: "[&_[data-icon]]:text-success",
          warning: "[&_[data-icon]]:text-warning",
          info: "[&_[data-icon]]:text-info",
        },
      }}
    />
  );
}

export interface ToastAction {
  /** Usually "Xem" linking to the job/asset. */
  label: string;
  onClick: () => void;
}

export const toast = {
  success(message: string, opts: { description?: string; action?: ToastAction } = {}) {
    return sonner.success(message, opts);
  },
  info(message: string, opts: { description?: string; action?: ToastAction } = {}) {
    return sonner.info(message, opts);
  },
  warning(message: string, opts: { description?: string; action?: ToastAction } = {}) {
    return sonner.warning(message, { duration: Infinity, ...opts });
  },
  /** Error toast mapped from `ApiError.code`; persistent, shows `request_id` for IT. */
  error(error: unknown, opts: { action?: ToastAction; fallbackMessage?: string } = {}) {
    const d = describeError(error);
    const message = d.code === "UNKNOWN" && opts.fallbackMessage ? opts.fallbackMessage : d.message;
    return sonner.error(message, {
      id: d.requestId ?? undefined,
      duration: Infinity,
      description: d.requestId ? `Mã yêu cầu: ${d.requestId}` : undefined,
      action: opts.action,
    });
  },
  dismiss: sonner.dismiss,
};
