import { useRef, useState } from "react";
import { isApiError } from "@/shared/kernel";
import { describeError } from "@/shared/lib/errors";
import { newIdempotencyKey } from "@/shared/lib/id";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { RadioGroup, RadioGroupItem } from "@/shared/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";
import { toast } from "@/shared/ui/toast";
import { StatusBadge } from "@/shared/ui/patterns";
import { useMusicTracks, useRerender, useSubmitForReview } from "../../application/useVideo";
import { canRerender, canSubmitForReview, type SubtitleStyle, type VideoProject } from "../../domain/video";

/** Inspector khi sản xuất: render lại (phụ đề, nhạc) và gửi duyệt (ACS-068, ACS-072). */
export function RenderPanel({ project, canEdit }: { project: VideoProject; canEdit: boolean }) {
  const s = project.settings;
  const [subs, setSubs] = useState(s.subtitles.enabled);
  const [style, setStyle] = useState<SubtitleStyle>(s.subtitles.style);
  const [music, setMusic] = useState(s.music.enabled);
  const [trackId, setTrackId] = useState<string | null>(s.music.trackBrandAssetId);
  const tracks = useMusicTracks(canEdit ? project.brandId : null);
  const rerender = useRerender(project.id);
  const submit = useSubmitForReview(project.id);
  const keyRef = useRef<string | null>(null);

  const changed = subs !== s.subtitles.enabled || style !== s.subtitles.style || music !== s.music.enabled || (music && trackId !== s.music.trackBrandAssetId);
  const rerenderable = canRerender(project);
  const musicMissing = music && !trackId;

  function onRender() {
    keyRef.current ??= newIdempotencyKey();
    rerender.mutate(
      {
        opts: {
          subtitles: { enabled: subs, language: s.subtitles.language || "vi", style },
          music: { enabled: music, trackBrandAssetId: music ? trackId : null },
        },
        key: keyRef.current,
      },
      {
        onSuccess: () => {
          keyRef.current = null;
          toast.success("Đang render lại video");
        },
        onError: (err) => {
          if (isApiError(err) && err.status < 500) keyRef.current = null;
        },
      },
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <section aria-label="Trạng thái duyệt" className="flex flex-col gap-2">
        <h2 className="text-section font-semibold text-neutral-900">Duyệt</h2>
        <div className="flex items-center gap-2 text-small text-neutral-700">
          Trạng thái <StatusBadge kind="asset" status={project.status} />
        </div>
        {canEdit ? (
          <Button
            variant="primary"
            disabled={!canSubmitForReview(project)}
            loading={submit.isPending}
            onClick={() => submit.mutate(undefined, { onSuccess: () => toast.success("Đã gửi duyệt video") })}
          >
            Gửi duyệt
          </Button>
        ) : null}
        {canEdit && !project.output ? <p className="text-caption text-neutral-500">Cần render xong video trước khi gửi duyệt.</p> : null}
      </section>

      {canEdit ? (
        <section aria-label="Render lại" className="flex flex-col gap-3">
          <h2 className="text-section font-semibold text-neutral-900">Render lại</h2>
          <p className="text-caption text-neutral-500">Đổi phụ đề hoặc nhạc nền rồi render lại; không sinh lại cảnh.</p>
          <label className="flex items-center justify-between gap-3 text-small">
            Phụ đề
            <Switch checked={subs} onCheckedChange={setSubs} aria-label="Phụ đề khi render lại" />
          </label>
          {subs ? (
            <RadioGroup value={style} onValueChange={(v) => setStyle(v as SubtitleStyle)} className="flex flex-col gap-2">
              <label className="flex items-center gap-2 text-small">
                <RadioGroupItem value="burn_in" aria-label="Chèn cứng vào hình" /> Chèn cứng vào hình
              </label>
              <label className="flex items-center gap-2 text-small">
                <RadioGroupItem value="soft" aria-label="Phụ đề rời" /> Phụ đề rời
              </label>
            </RadioGroup>
          ) : null}
          <label className="flex items-center justify-between gap-3 text-small">
            Nhạc nền
            <Switch checked={music} onCheckedChange={setMusic} aria-label="Nhạc nền khi render lại" />
          </label>
          {music ? (
            <Select value={trackId ?? undefined} onValueChange={setTrackId}>
              <SelectTrigger aria-label="Bản nhạc khi render lại">
                <SelectValue placeholder={tracks.isLoading ? "Đang tải nhạc…" : "Chọn bản nhạc có license"} />
              </SelectTrigger>
              <SelectContent>
                {(tracks.data ?? []).map((t) => (
                  <SelectItem key={t.id} value={t.id} disabled={!t.licensed}>
                    {t.label}
                    {t.licensed ? "" : " (chưa có license)"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : null}
          {rerender.error ? <Banner tone="danger">{describeError(rerender.error).message}</Banner> : null}
          <Button variant="secondary" onClick={onRender} loading={rerender.isPending} disabled={!rerenderable || musicMissing}>
            {changed ? "Lưu và render lại" : "Render lại video"}
          </Button>
          {!rerenderable ? <p className="text-caption text-neutral-500">Cần mọi cảnh sẵn sàng và không có job đang chạy.</p> : null}
        </section>
      ) : null}
    </div>
  );
}
