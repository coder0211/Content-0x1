import { useState } from "react";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Label } from "@/shared/ui/label";
import { ErrorState } from "@/shared/ui/patterns";
import { Skeleton } from "@/shared/ui/skeleton";
import { toast } from "@/shared/ui/toast";
import { useMemberCandidates, useUpdateCampaign } from "../../application/useCampaigns";
import type { Campaign } from "../../domain/types";

/** Assign content creators to the campaign (members may edit it and generate in it). */
export function MembersDialog({ campaign, open, onOpenChange }: { campaign: Campaign; open: boolean; onOpenChange: (o: boolean) => void }) {
  const candidates = useMemberCandidates(open);
  const [selected, setSelected] = useState<string[]>(campaign.memberIds);
  const update = useUpdateCampaign(campaign.id);

  const save = async () => {
    try {
      await update.mutateAsync({ memberIds: selected });
      toast.success("Đã cập nhật thành viên");
      onOpenChange(false);
    } catch (e) {
      toast.error(e);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (o) setSelected(campaign.memberIds);
        onOpenChange(o);
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Thành viên campaign</DialogTitle>
          <DialogDescription>Thành viên được sửa brief và tạo nội dung trong campaign này.</DialogDescription>
        </DialogHeader>
        {candidates.isLoading ? (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-5 w-56" />
            <Skeleton className="h-5 w-48" />
          </div>
        ) : candidates.error ? (
          <ErrorState error={candidates.error} onRetry={() => void candidates.refetch()} />
        ) : (
          <ul className="flex max-h-72 flex-col gap-2 overflow-y-auto">
            {(candidates.data ?? [])
              .filter((u) => u.id !== campaign.createdBy)
              .map((u) => (
                <li key={u.id} className="flex items-center gap-2">
                  <Checkbox
                    id={`member-${u.id}`}
                    checked={selected.includes(u.id)}
                    onCheckedChange={(v) => setSelected(v === true ? [...selected, u.id] : selected.filter((x) => x !== u.id))}
                  />
                  <Label htmlFor={`member-${u.id}`} className="font-normal">
                    {u.name} <span className="text-neutral-500">{u.email}</span>
                  </Label>
                </li>
              ))}
          </ul>
        )}
        <DialogFooter>
          <Button type="button" onClick={() => onOpenChange(false)}>
            Hủy
          </Button>
          <Button variant="primary" loading={update.isPending} onClick={() => void save()}>
            Lưu thành viên
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
