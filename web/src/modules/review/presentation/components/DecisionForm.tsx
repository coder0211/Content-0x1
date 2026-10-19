import { useState } from "react";
import { Check, MessageSquareWarning } from "lucide-react";
import { isApiError } from "@/shared/kernel";
import { describeError } from "@/shared/lib/errors";
import { useReviewDecision, type AssetDetail, type ReviewDecision } from "@/modules/library";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Textarea } from "@/shared/ui/textarea";
import { FormField } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";

/**
 * Comment + "Yêu cầu sửa" / "Duyệt" (ACS-081). The decision happens in place; `onDecided`
 * moves the queue to the next asset.
 */
export function DecisionForm({ asset, onDecided }: { asset: AssetDetail; onDecided: (id: string) => void }) {
  const [comment, setComment] = useState("");
  const [commentError, setCommentError] = useState<string | null>(null);
  const decide = useReviewDecision();

  if (asset.status !== "pending_review") {
    return <Banner tone="info">Asset này không còn chờ duyệt.</Banner>;
  }
  if (!asset.permissions.canReview) {
    return <Banner tone="warning">Bạn không thể tự duyệt asset do chính mình tạo. Một reviewer khác sẽ duyệt asset này.</Banner>;
  }

  const submit = (decision: ReviewDecision) => {
    if (decision === "request_changes" && !comment.trim()) {
      setCommentError("Nhập nhận xét để người tạo biết cần sửa gì.");
      return;
    }
    setCommentError(null);
    decide.mutate(
      { id: asset.id, decision, comment },
      {
        onSuccess: () => {
          toast.success(decision === "approve" ? `Đã duyệt "${asset.title}"` : `Đã yêu cầu sửa "${asset.title}"`);
          setComment("");
          onDecided(asset.id);
        },
      },
    );
  };

  const error = decide.error;
  return (
    <form
      className="flex flex-col gap-3"
      aria-label="Quyết định duyệt"
      onSubmit={(e) => {
        e.preventDefault();
        submit("approve");
      }}
    >
      {error ? (
        <Banner tone="danger">
          {isApiError(error) && error.code === "INVALID_STATE_TRANSITION"
            ? "Asset vừa được người khác xử lý. Hàng chờ đã được tải lại."
            : describeError(error).message}
        </Banner>
      ) : null}
      <FormField label="Nhận xét" helper="Bắt buộc khi yêu cầu sửa." error={commentError ?? undefined}>
        <Textarea
          value={comment}
          maxLength={2000}
          placeholder="VD: Đổi hook cho ngắn hơn, kiểm tra lại giá ưu đãi."
          onChange={(e) => {
            setComment(e.target.value);
            if (commentError && e.target.value.trim()) setCommentError(null);
          }}
        />
      </FormField>
      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" disabled={decide.isPending} onClick={() => submit("request_changes")}>
          <MessageSquareWarning />
          Yêu cầu sửa
        </Button>
        <Button type="submit" variant="primary" loading={decide.isPending && decide.variables?.decision === "approve"} disabled={decide.isPending}>
          <Check />
          Duyệt asset
        </Button>
      </div>
    </form>
  );
}
