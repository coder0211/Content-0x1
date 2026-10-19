import { useRef, useState } from "react";
import { Film, RefreshCw } from "lucide-react";
import { isApiError } from "@/shared/kernel";
import { cn } from "@/shared/lib/cn";
import { describeError } from "@/shared/lib/errors";
import { formatDurationMs } from "@/shared/lib/format";
import { newIdempotencyKey } from "@/shared/lib/id";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Textarea } from "@/shared/ui/textarea";
import { toast } from "@/shared/ui/toast";
import { JobProgress, StatusBadge } from "@/shared/ui/patterns";
import { useRegenerateScene } from "../../application/useVideo";
import {
  failedScenes,
  isJobActive,
  jobProgressLabel,
  jobTypeLabel,
  stepLabel,
  type Scene,
  type VideoProject,
} from "../../domain/video";
import { TimelineBar } from "./StoryboardEditor";

const ASPECT: Record<string, string> = { "9:16": "aspect-[9/16]", "1:1": "aspect-square", "16:9": "aspect-video" };

export interface ProductionViewProps {
  project: VideoProject;
  canEdit: boolean;
}

/** Trạng thái từng cảnh, sinh lại cảnh, timeline, player (ACS-072). */
export function ProductionView({ project, canEdit }: ProductionViewProps) {
  const job = project.currentJob;
  const active = isJobActive(job);
  const failed = failedScenes(project);
  const [regen, setRegen] = useState<Scene | null>(null);
  const failedStep = job?.steps.find((s) => s.status === "failed");
  const aspect = ASPECT[project.settings.aspectRatio] ?? "aspect-video";

  return (
    <div className="flex flex-col gap-5">
      {job && job.jobType !== "video_storyboard" && (active || job.status === "failed") ? (
        <section aria-label="Tiến độ">
          <div className="mb-1 text-small text-neutral-600">{jobTypeLabel(job.jobType)}</div>
          <JobProgress
            status={job.status}
            progress={job.progress}
            label={active ? jobProgressLabel(job) : undefined}
            errorKind={failedStep?.errorKind ?? null}
            errorDetail={failedStep ? `${stepLabel(failedStep.stepKey)}: ${failedStep.errorMessage ?? ""}` : (job.errorMessage ?? undefined)}
          />
        </section>
      ) : null}

      {failed.length > 0 && !active ? (
        <Banner tone="danger" title={`${failed.length} cảnh sinh lỗi`}>
          Các cảnh khác đã sẵn sàng và được giữ lại. Tạo lại riêng cảnh lỗi để render video.
        </Banner>
      ) : null}

      <section aria-label="Video" className="flex flex-col gap-2">
        <div className="flex items-baseline justify-between">
          <h2 className="text-section font-semibold text-neutral-900">Video</h2>
          {project.output ? (
            <span className="text-small text-neutral-600 tabular">
              Phiên bản {project.output.versionNo} · {formatDurationMs(project.output.durationMs)}
            </span>
          ) : null}
        </div>
        <div className="flex justify-center rounded bg-video-bg p-3">
          {project.output?.url ? (
            <video
              key={project.output.fileId}
              controls
              preload="metadata"
              poster={project.output.thumbnailUrl ?? undefined}
              src={project.output.url}
              aria-label="Video đã render"
              className="max-h-[480px] max-w-full rounded-sm"
            />
          ) : (
            <div className={cn("flex max-h-[360px] flex-col items-center justify-center gap-2 text-small text-video-fg", aspect, "h-[360px]")}>
              <Film className="size-5" aria-hidden />
              {project.output ? "Không có bản xem trước (liên kết hết hạn hoặc chế độ mock)." : "Chưa có video. Video xuất hiện sau khi render xong."}
            </div>
          )}
        </div>
      </section>

      <section aria-label="Timeline cảnh" className="flex flex-col gap-2">
        <h2 className="text-section font-semibold text-neutral-900">Timeline</h2>
        <TimelineBar
          durations={project.scenes.map((s) => s.durationMs)}
          maxMs={project.totalDurationMs}
          states={project.scenes.map((s) => s.status)}
        />
      </section>

      <section aria-label="Cảnh" className="flex flex-col gap-2">
        <h2 className="text-section font-semibold text-neutral-900">Cảnh</h2>
        <ul className="grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-3">
          {project.scenes.map((s) => (
            <li key={s.id} aria-label={`Cảnh ${s.sequenceNo}`} className="flex flex-col overflow-hidden rounded border border-neutral-200 bg-neutral-0">
              <div className={cn("relative flex items-center justify-center bg-neutral-100", "aspect-video")}>
                {s.thumbnailUrl ? (
                  <img src={s.thumbnailUrl} alt={`Ảnh xem trước cảnh ${s.sequenceNo}`} className="h-full w-full object-cover" />
                ) : (
                  <Film className="size-5 text-neutral-400" aria-hidden />
                )}
              </div>
              <div className="flex flex-col gap-1.5 p-2">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-small font-medium text-neutral-900 tabular">
                    Cảnh {s.sequenceNo} · {formatDurationMs(s.durationMs)}
                  </span>
                  <StatusBadge kind="scene" status={s.status} />
                </div>
                <p className="line-clamp-2 text-caption text-neutral-600">{s.visualPrompt}</p>
                {s.status === "failed" && s.lastError ? <p className="text-caption text-danger">{s.lastError}</p> : null}
                {s.previewUrl ? (
                  <a href={s.previewUrl} target="_blank" rel="noreferrer" className="text-caption text-accent-700 underline">
                    Xem clip
                  </a>
                ) : null}
                {canEdit && (s.status === "ready" || s.status === "failed") ? (
                  <Button variant={s.status === "failed" ? "secondary" : "ghost"} size="sm" disabled={active} onClick={() => setRegen(s)}>
                    <RefreshCw aria-hidden /> Tạo lại cảnh {s.sequenceNo}
                  </Button>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      </section>

      {regen ? <RegenerateDialog assetId={project.id} scene={regen} onClose={() => setRegen(null)} /> : null}
    </div>
  );
}

function RegenerateDialog({ assetId, scene, onClose }: { assetId: string; scene: Scene; onClose: () => void }) {
  const [prompt, setPrompt] = useState(scene.visualPrompt);
  const mutation = useRegenerateScene(assetId);
  const keyRef = useRef<string | null>(null);
  const changed = prompt.trim() !== scene.visualPrompt;

  function submit() {
    keyRef.current ??= newIdempotencyKey();
    mutation.mutate(
      { sceneId: scene.id, visualPrompt: changed ? prompt.trim() : null, key: keyRef.current },
      {
        onSuccess: () => {
          toast.success(`Đang tạo lại cảnh ${scene.sequenceNo}`);
          onClose();
        },
        onError: (err) => {
          if (isApiError(err) && err.status < 500) keyRef.current = null;
        },
      },
    );
  }

  return (
    <Dialog open onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Tạo lại cảnh {scene.sequenceNo}</DialogTitle>
          <DialogDescription>Chỉ sinh lại cảnh này rồi render video; các cảnh khác dùng lại clip cũ.</DialogDescription>
        </DialogHeader>
        <label className="flex flex-col gap-1 text-small text-neutral-700">
          Mô tả hình ảnh
          <Textarea value={prompt} onChange={(e) => setPrompt(e.target.value)} rows={4} maxLength={1000} aria-label="Mô tả hình ảnh mới" />
        </label>
        {mutation.error ? <Banner tone="danger">{describeError(mutation.error).message}</Banner> : null}
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            Đóng
          </Button>
          <Button variant="primary" onClick={submit} loading={mutation.isPending} disabled={!prompt.trim()}>
            Tạo lại cảnh {scene.sequenceNo}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
