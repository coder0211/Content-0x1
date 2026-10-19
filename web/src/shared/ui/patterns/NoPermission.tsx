import { Lock } from "lucide-react";
import { cn } from "@/shared/lib/cn";

/** Shown instead of a page the user cannot access — never hide the route silently (SPEC-05 §5). */
export function NoPermission({ className }: { className?: string }) {
  return (
    <div className={cn("flex flex-col items-center justify-center gap-3 px-6 py-16 text-center", className)}>
      <Lock className="size-5 text-neutral-400" aria-hidden />
      <p className="text-body text-neutral-700">Bạn không có quyền xem mục này. Liên hệ Admin.</p>
    </div>
  );
}
