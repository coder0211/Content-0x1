import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { isApiError } from "@/shared/kernel";
import { Button } from "@/shared/ui/button";
import { Banner } from "@/shared/ui/banner";
import { PageHeader } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";
import { useCreateCampaign } from "../../application/useCampaigns";
import { emptyDraft, isHeaderField, validateBrief, validateHeader, type FieldErrors } from "../../domain/rules";
import type { CampaignDraft, CampaignGoal } from "../../domain/types";
import { BriefFields, HeaderFields } from "../components/CampaignFields";

const STEPS = ["Thông tin chung", "Brief"] as const;

/** 2-step brief wizard (ACS-026): header → brief. Server field errors jump back to the step that owns them. */
export default function NewCampaignPage() {
  const [params] = useSearchParams();
  const [draft, setDraft] = useState<CampaignDraft>(() => emptyDraft(params.get("brand_id") ?? ""));
  const [step, setStep] = useState<0 | 1>(0);
  const [errors, setErrors] = useState<FieldErrors>({});
  const create = useCreateCampaign();
  const navigate = useNavigate();

  const next = () => {
    const e = validateHeader(draft);
    setErrors(e);
    if (Object.keys(e).length === 0) setStep(1);
  };

  const submit = async () => {
    const e = { ...validateHeader(draft), ...validateBrief(draft.brief) };
    setErrors(e);
    if (Object.keys(e).length) {
      if (Object.keys(e).some(isHeaderField)) setStep(0);
      return;
    }
    try {
      const c = await create.mutateAsync({
        brandId: draft.brandId,
        name: draft.name,
        goal: draft.goal as CampaignGoal,
        brief: draft.brief,
        startDate: draft.startDate || null,
        endDate: draft.endDate || null,
        memberIds: [],
      });
      toast.success("Đã tạo campaign");
      void navigate(`/campaigns/${c.id}`);
    } catch (err) {
      if (isApiError(err) && err.code === "VALIDATION_FAILED") {
        const fe = err.fieldErrors();
        setErrors(fe);
        if (Object.keys(fe).some(isHeaderField)) setStep(0);
      } else toast.error(err);
    }
  };

  return (
    <div className="max-w-2xl">
      <PageHeader title="Tạo campaign" description="Brief dùng chung cho mọi nội dung, hình ảnh và video của campaign." />
      <ol className="mb-6 flex items-center gap-6" aria-label="Các bước">
        {STEPS.map((s, i) => (
          <li key={s} aria-current={step === i ? "step" : undefined} className={i === step ? "text-body font-medium text-neutral-900" : "text-body text-neutral-500"}>
            <span className="tabular mr-2 text-neutral-500">{i + 1}.</span>
            {s}
          </li>
        ))}
      </ol>
      {errors.brand_id === "not_found" ? <Banner tone="danger">Brand không tồn tại hoặc đã bị xóa.</Banner> : null}
      <form
        className="flex flex-col gap-6"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          if (step === 0) next();
          else void submit();
        }}
      >
        {step === 0 ? (
          <HeaderFields value={draft} onChange={setDraft} errors={errors} />
        ) : (
          <BriefFields brandId={draft.brandId} value={draft.brief} onChange={(brief) => setDraft({ ...draft, brief })} errors={errors} />
        )}
        <div className="flex items-center justify-between border-t border-neutral-200 pt-4">
          <Button type="button" variant="ghost" onClick={() => (step === 0 ? void navigate("/campaigns") : setStep(0))}>
            {step === 0 ? "Hủy" : "Quay lại"}
          </Button>
          {step === 0 ? (
            <Button type="submit" variant="primary">
              Tiếp tục
            </Button>
          ) : (
            <Button type="submit" variant="primary" loading={create.isPending}>
              Tạo campaign
            </Button>
          )}
        </div>
      </form>
    </div>
  );
}
