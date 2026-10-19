import { useState, type ReactNode } from "react";
import { Link } from "react-router";
import { isApiError } from "@/shared/kernel";
import { formatDateTime } from "@/shared/lib/format";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Textarea } from "@/shared/ui/textarea";
import { FormField } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";
import { useUpdateAsset } from "../../application/useLibrary";
import { normalizeTags, type Approval, type AssetDetail, type Provenance } from "../../domain/asset";

export function Panel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2 border-b border-neutral-200 pb-4 last:border-b-0" aria-label={title}>
      <h2 className="text-caption font-medium uppercase text-neutral-500">{title}</h2>
      {children}
    </section>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-3 text-small">
      <dt className="text-neutral-500">{label}</dt>
      <dd className="min-w-0 truncate text-right text-neutral-900">{children}</dd>
    </div>
  );
}

/** Job / provider / model / prompt version / brand version (FR-AST-02). */
export function ProvenancePanel({ provenance }: { provenance: Provenance | null }) {
  return (
    <Panel title="Nguồn sinh">
      {provenance ? (
        <dl className="flex flex-col gap-1.5">
          <Field label="Job">
            <Link className="font-mono text-accent-700 hover:underline" to={`/jobs/${provenance.jobId}`}>
              {provenance.jobId.slice(0, 8)}
            </Link>
          </Field>
          <Field label="Loại job">{provenance.jobType}</Field>
          <Field label="Provider">{provenance.providerCode ?? "—"}</Field>
          <Field label="Model">{provenance.modelCode ?? "—"}</Field>
          <Field label="Prompt">
            {provenance.promptTemplateCode ? `${provenance.promptTemplateCode} v${provenance.promptTemplateVersion ?? "?"}` : "—"}
          </Field>
          <Field label="Brand">{provenance.brandVersionNo != null ? `v${provenance.brandVersionNo}` : "—"}</Field>
        </dl>
      ) : (
        <p className="text-small text-neutral-500">Phiên bản này được sửa thủ công, không do AI sinh.</p>
      )}
    </Panel>
  );
}

const DECISION_LABEL: Record<Approval["decision"], string> = {
  submitted: "Gửi duyệt",
  approved: "Đã duyệt",
  changes_requested: "Yêu cầu sửa",
};

export function ApprovalHistory({ approvals }: { approvals: Approval[] }) {
  return (
    <Panel title="Lịch sử duyệt">
      {approvals.length === 0 ? (
        <p className="text-small text-neutral-500">Chưa gửi duyệt lần nào.</p>
      ) : (
        <ol className="flex flex-col gap-3">
          {approvals.map((a) => (
            <li key={a.id} className="flex flex-col gap-0.5 text-small">
              <p className="text-neutral-900">
                <span className="font-medium">{DECISION_LABEL[a.decision]}</span> · v{a.versionNo} · {a.reviewerName}
              </p>
              <p className="text-caption font-normal text-neutral-500">{formatDateTime(a.createdAt)}</p>
              {a.comment ? <p className="whitespace-pre-line text-neutral-700">{a.comment}</p> : null}
              {a.checklist.typography_checked === true ? <p className="text-caption font-normal text-neutral-500">Đã kiểm tra chữ trên poster</p> : null}
            </li>
          ))}
        </ol>
      )}
    </Panel>
  );
}

/** Tags & usage rights (ACS-085). Editable for owner/campaign member/Lead/Admin. */
export function TagsRightsPanel({ asset }: { asset: AssetDetail }) {
  const update = useUpdateAsset(asset.id);
  const [tags, setTags] = useState(asset.tags.join(", "));
  const [rights, setRights] = useState(asset.usageRights ?? "");
  const tagError = isApiError(update.error) ? update.error.details.find((d) => d.field?.startsWith("tags"))?.issue : undefined;

  if (!asset.permissions.canEdit) {
    return (
      <Panel title="Tag & quyền sử dụng">
        <p className="text-small text-neutral-900">{asset.tags.length ? asset.tags.join(", ") : "Chưa có tag."}</p>
        <p className="whitespace-pre-line text-small text-neutral-700">{asset.usageRights || "Chưa ghi chú quyền sử dụng."}</p>
      </Panel>
    );
  }

  const dirty = normalizeTags(tags).join(",") !== asset.tags.join(",") || rights.trim() !== (asset.usageRights ?? "");
  return (
    <Panel title="Tag & quyền sử dụng">
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          update.mutate(
            { tags: normalizeTags(tags), usageRights: rights.trim() || null },
            { onSuccess: () => toast.success("Đã lưu tag và quyền sử dụng") },
          );
        }}
      >
        <FormField label="Tag" helper="Phân tách bằng dấu phẩy" error={tagError ? "Tag không hợp lệ" : null}>
          <Input value={tags} onChange={(e) => setTags(e.target.value)} />
        </FormField>
        <FormField label="Quyền sử dụng" helper="Nguồn nhạc, ảnh tham chiếu, thời hạn license">
          <Textarea value={rights} maxLength={2000} onChange={(e) => setRights(e.target.value)} />
        </FormField>
        <Button type="submit" size="sm" disabled={!dirty} loading={update.isPending} className="self-end">
          Lưu thông tin
        </Button>
      </form>
    </Panel>
  );
}
