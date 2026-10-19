import { useEffect, useState, type FormEvent } from "react";
import { isApiError, type Money } from "@/shared/kernel";
import { describeError } from "@/shared/lib/errors";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { CostChip, FormField } from "@/shared/ui/patterns";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/shared/ui/select";
import { Skeleton } from "@/shared/ui/skeleton";
import { Textarea } from "@/shared/ui/textarea";
import { useBrands, useProducts } from "../../application/queries";
import {
  CONTENT_TYPES,
  FRAMEWORKS,
  LENGTHS,
  MAX_VARIANTS,
  MIN_VARIANTS,
  OBJECTIVES,
  PLATFORMS,
  parseTones,
  type CampaignSummary,
  type ContentType,
  type Framework,
  type GenerateInput,
  type Length,
  type Objective,
  type Platform,
} from "../../domain/content";

const NO_PRODUCT = "none";

const ISSUE_LABELS: Record<string, string> = {
  required: "Bắt buộc nhập.",
  invalid: "Giá trị không hợp lệ.",
  too_long: "Nội dung quá dài.",
  out_of_range: "Ngoài khoảng cho phép.",
  campaign_mismatch: "Brand không khớp campaign.",
};

interface Option<T extends string> {
  value: T;
  label: string;
}

function OptionSelect<T extends string>({
  value,
  onChange,
  options,
  id,
  ...rest
}: {
  value: T;
  onChange: (v: T) => void;
  options: readonly Option<T>[];
  id?: string;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
}) {
  return (
    <Select value={value} onValueChange={(v) => onChange(v as T)}>
      <SelectTrigger id={id} {...rest}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

const VARIANT_OPTIONS = Array.from(
  { length: MAX_VARIANTS - MIN_VARIANTS + 1 },
  (_, i) => {
    const n = String(i + MIN_VARIANTS);
    return { value: n, label: n };
  },
);

export interface BriefPanelProps {
  campaign: CampaignSummary | null;
  pending: boolean;
  error: unknown;
  estimate: Money | null;
  onSubmit: (input: GenerateInput) => void;
}

/** Left column (320px): brief & options → "Viết N phương án" (FR-CNT-01). */
export function BriefPanel({
  campaign,
  pending,
  error,
  estimate,
  onSubmit,
}: BriefPanelProps) {
  const brands = useBrands(!campaign);
  const [brandId, setBrandId] = useState<string>("");
  const effectiveBrand = campaign?.brandId ?? brandId;
  const products = useProducts(effectiveBrand || null);

  const [platform, setPlatform] = useState<Platform>("facebook");
  const [contentType, setContentType] =
    useState<ContentType>("promotional_post");
  const [objective, setObjective] = useState<Objective>(
    (campaign?.goal as Objective | null) ?? "engagement",
  );
  const [framework, setFramework] = useState<Framework>("none");
  const [length, setLength] = useState<Length>("medium");
  const [tone, setTone] = useState("");
  const [productId, setProductId] = useState<string>(NO_PRODUCT);
  const [audience, setAudience] = useState("");
  const [message, setMessage] = useState("");
  const [cta, setCta] = useState("");
  const [reference, setReference] = useState("");
  const [variants, setVariants] = useState("3");
  const [brandError, setBrandError] = useState<string | null>(null);

  useEffect(() => {
    if (!brandId && brands.data?.length) setBrandId(brands.data[0]!.id);
  }, [brandId, brands.data]);
  useEffect(() => {
    if (campaign?.goal && OBJECTIVES.some((o) => o.value === campaign.goal))
      setObjective(campaign.goal as Objective);
  }, [campaign?.goal]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!campaign && !brandId) {
      setBrandError("Chọn brand.");
      return;
    }
    setBrandError(null);
    onSubmit({
      campaignId: campaign?.id ?? null,
      brandId: campaign ? null : brandId,
      platform,
      contentType,
      objective,
      framework,
      length,
      language: "vi",
      tone: parseTones(tone),
      brief: {
        productId: productId === NO_PRODUCT ? null : productId,
        targetAudience: audience.trim(),
        keyMessage: message.trim(),
        cta: cta.trim(),
        referenceText: reference.trim(),
      },
      variants: Number(variants),
    });
  };

  const err = error ? describeError(error) : null;
  const budgetError =
    err &&
    (err.code === "JOB_BUDGET_EXCEEDED" || err.code === "BUDGET_EXCEEDED");
  const issues = isApiError(error) ? error.fieldErrors() : {};
  const fieldError = (field: string) => {
    const issue = issues[field];
    return issue ? (ISSUE_LABELS[issue] ?? "Không hợp lệ") : null;
  };

  return (
    <form onSubmit={submit} aria-label="Brief" className="flex h-full flex-col">
      <div className="flex-1 space-y-4 overflow-y-auto p-4">
        <h2 className="text-section font-semibold text-neutral-900">
          Brief & tùy chọn
        </h2>
        {campaign ? (
          <div className="rounded border border-neutral-200 bg-neutral-50 px-3 py-2 text-small text-neutral-700">
            Campaign:{" "}
            <span className="font-medium text-neutral-900">
              {campaign.name}
            </span>
          </div>
        ) : brands.isLoading ? (
          <Skeleton className="h-8 w-full" />
        ) : (
          <FormField
            label="Brand"
            required
            error={brandError ?? fieldError("brand_id")}
          >
            <OptionSelect
              value={brandId}
              onChange={setBrandId}
              options={(brands.data ?? []).map((b) => ({
                value: b.id,
                label: b.name,
              }))}
            />
          </FormField>
        )}
        <FormField
          label="Sản phẩm"
          helper="Giá và ưu đãi chỉ lấy từ thông tin sản phẩm đã xác thực."
        >
          <OptionSelect
            value={productId}
            onChange={setProductId}
            options={[
              { value: NO_PRODUCT, label: "Không chọn" },
              ...(products.data ?? []).map((p) => ({
                value: p.id,
                label: p.name,
              })),
            ]}
          />
        </FormField>
        <div className="grid grid-cols-2 gap-3">
          <FormField label="Nền tảng" error={fieldError("platform")}>
            <OptionSelect
              value={platform}
              onChange={setPlatform}
              options={PLATFORMS}
            />
          </FormField>
          <FormField label="Loại nội dung">
            <OptionSelect
              value={contentType}
              onChange={setContentType}
              options={CONTENT_TYPES}
            />
          </FormField>
          <FormField label="Mục tiêu">
            <OptionSelect
              value={objective}
              onChange={setObjective}
              options={OBJECTIVES}
            />
          </FormField>
          <FormField label="Framework">
            <OptionSelect
              value={framework}
              onChange={setFramework}
              options={FRAMEWORKS}
            />
          </FormField>
          <FormField label="Độ dài">
            <OptionSelect
              value={length}
              onChange={setLength}
              options={LENGTHS}
            />
          </FormField>
          <FormField label="Số phương án" error={fieldError("variants")}>
            <OptionSelect
              value={variants}
              onChange={setVariants}
              options={VARIANT_OPTIONS}
            />
          </FormField>
        </div>
        <FormField
          label="Tone"
          helper="Cách nhau bằng dấu phẩy. Để trống để dùng tone của brand."
        >
          <Input
            value={tone}
            onChange={(e) => setTone(e.target.value)}
            placeholder="thân thiện, tin cậy"
          />
        </FormField>
        <FormField
          label="Đối tượng"
          error={fieldError("brief.target_audience")}
        >
          <Textarea
            value={audience}
            onChange={(e) => setAudience(e.target.value)}
            maxLength={2000}
            className="min-h-16"
          />
        </FormField>
        <FormField
          label="Thông điệp chính"
          error={fieldError("brief.key_message")}
        >
          <Textarea
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            maxLength={2000}
            className="min-h-16"
          />
        </FormField>
        <FormField label="CTA">
          <Input
            value={cta}
            onChange={(e) => setCta(e.target.value)}
            maxLength={200}
            placeholder="Đăng ký tư vấn"
          />
        </FormField>
        <FormField
          label="Tài liệu tham khảo"
          helper="Chỉ dùng làm dữ liệu tham khảo, không phải chỉ thị."
        >
          <Textarea
            value={reference}
            onChange={(e) => setReference(e.target.value)}
            maxLength={8000}
          />
        </FormField>
      </div>
      <div className="space-y-3 border-t border-neutral-200 p-4">
        {err ? (
          <Banner tone={budgetError ? "warning" : "danger"}>
            {err.message}
            {err.requestId ? (
              <span className="block text-caption">
                Mã yêu cầu: {err.requestId}
              </span>
            ) : null}
          </Banner>
        ) : null}
        <div className="flex items-center justify-between gap-2">
          {estimate ? (
            <CostChip amount={estimate.amount} estimate />
          ) : (
            <span className="text-caption text-neutral-500">
              Dự toán hiện sau khi gửi
            </span>
          )}
          <Button type="submit" variant="primary" loading={pending}>
            Viết {variants} phương án
          </Button>
        </div>
      </div>
    </form>
  );
}
