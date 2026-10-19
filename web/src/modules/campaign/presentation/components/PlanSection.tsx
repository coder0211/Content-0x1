import { useEffect, useState, type ReactNode } from "react";
import { Link } from "react-router";
import { CalendarDays, Plus, Trash2 } from "lucide-react";
import { isApiError } from "@/shared/kernel";
import { cn } from "@/shared/lib/cn";
import { formatDate, formatDateTime } from "@/shared/lib/format";
import { newIdempotencyKey } from "@/shared/lib/id";
import { useCan } from "@/shared/lib/session";
import { Banner } from "@/shared/ui/banner";
import { Button, buttonVariants } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { controlClass, Input } from "@/shared/ui/input";
import { EmptyState, JobProgress } from "@/shared/ui/patterns";
import { Textarea } from "@/shared/ui/textarea";
import { toast } from "@/shared/ui/toast";
import { useDiscardProposal, useGeneratePlan, usePlanJob, useSavePlan } from "../../application/useCampaigns";
import {
  acceptsGeneration,
  assetTypeOfFormat,
  CHANNELS,
  channelLabel,
  FORMATS,
  formatLabel,
  issueMessage,
  newPlanItem,
  planProgress,
  studioLink,
  validatePlan,
  type FieldErrors,
} from "../../domain/rules";
import type { Campaign, ContentPlan, PlanItem } from "../../domain/types";

function PlanEditor({
  initial,
  saveLabel,
  onSave,
  onCancel,
  saving,
  extraAction,
}: {
  initial: ContentPlan;
  saveLabel: string;
  onSave: (p: ContentPlan) => Promise<FieldErrors | null>;
  onCancel?: () => void;
  saving: boolean;
  extraAction?: ReactNode;
}) {
  const [plan, setPlan] = useState<ContentPlan>(initial);
  const [errors, setErrors] = useState<FieldErrors>({});
  const patchItem = (i: number, p: Partial<PlanItem>) => setPlan({ ...plan, items: plan.items.map((it, j) => (j === i ? { ...it, ...p } : it)) });
  const err = (i: number, f: string) => errors[`plan.items[${i}].${f}`];

  const save = async () => {
    const e = validatePlan(plan);
    setErrors(e);
    if (Object.keys(e).length) return;
    const server = await onSave(plan);
    if (server) setErrors(server);
  };

  return (
    <form
      className="flex flex-col gap-3"
      aria-label="Sửa content plan"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <Textarea aria-label="Tóm tắt plan" rows={2} placeholder="Tóm tắt" value={plan.summary} onChange={(e) => setPlan({ ...plan, summary: e.target.value })} />
      <div className="overflow-x-auto rounded-lg border border-neutral-200">
        <table className="w-full text-body">
          <thead className="bg-neutral-50 text-left text-small text-neutral-500">
            <tr>
              <th className="px-2 py-2 font-medium">Chủ đề *</th>
              <th className="px-2 py-2 font-medium">Góc tiếp cận</th>
              <th className="px-2 py-2 font-medium">Kênh</th>
              <th className="px-2 py-2 font-medium">Định dạng *</th>
              <th className="px-2 py-2 font-medium">Ngày đăng</th>
              <th className="w-10" />
            </tr>
          </thead>
          <tbody>
            {plan.items.map((it, i) => (
              <tr key={it.id} className="border-t border-neutral-200 align-top">
                <td className="p-2">
                  <Input aria-label={`Chủ đề ${i + 1}`} value={it.topic} aria-invalid={err(i, "topic") ? true : undefined} onChange={(e) => patchItem(i, { topic: e.target.value })} />
                  {err(i, "topic") ? <p className="mt-1 text-small text-danger">{issueMessage(err(i, "topic"))}</p> : null}
                </td>
                <td className="p-2">
                  <Input aria-label={`Góc tiếp cận ${i + 1}`} value={it.angle} onChange={(e) => patchItem(i, { angle: e.target.value })} />
                </td>
                <td className="p-2">
                  <select aria-label={`Kênh ${i + 1}`} className={cn(controlClass, "w-32")} value={it.channel} onChange={(e) => patchItem(i, { channel: e.target.value })}>
                    <option value="">—</option>
                    {it.channel && !CHANNELS.some((c) => c.value === it.channel) ? <option value={it.channel}>{it.channel}</option> : null}
                    {CHANNELS.map((c) => (
                      <option key={c.value} value={c.value}>
                        {c.label}
                      </option>
                    ))}
                  </select>
                </td>
                <td className="p-2">
                  <select
                    aria-label={`Định dạng ${i + 1}`}
                    className={cn(controlClass, "w-40")}
                    value={it.format}
                    aria-invalid={err(i, "format") ? true : undefined}
                    onChange={(e) => patchItem(i, { format: e.target.value })}
                  >
                    <option value="">Chọn</option>
                    {it.format && !FORMATS.some((f) => f.value === it.format) ? <option value={it.format}>{it.format}</option> : null}
                    {FORMATS.map((f) => (
                      <option key={f.value} value={f.value}>
                        {f.label}
                      </option>
                    ))}
                  </select>
                  {err(i, "format") ? <p className="mt-1 text-small text-danger">{issueMessage(err(i, "format"))}</p> : null}
                </td>
                <td className="p-2">
                  <Input aria-label={`Ngày đăng ${i + 1}`} type="date" className="w-40" value={it.publishDate} onChange={(e) => patchItem(i, { publishDate: e.target.value })} />
                </td>
                <td className="p-2">
                  <Button type="button" size="icon" variant="ghost" aria-label={`Xóa mục ${i + 1}`} onClick={() => setPlan({ ...plan, items: plan.items.filter((_, j) => j !== i) })}>
                    <Trash2 />
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {errors["plan.items"] ? (
        <p role="alert" className="text-small text-danger">
          Plan cần ít nhất một mục.
        </p>
      ) : null}
      <div className="flex items-center justify-between">
        <Button type="button" size="sm" onClick={() => setPlan({ ...plan, items: [...plan.items, newPlanItem(plan.items)] })}>
          <Plus /> Thêm mục
        </Button>
        <div className="flex items-center gap-2">
          {extraAction}
          {onCancel ? (
            <Button type="button" variant="ghost" onClick={onCancel}>
              Hủy
            </Button>
          ) : null}
          <Button type="submit" variant="primary" loading={saving}>
            {saveLabel}
          </Button>
        </div>
      </div>
    </form>
  );
}

function PlanChecklist({ campaign, canEdit, canGenerate }: { campaign: Campaign; canEdit: boolean; canGenerate: boolean }) {
  const plan = campaign.contentPlan!;
  const save = useSavePlan(campaign.id);
  const { done, total } = planProgress(plan);
  const toggle = (id: string, v: boolean) =>
    save.mutate({ ...plan, items: plan.items.map((it) => (it.id === id ? { ...it, done: v } : it)) }, { onError: (e) => toast.error(e) });
  return (
    <div className="flex flex-col gap-3">
      {plan.summary ? <p className="text-body text-neutral-700">{plan.summary}</p> : null}
      <p className="text-small text-neutral-500">
        Đã xong <span className="tabular">{done}/{total}</span> đầu ra
        {campaign.contentPlanApprovedAt ? ` · Duyệt lúc ${formatDateTime(campaign.contentPlanApprovedAt)}` : null}
      </p>
      <ul className="divide-y divide-neutral-200 rounded-lg border border-neutral-200" aria-label="Checklist đầu ra">
        {plan.items.map((it) => (
          <li key={it.id} className="flex items-center gap-3 px-3 py-2">
            <Checkbox id={`plan-${it.id}`} aria-label={`Đã xong: ${it.topic}`} checked={it.done} disabled={!canEdit || save.isPending} onCheckedChange={(v) => toggle(it.id, v === true)} />
            <label htmlFor={`plan-${it.id}`} className={cn("min-w-0 flex-1 text-body", it.done ? "text-neutral-500 line-through" : "text-neutral-900")}>
              {it.topic}
              {it.angle ? <span className="ml-2 text-small text-neutral-500">{it.angle}</span> : null}
            </label>
            <span className="text-small text-neutral-500">
              {[formatLabel(it.format), it.channel ? channelLabel(it.channel) : null].filter(Boolean).join(" · ")}
            </span>
            {it.publishDate ? (
              <span className="flex items-center gap-1 text-small text-neutral-500 tabular">
                <CalendarDays className="size-4" aria-hidden />
                {formatDate(it.publishDate)}
              </span>
            ) : null}
            {canGenerate && !it.done ? (
              <Link
                to={studioLink(assetTypeOfFormat(it.format), campaign.id, it.id)}
                aria-label={`Tạo đầu ra: ${it.topic}`}
                className={buttonVariants({ size: "sm", variant: "ghost" })}
              >
                Tạo
              </Link>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Content plan of a campaign (ACS-037): AI proposal → edit → approve → checklist on Overview. */
export function PlanSection({ campaign, canEdit }: { campaign: Campaign; canEdit: boolean }) {
  const canGenerate = useCan("generate.run") && acceptsGeneration(campaign.status);
  const [editing, setEditing] = useState<ContentPlan | null>(null);
  const [jobId, setJobId] = useState<string | null>(null);
  const [key, setKey] = useState<string>(() => newIdempotencyKey());
  const generate = useGeneratePlan(campaign.id);
  const job = usePlanJob(campaign.id, jobId);
  const save = useSavePlan(campaign.id);
  const discard = useDiscardProposal(campaign.id);

  const jobStatus = job.data?.status;
  useEffect(() => {
    if (jobId && jobStatus === "succeeded") {
      setJobId(null);
      toast.success("Đã có đề xuất content plan");
    }
  }, [jobId, jobStatus]);

  const startGeneration = async () => {
    try {
      const r = await generate.mutateAsync(key);
      setKey(newIdempotencyKey());
      setJobId(r.jobId);
    } catch {
      // toast shown by the global mutation handler
    }
  };

  const onSave = async (p: ContentPlan): Promise<FieldErrors | null> => {
    try {
      await save.mutateAsync(p);
      toast.success("Đã duyệt content plan");
      setEditing(null);
      return null;
    } catch (e) {
      if (isApiError(e) && e.code === "VALIDATION_FAILED") return e.fieldErrors();
      toast.error(e);
      return null;
    }
  };

  const generateButton = canGenerate ? (
    <Button onClick={() => void startGeneration()} loading={generate.isPending} disabled={!!jobId}>
      {campaign.contentPlan || campaign.planProposal ? "Tạo lại kế hoạch" : "Tạo kế hoạch"}
    </Button>
  ) : null;

  let body: ReactNode;
  if (editing) {
    body = <PlanEditor initial={editing} saveLabel="Lưu và duyệt plan" saving={save.isPending} onSave={onSave} onCancel={() => setEditing(null)} />;
  } else if (campaign.planProposal && canEdit) {
    body = (
      <div className="flex flex-col gap-3">
        <Banner tone="info" title="Đề xuất từ AI đang chờ duyệt">
          Sửa nếu cần rồi duyệt. Plan chỉ được lưu sau khi bạn duyệt.
          {campaign.planProposedAt ? ` Tạo lúc ${formatDateTime(campaign.planProposedAt)}.` : null}
        </Banner>
        <PlanEditor
          key={campaign.planProposedAt ?? "proposal"}
          initial={campaign.planProposal}
          saveLabel="Duyệt plan"
          saving={save.isPending}
          onSave={onSave}
          extraAction={
            <Button type="button" variant="ghost" loading={discard.isPending} onClick={() => discard.mutate()}>
              Bỏ đề xuất
            </Button>
          }
        />
      </div>
    );
  } else if (campaign.contentPlan) {
    body = <PlanChecklist campaign={campaign} canEdit={canEdit} canGenerate={canGenerate} />;
  } else {
    body = (
      <EmptyState
        icon={CalendarDays}
        message={campaign.planProposal ? "Đang có đề xuất plan chờ người phụ trách duyệt." : "Chưa có content plan."}
        action={
          canEdit ? (
            <Button size="sm" onClick={() => setEditing({ summary: "", themes: [], items: [newPlanItem([])] })}>
              Tự lập plan
            </Button>
          ) : undefined
        }
      />
    );
  }

  return (
    <section aria-labelledby="plan-heading" className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <h2 id="plan-heading" className="text-section text-neutral-900">
          Content plan
        </h2>
        <div className="flex items-center gap-2">
          {campaign.contentPlan && canEdit && !editing && !(campaign.planProposal && canEdit) ? (
            <Button variant="ghost" onClick={() => setEditing(campaign.contentPlan)}>
              Sửa plan
            </Button>
          ) : null}
          {generateButton}
        </div>
      </div>
      {jobId && job.data && jobStatus !== "succeeded" ? (
        <JobProgress
          status={job.data.status}
          progress={job.data.progress}
          label="Đang lập content plan từ brief"
          errorKind={job.data.errorKind}
          errorDetail={job.data.errorDetail ?? undefined}
          onRetry={jobStatus === "failed" ? () => void startGeneration() : undefined}
        />
      ) : null}
      {body}
    </section>
  );
}
