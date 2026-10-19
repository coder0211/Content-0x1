import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import {
  AlertTriangle,
  ArrowRightLeft,
  Clapperboard,
  History,
  Library,
  RotateCcw,
  Send,
} from "lucide-react";
import type { ID } from "@/shared/kernel";
import { formatRelative } from "@/shared/lib/format";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu";
import { Input } from "@/shared/ui/input";
import {
  AIProvenance,
  ErrorState,
  FormField,
  StatusBadge,
} from "@/shared/ui/patterns";
import { Skeleton } from "@/shared/ui/skeleton";
import { Textarea } from "@/shared/ui/textarea";
import { toast } from "@/shared/ui/toast";
import {
  useRestoreVersion,
  useRewrite,
  useSaveVersion,
  useSubmitReview,
  useTransform,
} from "../../application/mutations";
import { useAssetVersions, useContentAsset } from "../../application/queries";
import {
  FORMATS,
  REWRITES,
  TRANSFORM_TARGETS,
  WARNING_LABELS,
  activeWarnings,
  applyDraft,
  contentTexts,
  findWarning,
  isDirty,
  labelOf,
  parseHashtags,
  parseTones,
  videoStudioLink,
  type ContentAsset,
  type ContentBody,
  type ContentFormat,
  type ContentWarning,
  type RewriteInstruction,
} from "../../domain/content";

export interface StartedJob {
  id: ID;
  kind: "rewrite" | "transform";
  assetId: ID;
  label: string;
}

interface Draft {
  title: string;
  hook: string;
  bodyText: string;
  cta: string;
  hashtags: string;
}

type FieldKey = "title" | "hook" | "bodyText" | "cta";

const toDraft = (c: ContentBody): Draft => ({
  title: c.title,
  hook: c.hook,
  bodyText: c.bodyText,
  cta: c.cta,
  hashtags: c.hashtags.join(" "),
});

const emptyBody: ContentBody = {
  title: "",
  hook: "",
  bodyText: "",
  bodyHtml: "",
  cta: "",
  hashtags: [],
  platform: "",
  format: "",
  language: "vi",
  scenes: [],
  warnings: [],
  provenance: null,
};

export interface AssetWorkspaceProps {
  assetId: ID;
  /** A rewrite/transform job of this asset is running: quick actions are disabled. */
  busy: boolean;
  onJobStarted: (job: StartedJob) => void;
}

/** Center editor (max 720px) + 300px inspector (versions, warnings, provenance) for one variant. */
export function AssetWorkspace({
  assetId,
  busy,
  onJobStarted,
}: AssetWorkspaceProps) {
  const asset = useContentAsset(assetId);
  if (asset.isLoading) {
    return (
      <div className="flex flex-1 gap-4 p-6" aria-busy="true">
        <div className="mx-auto w-full max-w-editor space-y-3">
          <Skeleton className="h-8 w-2/3" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      </div>
    );
  }
  if (asset.error || !asset.data) {
    return (
      <div className="p-6">
        <ErrorState error={asset.error} onRetry={() => void asset.refetch()} />
      </div>
    );
  }
  return (
    <Workspace
      key={`${assetId}:${asset.data.currentVersion?.versionNo ?? 0}`}
      asset={asset.data}
      busy={busy}
      onJobStarted={onJobStarted}
    />
  );
}

function Workspace({
  asset,
  busy,
  onJobStarted,
}: {
  asset: ContentAsset;
  busy: boolean;
  onJobStarted: (job: StartedJob) => void;
}) {
  const navigate = useNavigate();
  const base = asset.currentVersion?.content ?? emptyBody;
  const versionNo = asset.currentVersion?.versionNo ?? 0;
  const [draft, setDraft] = useState<Draft>(() => toDraft(base));
  const [changeNote, setChangeNote] = useState("");
  const [rewriteMode, setRewriteMode] = useState<
    "change_tone" | "custom" | null
  >(null);
  const [rewriteText, setRewriteText] = useState("");
  const refs = {
    title: useRef<HTMLInputElement>(null),
    hook: useRef<HTMLInputElement>(null),
    bodyText: useRef<HTMLTextAreaElement>(null),
    cta: useRef<HTMLInputElement>(null),
  };

  const save = useSaveVersion(asset.id);
  const rewrite = useRewrite(asset.id);
  const transform = useTransform(asset.id);
  const submit = useSubmitReview(asset.id);

  const next = useMemo(
    () =>
      applyDraft(base, { ...draft, hashtags: parseHashtags(draft.hashtags) }),
    [base, draft],
  );
  const dirty = isDirty(base, next);
  const warnings = activeWarnings(base.warnings, contentTexts(next));
  const set = (k: keyof Draft) => (e: { target: { value: string } }) =>
    setDraft((d) => ({ ...d, [k]: e.target.value }));

  useEffect(() => {
    setRewriteMode(null);
    setRewriteText("");
  }, [versionNo]);

  const jump = (w: ContentWarning) => {
    for (const k of ["bodyText", "title", "hook", "cta"] as FieldKey[]) {
      const pos = findWarning(draft[k], w);
      const el = refs[k].current;
      if (pos && el) {
        el.focus();
        el.setSelectionRange(pos.start, pos.end);
        return;
      }
    }
  };

  const doSave = () => {
    save.mutate(
      {
        content: next,
        changeNote: changeNote.trim() || "Chỉnh sửa thủ công",
        baseVersionNo: versionNo,
      },
      {
        onSuccess: (v) => {
          setChangeNote("");
          toast.success(`Đã lưu phiên bản ${v.versionNo}`);
        },
      },
    );
  };

  const doRewrite = (instruction: RewriteInstruction) => {
    if (instruction === "change_tone" || instruction === "custom") {
      if (rewriteMode !== instruction) {
        setRewriteMode(instruction);
        return;
      }
      if (!rewriteText.trim()) return;
    }
    const input =
      instruction === "change_tone"
        ? { instruction, tone: parseTones(rewriteText) }
        : instruction === "custom"
          ? { instruction, customPrompt: rewriteText.trim() }
          : { instruction };
    rewrite.mutate(input, {
      onSuccess: (job) =>
        onJobStarted({
          id: job.jobId,
          kind: "rewrite",
          assetId: asset.id,
          label: `Đang viết lại (${labelOf(REWRITES, instruction)})`,
        }),
    });
  };

  const doTransform = (target: ContentFormat) => {
    transform.mutate(target, {
      onSuccess: (job) =>
        onJobStarted({
          id: job.jobId,
          kind: "transform",
          assetId: asset.id,
          label: `Đang chuyển sang ${labelOf(FORMATS, target)}`,
        }),
    });
  };

  const actionsDisabled = busy || rewrite.isPending || transform.isPending;

  return (
    <div className="flex min-h-0 flex-1">
      <section
        aria-label="Trình soạn thảo"
        className="min-w-0 flex-1 overflow-y-auto p-6"
      >
        <div className="mx-auto w-full max-w-editor space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge kind="asset" status={asset.status} />
            <span className="text-small text-neutral-500">
              {labelOf(FORMATS, asset.subtype)} · Phiên bản {versionNo}
            </span>
          </div>

          <div
            role="toolbar"
            aria-label="Viết lại nhanh"
            className="flex flex-wrap items-center gap-2 rounded border border-neutral-200 bg-neutral-0 p-2"
          >
            {REWRITES.map((r) => (
              <Button
                key={r.value}
                size="sm"
                variant={rewriteMode === r.value ? "secondary" : "ghost"}
                disabled={actionsDisabled || dirty}
                onClick={() => doRewrite(r.value)}
              >
                {r.label}
              </Button>
            ))}
            {dirty ? (
              <span className="text-caption text-neutral-500">
                Lưu phiên bản trước khi viết lại.
              </span>
            ) : null}
          </div>
          {rewriteMode ? (
            <div className="flex items-end gap-2">
              <FormField
                label={
                  rewriteMode === "change_tone"
                    ? "Tone mới"
                    : "Yêu cầu chỉnh sửa"
                }
                className="flex-1"
              >
                <Input
                  value={rewriteText}
                  onChange={(e) => setRewriteText(e.target.value)}
                  maxLength={rewriteMode === "custom" ? 1000 : 200}
                  placeholder={
                    rewriteMode === "change_tone"
                      ? "trẻ trung, hài hước"
                      : "Thêm câu hỏi mở ở đầu bài"
                  }
                />
              </FormField>
              <Button
                size="md"
                variant="secondary"
                disabled={actionsDisabled || !rewriteText.trim()}
                onClick={() => doRewrite(rewriteMode)}
              >
                Viết lại
              </Button>
              <Button
                size="md"
                variant="ghost"
                onClick={() => setRewriteMode(null)}
              >
                Bỏ qua
              </Button>
            </div>
          ) : null}

          <div className="space-y-4 rounded border border-neutral-200 bg-neutral-0 p-4">
            <FormField label="Tiêu đề">
              <Input
                ref={refs.title}
                value={draft.title}
                onChange={set("title")}
                maxLength={200}
              />
            </FormField>
            <FormField label="Hook">
              <Input
                ref={refs.hook}
                value={draft.hook}
                onChange={set("hook")}
              />
            </FormField>
            <FormField label="Nội dung">
              <Textarea
                ref={refs.bodyText}
                value={draft.bodyText}
                onChange={set("bodyText")}
                className="min-h-72 text-editor"
              />
            </FormField>
            {warnings.length ? (
              <ul
                aria-label="Cảnh báo trong nội dung"
                className="flex flex-wrap gap-2"
              >
                {warnings.map((w) => (
                  <li key={`${w.type}:${w.text}`}>
                    <button
                      type="button"
                      onClick={() => jump(w)}
                      className="inline-flex items-center gap-1 rounded-sm border border-warning-border bg-warning-bg px-2 py-0.5 text-small text-warning underline decoration-wavy underline-offset-2"
                    >
                      <AlertTriangle className="size-3.5" aria-hidden />
                      {WARNING_LABELS[w.type]}: {w.text}
                    </button>
                  </li>
                ))}
              </ul>
            ) : null}
            <div className="grid grid-cols-2 gap-3">
              <FormField label="CTA">
                <Input ref={refs.cta} value={draft.cta} onChange={set("cta")} />
              </FormField>
              <FormField label="Hashtag">
                <Input
                  value={draft.hashtags}
                  onChange={set("hashtags")}
                  placeholder="#tienganh #giaotiep"
                />
              </FormField>
            </div>
            {base.scenes.length ? (
              <div className="space-y-2">
                <h3 className="text-small font-medium text-neutral-700">
                  Cảnh ({base.scenes.length})
                </h3>
                <ol className="space-y-1 text-small text-neutral-700">
                  {base.scenes.map((s, i) => (
                    <li
                      key={i}
                      className="rounded border border-neutral-200 px-3 py-2"
                    >
                      <span className="font-medium">Cảnh {i + 1}</span>
                      {s.durationSec ? (
                        <span className="tabular text-neutral-500">
                          {" "}
                          · {s.durationSec}s
                        </span>
                      ) : null}{" "}
                      — {s.narration}
                      {s.visualHint ? (
                        <span className="block text-caption text-neutral-500">
                          {s.visualHint}
                        </span>
                      ) : null}
                    </li>
                  ))}
                </ol>
              </div>
            ) : null}
          </div>

          <div className="flex flex-wrap items-end gap-2">
            <FormField label="Ghi chú thay đổi" className="min-w-60 flex-1">
              <Input
                value={changeNote}
                onChange={(e) => setChangeNote(e.target.value)}
                placeholder="Sửa hook, thêm CTA"
                maxLength={200}
              />
            </FormField>
            <Button
              variant="primary"
              disabled={!dirty}
              loading={save.isPending}
              onClick={doSave}
            >
              Lưu phiên bản
            </Button>
          </div>
          <div className="flex flex-wrap items-center gap-2 border-t border-neutral-200 pt-4">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="secondary" disabled={actionsDisabled}>
                  <ArrowRightLeft aria-hidden />
                  Chuyển định dạng
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                {TRANSFORM_TARGETS.filter((t) => t !== asset.subtype).map(
                  (t) => (
                    <DropdownMenuItem key={t} onSelect={() => doTransform(t)}>
                      {labelOf(FORMATS, t)}
                    </DropdownMenuItem>
                  ),
                )}
              </DropdownMenuContent>
            </DropdownMenu>
            <Button
              variant="secondary"
              onClick={() => void navigate(videoStudioLink(asset.id))}
            >
              <Clapperboard aria-hidden />
              Gửi sang Video Studio
            </Button>
            <Button asChild variant="ghost">
              <Link to={`/library/${asset.id}`}>
                <Library aria-hidden />
                Mở trong thư viện
              </Link>
            </Button>
            {asset.status === "draft" ? (
              <Button
                variant="secondary"
                className="ml-auto"
                disabled={dirty || actionsDisabled}
                title={dirty ? "Lưu phiên bản trước khi gửi duyệt" : undefined}
                loading={submit.isPending}
                onClick={() =>
                  submit.mutate(undefined, {
                    onSuccess: () => toast.success("Đã gửi duyệt"),
                  })
                }
              >
                <Send aria-hidden />
                Gửi duyệt
              </Button>
            ) : null}
          </div>
        </div>
      </section>
      <Inspector asset={asset} base={base} warnings={warnings} onJump={jump} />
    </div>
  );
}

function Inspector({
  asset,
  base,
  warnings,
  onJump,
}: {
  asset: ContentAsset;
  base: ContentBody;
  warnings: ContentWarning[];
  onJump: (w: ContentWarning) => void;
}) {
  const versions = useAssetVersions(asset.id);
  const restore = useRestoreVersion(asset.id);
  const current = asset.currentVersion?.versionNo ?? 0;
  const p = base.provenance;

  return (
    <aside
      aria-label="Thông tin phiên bản"
      className="w-[300px] shrink-0 space-y-6 overflow-y-auto border-l border-neutral-200 bg-neutral-0 p-4"
    >
      <section className="space-y-2">
        <h3 className="text-small font-semibold text-neutral-900">
          Cảnh báo ({warnings.length})
        </h3>
        {warnings.length ? (
          <ul className="space-y-1">
            {warnings.map((w) => (
              <li key={`${w.type}:${w.text}`}>
                <button
                  type="button"
                  onClick={() => onJump(w)}
                  className="w-full rounded px-2 py-1 text-left text-small text-neutral-700 hover:bg-neutral-100"
                >
                  <span className="text-warning">{WARNING_LABELS[w.type]}</span>{" "}
                  · {w.text}
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-small text-neutral-500">Không có cảnh báo.</p>
        )}
        {warnings.some((w) => w.type === "unverified_claim") ? (
          <Banner tone="warning">
            Kiểm tra lại số liệu chưa có trong thông tin sản phẩm trước khi gửi
            duyệt.
          </Banner>
        ) : null}
      </section>

      <section className="space-y-2">
        <h3 className="flex items-center gap-1.5 text-small font-semibold text-neutral-900">
          <History className="size-4" aria-hidden />
          Phiên bản
        </h3>
        {versions.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : versions.error ? (
          <ErrorState
            error={versions.error}
            onRetry={() => void versions.refetch()}
          />
        ) : (
          <ol aria-label="Lịch sử phiên bản" className="space-y-1">
            {(versions.data ?? []).map((v) => (
              <li
                key={v.versionNo}
                className="flex items-start justify-between gap-2 rounded px-2 py-1.5 hover:bg-neutral-50"
              >
                <div className="min-w-0">
                  <p className="text-small font-medium text-neutral-900">
                    Phiên bản {v.versionNo}
                    {v.versionNo === current ? (
                      <span className="font-normal text-neutral-500">
                        {" "}
                        · hiện tại
                      </span>
                    ) : null}
                  </p>
                  <p
                    className="truncate text-caption text-neutral-500"
                    title={v.changeNote}
                  >
                    {v.changeNote || "—"} · {formatRelative(v.createdAt)}
                  </p>
                </div>
                {v.versionNo !== current ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={`Khôi phục phiên bản ${v.versionNo}`}
                    disabled={restore.isPending}
                    onClick={() =>
                      restore.mutate(v.versionNo, {
                        onSuccess: (nv) =>
                          toast.success(
                            `Đã khôi phục thành phiên bản ${nv.versionNo}`,
                          ),
                      })
                    }
                  >
                    <RotateCcw aria-hidden />
                    Khôi phục
                  </Button>
                ) : null}
              </li>
            ))}
          </ol>
        )}
      </section>

      {p ? (
        <section className="space-y-1">
          <h3 className="text-small font-semibold text-neutral-900">
            Nguồn gốc
          </h3>
          <AIProvenance
            model={p.modelCode || p.providerCode}
            brandVersion={p.brandVersionNo}
            createdAt={asset.currentVersion?.createdAt ?? new Date()}
          />
          <p className="text-caption text-neutral-500">
            Template {p.promptTemplateCode}
            {p.promptTemplateVersion != null
              ? ` v${p.promptTemplateVersion}`
              : ""}
          </p>
        </section>
      ) : null}
    </aside>
  );
}
