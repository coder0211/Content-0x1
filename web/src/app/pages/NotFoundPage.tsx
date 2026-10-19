import { Link } from "react-router";
import { FileQuestion } from "lucide-react";
import { Button } from "@/shared/ui/button";
import { EmptyState } from "@/shared/ui/patterns";

export function NotFoundPage() {
  return (
    <EmptyState
      icon={FileQuestion}
      message="Không tìm thấy trang này."
      action={
        <Button asChild variant="secondary">
          <Link to="/">Về trang tổng quan</Link>
        </Button>
      }
    />
  );
}
