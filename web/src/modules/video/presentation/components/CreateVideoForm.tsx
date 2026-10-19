import { useRef, useState, type FormEvent } from "react";
import { isApiError } from "@/shared/kernel";
import { describeError } from "@/shared/lib/errors";
import { newIdempotencyKey } from "@/shared/lib/id";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { RadioGroup, RadioGroupItem } from "@/shared/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";
import { Textarea } from "@/shared/ui/textarea";
import { FormField } from "@/shared/ui/patterns";
import { useBrandOptions, useCreateVideo, useMusicTracks } from "../../application/useVideo";
import {
  DEFAULT_LIMITS,
  MIN_TARGET_SECONDS,
  PRESETS,
  type CreateVideoInput,
  type SourceType,
  type SubtitleStyle,
  type VideoCreated,
  type VideoPreset,
} from "../../domain/video";

const SERVER_ISSUES: Record<string, string> = {
  required: "Trường này bắt buộc.",
  not_licensed: "Nhạc nền chưa có license hợp lệ. Chọn bản nhạc khác.",
  between_4_60: `Thời lượng từ ${MIN_TARGET_SECONDS} đến ${DEFAULT_LIMITS.maxTotalSeconds} giây.`,
};

export interface CreateVideoFormProps {
  campaignId: string | null;
  scriptFromAssetId: string | null;
  onCreated: (v: VideoCreated) => void;
}

/** Bước 1 — Kịch bản: nguồn, preset, thời lượng, giọng đọc/phụ đề/nhạc (ACS-071). */
export function CreateVideoForm({ campaignId, scriptFromAssetId, onCreated }: CreateVideoFormProps) {
  const [source, setSource] = useState<SourceType>(scriptFromAssetId ? "script" : "prompt");
  const [title, setTitle] = useState("");
  const [prompt, setPrompt] = useState("");
  const [scriptAsset, setScriptAsset] = useState(scriptFromAssetId ?? "");
  const [brandId, setBrandId] = useState<string | null>(null);
  const [preset, setPreset] = useState<VideoPreset>("social_vertical");
  const [duration, setDuration] = useState("20");
  const [voice, setVoice] = useState(true);
  const [subs, setSubs] = useState(true);
  const [subStyle, setSubStyle] = useState<SubtitleStyle>("burn_in");
  const [music, setMusic] = useState(false);
  const [trackId, setTrackId] = useState<string | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const keyRef = useRef<string | null>(null);

  const brands = useBrandOptions(!campaignId);
  const tracks = useMusicTracks(campaignId ? null : brandId);
  const create = useCreateVideo();

  function validate(): Record<string, string> {
    const e: Record<string, string> = {};
    if (!campaignId && !brandId) e.brand_id = "Chọn brand cho video.";
    if (source === "prompt" && !prompt.trim()) e.prompt = "Nhập mô tả video.";
    if (source === "script" && !scriptAsset.trim()) e.script_from_asset_id = "Nhập mã asset kịch bản.";
    const d = Number(duration);
    if (!Number.isFinite(d) || d < MIN_TARGET_SECONDS || d > DEFAULT_LIMITS.maxTotalSeconds) {
      e.target_duration_seconds = `Thời lượng từ ${MIN_TARGET_SECONDS} đến ${DEFAULT_LIMITS.maxTotalSeconds} giây.`;
    }
    if (music && !trackId) e["music.track_brand_asset_id"] = "Chọn bản nhạc có license.";
    return e;
  }

  function submit(ev: FormEvent) {
    ev.preventDefault();
    const e = validate();
    setErrors(e);
    if (Object.keys(e).length) return;
    keyRef.current ??= newIdempotencyKey();
    const input: CreateVideoInput = {
      campaignId,
      brandId: campaignId ? null : brandId,
      title: title.trim() || null,
      source: { type: source, fileIds: [] },
      prompt: source === "prompt" ? prompt.trim() : prompt.trim() || null,
      scriptFromAssetId: source === "script" ? scriptAsset.trim() : null,
      preset,
      targetDurationSeconds: Number(duration),
      voiceover: { enabled: voice, language: "vi", voiceId: null, mode: "per_scene" },
      subtitles: { enabled: subs, language: "vi", style: subStyle },
      music: { enabled: music, trackBrandAssetId: music ? trackId : null },
    };
    create.mutate(
      { input, key: keyRef.current },
      {
        onSuccess: (v) => {
          keyRef.current = null;
          onCreated(v);
        },
        onError: (err) => {
          if (isApiError(err) && err.details.length) {
            const fe: Record<string, string> = {};
            for (const [field, issue] of Object.entries(err.fieldErrors())) fe[field] = SERVER_ISSUES[issue] ?? issue;
            setErrors(fe);
          }
          if (isApiError(err) && err.status < 500 && err.code !== "NETWORK_ERROR") keyRef.current = null;
        },
      },
    );
  }

  const createError = create.error;

  return (
    <form onSubmit={submit} noValidate className="flex max-w-[720px] flex-col gap-5">
      {campaignId ? (
        <Banner tone="info">Video sẽ được gắn vào campaign đã chọn; brand lấy theo campaign.</Banner>
      ) : (
        <FormField label="Brand" required error={errors.brand_id}>
          <Select value={brandId ?? undefined} onValueChange={(v) => setBrandId(v)} disabled={brands.isLoading}>
            <SelectTrigger aria-label="Brand">
              <SelectValue placeholder={brands.isLoading ? "Đang tải brand…" : "Chọn brand"} />
            </SelectTrigger>
            <SelectContent>
              {(brands.data ?? []).map((b) => (
                <SelectItem key={b.id} value={b.id}>
                  {b.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FormField>
      )}

      <FormField label="Tiêu đề" helper="Để trống: hệ thống đặt theo mô tả.">
        <Input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={200} />
      </FormField>

      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1.5 text-small font-medium text-neutral-900">Nguồn</legend>
        <RadioGroup value={source} onValueChange={(v) => setSource(v as SourceType)} className="flex gap-6">
          <label className="flex items-center gap-2 text-body">
            <RadioGroupItem value="prompt" aria-label="Từ mô tả" /> Từ mô tả
          </label>
          <label className="flex items-center gap-2 text-body">
            <RadioGroupItem value="script" aria-label="Từ kịch bản Content Studio" /> Từ kịch bản Content Studio
          </label>
        </RadioGroup>
      </fieldset>

      {source === "prompt" ? (
        <FormField label="Mô tả video" required error={errors.prompt} helper="Nội dung chính, đối tượng, phong cách hình ảnh.">
          <Textarea value={prompt} onChange={(e) => setPrompt(e.target.value)} rows={4} maxLength={4000} />
        </FormField>
      ) : (
        <>
          <FormField label="Mã asset kịch bản" required error={errors.script_from_asset_id} helper="Asset nội dung loại kịch bản video.">
            <Input value={scriptAsset} onChange={(e) => setScriptAsset(e.target.value)} className="font-mono" />
          </FormField>
          <FormField label="Ghi chú thêm" helper="Tùy chọn: phong cách hình ảnh mong muốn.">
            <Textarea value={prompt} onChange={(e) => setPrompt(e.target.value)} rows={2} maxLength={4000} />
          </FormField>
        </>
      )}

      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1.5 text-small font-medium text-neutral-900">Khung hình</legend>
        <RadioGroup value={preset} onValueChange={(v) => setPreset(v as VideoPreset)} className="flex gap-6">
          {(Object.keys(PRESETS) as VideoPreset[]).map((k) => (
            <label key={k} className="flex items-center gap-2 text-body">
              <RadioGroupItem value={k} aria-label={PRESETS[k].label} /> {PRESETS[k].label}
            </label>
          ))}
        </RadioGroup>
      </fieldset>

      <FormField
        label="Thời lượng mục tiêu (giây)"
        required
        error={errors.target_duration_seconds}
        helper={`Tối đa ${DEFAULT_LIMITS.maxTotalSeconds} giây, ${DEFAULT_LIMITS.maxScenes} cảnh.`}
      >
        <Input
          type="number"
          min={MIN_TARGET_SECONDS}
          max={DEFAULT_LIMITS.maxTotalSeconds}
          value={duration}
          onChange={(e) => setDuration(e.target.value)}
          className="w-32 tabular"
        />
      </FormField>

      <div className="flex flex-col gap-3 rounded border border-neutral-200 p-4">
        <label className="flex items-center justify-between gap-4 text-body">
          Giọng đọc tiếng Việt
          <Switch checked={voice} onCheckedChange={setVoice} aria-label="Giọng đọc tiếng Việt" />
        </label>
        <label className="flex items-center justify-between gap-4 text-body">
          Phụ đề
          <Switch checked={subs} onCheckedChange={setSubs} aria-label="Phụ đề" />
        </label>
        {subs ? (
          <RadioGroup value={subStyle} onValueChange={(v) => setSubStyle(v as SubtitleStyle)} className="flex gap-6 pl-1">
            <label className="flex items-center gap-2 text-small">
              <RadioGroupItem value="burn_in" aria-label="Chèn cứng vào hình" /> Chèn cứng vào hình
            </label>
            <label className="flex items-center gap-2 text-small">
              <RadioGroupItem value="soft" aria-label="Phụ đề rời (bật/tắt được)" /> Phụ đề rời (bật/tắt được)
            </label>
          </RadioGroup>
        ) : null}
        <label className="flex items-center justify-between gap-4 text-body">
          Nhạc nền từ thư viện brand
          <Switch checked={music} onCheckedChange={setMusic} aria-label="Nhạc nền" />
        </label>
        {music ? (
          <FormField label="Bản nhạc" required error={errors["music.track_brand_asset_id"]} helper="Chỉ dùng được nhạc có license.">
            <Select value={trackId ?? undefined} onValueChange={setTrackId} disabled={!!campaignId || !brandId}>
              <SelectTrigger aria-label="Bản nhạc">
                <SelectValue placeholder={campaignId || !brandId ? "Chọn brand trước" : "Chọn bản nhạc"} />
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
          </FormField>
        ) : null}
      </div>

      {createError && !(isApiError(createError) && createError.code === "VALIDATION_FAILED") ? (
        <Banner tone="danger">{describeError(createError).message}</Banner>
      ) : null}

      <div>
        <Button type="submit" variant="primary" loading={create.isPending}>
          Tạo storyboard
        </Button>
      </div>
    </form>
  );
}
