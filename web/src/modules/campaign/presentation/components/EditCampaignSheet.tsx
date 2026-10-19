import { useState } from "react";
import { isApiError } from "@/shared/kernel";
import { Button } from "@/shared/ui/button";
import { Sheet, SheetBody, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { toast } from "@/shared/ui/toast";
import { useUpdateCampaign } from "../../application/useCampaigns";
import { validateBrief, validateHeader, type FieldErrors } from "../../domain/rules";
import type { Campaign, CampaignDraft, CampaignGoal } from "../../domain/types";
import { BriefFields, HeaderFields } from "./CampaignFields";

function toDraft(c: Campaign): CampaignDraft {
  return { brandId: c.brandId, name: c.name, goal: c.goal, startDate: c.startDate ?? "", endDate: c.endDate ?? "", brief: c.brief };
}

/** Edit header + brief of an existing campaign (PATCH). */
export function EditCampaignSheet({ campaign, open, onOpenChange }: { campaign: Campaign; open: boolean; onOpenChange: (o: boolean) => void }) {
  const [draft, setDraft] = useState<CampaignDraft>(() => toDraft(campaign));
  const [errors, setErrors] = useState<FieldErrors>({});
  const update = useUpdateCampaign(campaign.id);

  const submit = async () => {
    const e = { ...validateHeader(draft), ...validateBrief(draft.brief) };
    setErrors(e);
    if (Object.keys(e).length) return;
    try {
      await update.mutateAsync({
        name: draft.name,
        goal: draft.goal as CampaignGoal,
        startDate: draft.startDate,
        endDate: draft.endDate,
        brief: draft.brief,
      });
      toast.success("Đã lưu brief");
      onOpenChange(false);
    } catch (err) {
      if (isApiError(err) && err.code === "VALIDATION_FAILED") setErrors(err.fieldErrors());
      else toast.error(err);
    }
  };

  return (
    <Sheet
      open={open}
      onOpenChange={(o) => {
        if (o) {
          setDraft(toDraft(campaign));
          setErrors({});
        }
        onOpenChange(o);
      }}
    >
      <SheetContent>
        <form
          className="flex h-full flex-col"
          noValidate
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <SheetHeader>
            <SheetTitle>Sửa brief</SheetTitle>
            <SheetDescription>Nội dung sinh sau khi lưu sẽ dùng brief mới.</SheetDescription>
          </SheetHeader>
          <SheetBody className="flex flex-col gap-6">
            <HeaderFields value={draft} onChange={setDraft} errors={errors} brandLocked />
            <BriefFields brandId={draft.brandId} value={draft.brief} onChange={(brief) => setDraft({ ...draft, brief })} errors={errors} />
          </SheetBody>
          <SheetFooter>
            <Button type="button" onClick={() => onOpenChange(false)}>
              Hủy
            </Button>
            <Button type="submit" variant="primary" loading={update.isPending}>
              Lưu brief
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
