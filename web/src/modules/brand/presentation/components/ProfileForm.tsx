import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { isApiError } from "@/shared/kernel";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { FormField } from "@/shared/ui/patterns";
import { Textarea } from "@/shared/ui/textarea";
import { toast } from "@/shared/ui/toast";
import { useUpdateBrand } from "../../application/useBrands";
import { issueMessage, isHexColor, validateProfile } from "../../domain/rules";
import type { Brand, BrandProfile } from "../../domain/types";
import { LinesField } from "./LinesField";

interface Props {
  brand: Brand;
  readOnly: boolean;
  onReload: () => void;
}

/** Brand profile editor. Each save creates a new version guarded by expected_version_no (ACS-020/021). */
export function ProfileForm({ brand, readOnly, onReload }: Props) {
  const [name, setName] = useState(brand.name);
  const [profile, setProfile] = useState<BrandProfile>(brand.currentVersion.profile);
  const [baseVersion, setBaseVersion] = useState(brand.currentVersion.versionNo);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [conflict, setConflict] = useState<number | null>(null);
  const update = useUpdateBrand(brand.id);

  useEffect(() => {
    setName(brand.name);
    setProfile(brand.currentVersion.profile);
    setBaseVersion(brand.currentVersion.versionNo);
  }, [brand]);

  const set = <K extends keyof BrandProfile>(key: K, v: BrandProfile[K]) => setProfile((p) => ({ ...p, [key]: v }));
  const err = (field: string) => issueMessage(errors[field]);

  const save = async () => {
    const local = validateProfile(profile);
    if (!name.trim()) local.name = "required";
    setErrors(local);
    if (Object.keys(local).length) return;
    setConflict(null);
    try {
      const b = await update.mutateAsync({ name: name.trim(), profile, expectedVersionNo: baseVersion });
      toast.success(b.currentVersion.versionNo > baseVersion ? `Đã lưu phiên bản ${b.currentVersion.versionNo}` : "Đã lưu thay đổi");
    } catch (e) {
      if (isApiError(e) && e.code === "VERSION_CONFLICT") {
        const current = e.details.find((d) => typeof d.current_version_no === "number")?.current_version_no as number | undefined;
        setConflict(current ?? baseVersion + 1);
      } else if (isApiError(e) && e.code === "VALIDATION_FAILED") {
        setErrors(e.fieldErrors());
      } else {
        toast.error(e);
      }
    }
  };

  return (
    <form
      className="flex max-w-3xl flex-col gap-6"
      aria-label="Hồ sơ brand"
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      {readOnly ? <Banner tone="info">Bạn chỉ có quyền xem hồ sơ brand.</Banner> : null}
      {conflict !== null ? (
        <Banner
          tone="warning"
          title="Brand vừa được người khác cập nhật"
          action={
            <Button
              type="button"
              size="sm"
              onClick={() => {
                setConflict(null);
                onReload();
              }}
            >
              Tải lại phiên bản mới
            </Button>
          }
        >
          Phiên bản hiện tại là {conflict}. Thay đổi của bạn chưa được lưu.
        </Banner>
      ) : null}

      <fieldset disabled={readOnly} className="flex flex-col gap-4">
        <FormField label="Tên brand" required error={err("name")}>
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={200} />
        </FormField>
        <FormField label="Mô tả">
          <Textarea rows={3} value={profile.description} onChange={(e) => set("description", e.target.value)} />
        </FormField>

        <section aria-labelledby="colors-h" className="flex flex-col gap-2">
          <h3 id="colors-h" className="text-section text-neutral-900">
            Màu thương hiệu
          </h3>
          {profile.colors.map((c, i) => (
            <div key={i} className="flex items-start gap-2">
              <span aria-hidden className="mt-1.5 size-5 shrink-0 rounded-sm border border-neutral-200" style={{ backgroundColor: isHexColor(c.hex) ? c.hex : "transparent" }} />
              <FormField label={`Tên màu ${i + 1}`} error={err(`profile.colors[${i}].name`)} className="flex-1">
                <Input value={c.name} onChange={(e) => set("colors", profile.colors.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} />
              </FormField>
              <FormField label={`Mã hex ${i + 1}`} error={err(`profile.colors[${i}].hex`)} className="w-40">
                <Input className="font-mono" value={c.hex} placeholder="#0055FF" onChange={(e) => set("colors", profile.colors.map((x, j) => (j === i ? { ...x, hex: e.target.value } : x)))} />
              </FormField>
              {!readOnly ? (
                <Button type="button" variant="ghost" size="icon" className="mt-6" aria-label={`Xóa màu ${i + 1}`} onClick={() => set("colors", profile.colors.filter((_, j) => j !== i))}>
                  <Trash2 />
                </Button>
              ) : null}
            </div>
          ))}
          {!readOnly ? (
            <Button type="button" size="sm" className="self-start" onClick={() => set("colors", [...profile.colors, { name: "", hex: "" }])}>
              <Plus /> Thêm màu
            </Button>
          ) : null}
        </section>

        <div className="grid grid-cols-2 gap-4">
          <FormField label="Font tiêu đề">
            <Input value={profile.typography.heading} onChange={(e) => set("typography", { ...profile.typography, heading: e.target.value })} />
          </FormField>
          <FormField label="Font nội dung">
            <Input value={profile.typography.body} onChange={(e) => set("typography", { ...profile.typography, body: e.target.value })} />
          </FormField>
        </div>

        <FormField label="Tone of voice" helper="Mỗi dòng một tính từ, ví dụ: thân thiện.">
          <LinesField value={profile.toneOfVoice} onChange={(v) => set("toneOfVoice", v)} />
        </FormField>
        <div className="grid grid-cols-2 gap-4">
          <FormField label="Từ nên dùng" helper="Mỗi dòng một từ hoặc cụm từ." error={err("profile.preferred_words")}>
            <LinesField value={profile.preferredWords} onChange={(v) => set("preferredWords", v)} />
          </FormField>
          <FormField label="Từ cấm dùng" helper="Được đưa vào prompt và kiểm tra sau khi sinh.">
            <LinesField value={profile.forbiddenWords} onChange={(v) => set("forbiddenWords", v)} />
          </FormField>
        </div>
        <FormField label="CTA mặc định" helper="Mỗi dòng một CTA.">
          <LinesField value={profile.defaultCtas} onChange={(v) => set("defaultCtas", v)} rows={2} />
        </FormField>

        <section aria-labelledby="personas-h" className="flex flex-col gap-3">
          <h3 id="personas-h" className="text-section text-neutral-900">
            Chân dung khách hàng
          </h3>
          {profile.audiencePersonas.map((p, i) => {
            const patch = (v: Partial<typeof p>) => set("audiencePersonas", profile.audiencePersonas.map((x, j) => (j === i ? { ...x, ...v } : x)));
            return (
              <div key={i} className="flex flex-col gap-3 rounded border border-neutral-200 p-3">
                <div className="flex items-start gap-2">
                  <FormField label={`Tên persona ${i + 1}`} required error={err(`profile.audience_personas[${i}].name`)} className="flex-1">
                    <Input value={p.name} onChange={(e) => patch({ name: e.target.value })} />
                  </FormField>
                  {!readOnly ? (
                    <Button type="button" variant="ghost" size="icon" className="mt-6" aria-label={`Xóa persona ${i + 1}`} onClick={() => set("audiencePersonas", profile.audiencePersonas.filter((_, j) => j !== i))}>
                      <Trash2 />
                    </Button>
                  ) : null}
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <FormField label="Nỗi đau">
                    <LinesField value={p.painPoints} onChange={(v) => patch({ painPoints: v })} rows={2} />
                  </FormField>
                  <FormField label="Động lực">
                    <LinesField value={p.motivations} onChange={(v) => patch({ motivations: v })} rows={2} />
                  </FormField>
                </div>
              </div>
            );
          })}
          {!readOnly ? (
            <Button type="button" size="sm" className="self-start" onClick={() => set("audiencePersonas", [...profile.audiencePersonas, { name: "", painPoints: [], motivations: [] }])}>
              <Plus /> Thêm persona
            </Button>
          ) : null}
        </section>

        <FormField label="Ghi chú">
          <Textarea rows={2} value={profile.notes} onChange={(e) => set("notes", e.target.value)} />
        </FormField>
      </fieldset>

      {!readOnly ? (
        <div className="flex items-center gap-3">
          <Button type="submit" variant="primary" loading={update.isPending}>
            Lưu phiên bản mới
          </Button>
          <span className="text-small text-neutral-500">Đang sửa từ phiên bản {baseVersion}</span>
        </div>
      ) : null}
    </form>
  );
}
