import { formatDateTime, formatDuration } from "@/shared/lib/format";
import { StatusBadge } from "@/shared/ui/patterns";
import { PRESETS, stageLabel, type VideoProject } from "../../domain/video";

function onOff(v: boolean) {
  return v ? "Bật" : "Tắt";
}

/** Cột trái 320px: thông số dự án (preset, thời lượng, giọng đọc/phụ đề/nhạc, kịch bản). */
export function BriefPanel({ project }: { project: VideoProject }) {
  const s = project.settings;
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <h2 className="text-section font-semibold text-neutral-900">Thông số</h2>
        <div className="flex items-center gap-2">
          <StatusBadge kind="asset" status={project.status} />
          <span className="text-caption text-neutral-500">{stageLabel(project.stage)}</span>
        </div>
      </div>
      <dl className="grid grid-cols-[110px_1fr] gap-x-3 gap-y-1.5 text-small">
        <dt className="text-neutral-500">Khung hình</dt>
        <dd className="text-neutral-900">{PRESETS[s.preset]?.label ?? s.aspectRatio}</dd>
        <dt className="text-neutral-500">Mục tiêu</dt>
        <dd className="text-neutral-900 tabular">{formatDuration(s.targetDurationSeconds)}</dd>
        <dt className="text-neutral-500">Giọng đọc</dt>
        <dd className="text-neutral-900">{onOff(s.voiceover.enabled)}</dd>
        <dt className="text-neutral-500">Phụ đề</dt>
        <dd className="text-neutral-900">{s.subtitles.enabled ? (s.subtitles.style === "burn_in" ? "Chèn cứng" : "Phụ đề rời") : "Tắt"}</dd>
        <dt className="text-neutral-500">Nhạc nền</dt>
        <dd className="text-neutral-900">{onOff(s.music.enabled)}</dd>
        <dt className="text-neutral-500">Cập nhật</dt>
        <dd className="text-neutral-900 tabular">{formatDateTime(project.updatedAt)}</dd>
      </dl>
      {project.prompt ? (
        <div className="flex flex-col gap-1">
          <h3 className="text-small font-medium text-neutral-900">Mô tả</h3>
          <p className="whitespace-pre-line text-small text-neutral-700">{project.prompt}</p>
        </div>
      ) : null}
      {project.script ? (
        <div className="flex flex-col gap-1">
          <h3 className="text-small font-medium text-neutral-900">Kịch bản</h3>
          <p className="max-h-[320px] overflow-auto whitespace-pre-line text-small text-neutral-700">{project.script}</p>
        </div>
      ) : null}
    </div>
  );
}
