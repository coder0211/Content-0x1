import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { ArrowDown, ArrowUp, GripVertical, Plus, Trash2 } from "lucide-react";
import { isApiError, type SceneStatus } from "@/shared/kernel";
import { cn } from "@/shared/lib/cn";
import { describeError } from "@/shared/lib/errors";
import { formatDuration, formatDurationMs } from "@/shared/lib/format";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Textarea } from "@/shared/ui/textarea";
import { StatusBadge } from "@/shared/ui/patterns";
import type { StoryboardDrafts } from "../../application/useStoryboardDrafts";
import type { SceneDraft, StoryboardLimits } from "../../domain/video";

export interface StoryboardEditorProps {
  sb: StoryboardDrafts;
  limits: StoryboardLimits;
  /** Server status per scene id (drafts of new scenes have none). */
  statusById: Record<string, SceneStatus>;
  readOnly: boolean;
  locked: boolean;
}

/** Danh sách cảnh kéo-thả: sửa mô tả hình ảnh, lời thoại, phụ đề, thời lượng (ACS-071). */
export function StoryboardEditor({ sb, limits, statusById, readOnly, locked }: StoryboardEditorProps) {
  const sensors = useSensors(useSensor(PointerSensor), useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }));
  const disabled = readOnly || locked;
  const overTotal = sb.totalMs > limits.maxTotalSeconds * 1000;
  const overCount = sb.drafts.length > limits.maxScenes;

  function onDragEnd(e: DragEndEvent) {
    if (!e.over || e.active.id === e.over.id) return;
    const from = sb.drafts.findIndex((d) => d.key === e.active.id);
    const to = sb.drafts.findIndex((d) => d.key === e.over!.id);
    sb.move(from, to);
  }

  return (
    <section aria-label="Storyboard" className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-baseline gap-3">
          <h2 className="text-section font-semibold text-neutral-900">Storyboard</h2>
          <span className={cn("text-small tabular", overTotal ? "text-danger" : "text-neutral-600")} data-testid="storyboard-total">
            Tổng {formatDurationMs(sb.totalMs)} / {formatDuration(limits.maxTotalSeconds)}
          </span>
          <span className={cn("text-small tabular", overCount ? "text-danger" : "text-neutral-600")}>
            {sb.drafts.length} / {limits.maxScenes} cảnh
          </span>
        </div>
        {!readOnly ? (
          <div className="flex items-center gap-2">
            {sb.dirty ? (
              <Button variant="ghost" size="sm" onClick={sb.reset} disabled={sb.saving}>
                Hoàn tác
              </Button>
            ) : null}
            <Button variant="secondary" size="sm" onClick={sb.add} disabled={disabled || sb.drafts.length >= limits.maxScenes}>
              <Plus aria-hidden /> Thêm cảnh
            </Button>
            <Button
              variant="secondary"
              size="sm"
              onClick={sb.save}
              loading={sb.saving}
              disabled={disabled || !sb.dirty || sb.issues.length > 0}
            >
              Lưu storyboard
            </Button>
          </div>
        ) : null}
      </div>

      <TimelineBar durations={sb.drafts.map((d) => d.durationMs)} maxMs={limits.maxTotalSeconds * 1000} />

      {locked && !readOnly ? <Banner tone="info">Đang có job chạy cho video này. Storyboard tạm khóa đến khi job xong.</Banner> : null}
      {sb.issues.length > 0 ? (
        <Banner tone="warning" title="Storyboard chưa đạt giới hạn">
          <ul className="list-disc pl-4">
            {sb.issues.map((i) => (
              <li key={i.field + i.message}>{i.message}</li>
            ))}
          </ul>
        </Banner>
      ) : null}
      {sb.saveError ? <SaveError error={sb.saveError} /> : null}

      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
        <SortableContext items={sb.drafts.map((d) => d.key)} strategy={verticalListSortingStrategy}>
          <ol className="flex flex-col gap-2">
            {sb.drafts.map((d, i) => (
              <SceneRow
                key={d.key}
                draft={d}
                index={i}
                count={sb.drafts.length}
                status={d.id ? statusById[d.id] : undefined}
                limits={limits}
                disabled={disabled}
                onChange={(patch) => sb.update(d.key, patch)}
                onMove={(to) => sb.move(i, to)}
                onRemove={() => sb.remove(d.key)}
              />
            ))}
          </ol>
        </SortableContext>
      </DndContext>
    </section>
  );
}

function SaveError({ error }: { error: unknown }) {
  if (isApiError(error) && error.code === "VALIDATION_FAILED") {
    return <Banner tone="danger">Máy chủ từ chối storyboard: kiểm tra số cảnh và thời lượng.</Banner>;
  }
  return <Banner tone="danger">{describeError(error).message}</Banner>;
}

interface SceneRowProps {
  draft: SceneDraft;
  index: number;
  count: number;
  status: SceneStatus | undefined;
  limits: StoryboardLimits;
  disabled: boolean;
  onChange: (patch: Partial<SceneDraft>) => void;
  onMove: (to: number) => void;
  onRemove: () => void;
}

function SceneRow({ draft, index, count, status, limits, disabled, onChange, onMove, onRemove }: SceneRowProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: draft.key, disabled });
  const n = index + 1;
  const badDuration = draft.durationMs < limits.minSceneMs || draft.durationMs > limits.maxSceneMs;
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      aria-label={`Cảnh ${n}`}
      className={cn("rounded border border-neutral-200 bg-neutral-0", isDragging && "border-accent-600 shadow-sm")}
    >
      <div className="flex items-center gap-2 border-b border-neutral-200 px-3 py-2">
        <button
          type="button"
          className="cursor-grab text-neutral-500 hover:text-neutral-900 disabled:cursor-not-allowed"
          aria-label={`Kéo để sắp xếp cảnh ${n}`}
          disabled={disabled}
          {...attributes}
          {...listeners}
        >
          <GripVertical className="size-4" aria-hidden />
        </button>
        <span className="text-small font-medium text-neutral-900 tabular">Cảnh {n}</span>
        {status ? <StatusBadge kind="scene" status={status} /> : <span className="text-caption text-neutral-500">Mới</span>}
        <div className="ml-auto flex items-center gap-1">
          <Button variant="ghost" size="icon-sm" aria-label={`Đưa cảnh ${n} lên`} disabled={disabled || index === 0} onClick={() => onMove(index - 1)}>
            <ArrowUp aria-hidden />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={`Đưa cảnh ${n} xuống`}
            disabled={disabled || index === count - 1}
            onClick={() => onMove(index + 1)}
          >
            <ArrowDown aria-hidden />
          </Button>
          <Button variant="ghost" size="icon-sm" aria-label={`Xóa cảnh ${n}`} disabled={disabled || count <= 1} onClick={onRemove}>
            <Trash2 aria-hidden />
          </Button>
        </div>
      </div>
      <div className="grid grid-cols-[1fr_1fr_120px] gap-3 p-3">
        <label className="col-span-2 flex flex-col gap-1 text-small text-neutral-700">
          Mô tả hình ảnh
          <Textarea
            value={draft.visualPrompt}
            onChange={(e) => onChange({ visualPrompt: e.target.value })}
            rows={2}
            maxLength={1000}
            disabled={disabled}
            aria-label={`Mô tả hình ảnh cảnh ${n}`}
          />
        </label>
        <label className="flex flex-col gap-1 text-small text-neutral-700">
          Thời lượng (giây)
          <Input
            type="number"
            step={0.5}
            min={limits.minSceneMs / 1000}
            max={limits.maxSceneMs / 1000}
            value={draft.durationMs / 1000}
            onChange={(e) => onChange({ durationMs: Math.round(Number(e.target.value) * 1000) || 0 })}
            disabled={disabled}
            aria-label={`Thời lượng cảnh ${n}`}
            aria-invalid={badDuration || undefined}
            className={cn("tabular", badDuration && "border-danger")}
          />
        </label>
        <label className="flex flex-col gap-1 text-small text-neutral-700">
          Lời thoại
          <Textarea
            value={draft.narration}
            onChange={(e) => onChange({ narration: e.target.value })}
            rows={2}
            maxLength={1000}
            disabled={disabled}
            aria-label={`Lời thoại cảnh ${n}`}
          />
        </label>
        <label className="col-span-2 flex flex-col gap-1 text-small text-neutral-700">
          Phụ đề
          <Textarea
            value={draft.subtitleText}
            onChange={(e) => onChange({ subtitleText: e.target.value })}
            rows={2}
            maxLength={1000}
            disabled={disabled}
            aria-label={`Phụ đề cảnh ${n}`}
          />
        </label>
      </div>
    </li>
  );
}

/** Simple timeline: one segment per scene, proportional to its duration, against the 60 s limit. */
export function TimelineBar({ durations, maxMs, states }: { durations: number[]; maxMs: number; states?: (SceneStatus | undefined)[] }) {
  const total = durations.reduce((s, d) => s + d, 0);
  const scale = Math.max(total, maxMs);
  return (
    <div aria-label="Timeline" className="flex flex-col gap-1">
      <div className="flex h-6 w-full overflow-hidden rounded-sm border border-neutral-200 bg-neutral-100">
        {durations.map((d, i) => {
          const st = states?.[i];
          return (
            <div
              key={i}
              title={`Cảnh ${i + 1}: ${formatDurationMs(d)}`}
              style={{ width: `${(d / scale) * 100}%` }}
              className={cn(
                "flex items-center justify-center border-r border-neutral-0 text-caption tabular",
                st === "failed" ? "bg-danger-bg text-danger" : st === "ready" ? "bg-success-bg text-success" : "bg-accent-50 text-accent-700",
              )}
            >
              {i + 1}
            </div>
          );
        })}
      </div>
      <div className="flex justify-between text-caption text-neutral-500 tabular">
        <span>0:00</span>
        <span>{formatDurationMs(scale)}</span>
      </div>
    </div>
  );
}
