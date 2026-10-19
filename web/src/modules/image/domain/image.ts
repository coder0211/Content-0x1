import type { ApiErrorDetail, AssetStatus, ID, ISODateTime, JobStatus, Money } from "@/shared/kernel";

/** FR-IMG-01 presets, in UI order. */
export const PRESETS = ["social_post", "poster", "product_photo", "thumbnail"] as const;
export type Preset = (typeof PRESETS)[number];

export const PRESET_LABELS: Record<Preset, string> = {
  social_post: "Bài đăng mạng xã hội",
  poster: "Poster",
  product_photo: "Ảnh sản phẩm",
  thumbnail: "Thumbnail",
};

export const ASPECT_RATIOS = ["1:1", "4:5", "9:16", "16:9"] as const;
export type AspectRatio = (typeof ASPECT_RATIOS)[number];

/** Default ratio per preset (the user may change it). */
export const PRESET_DEFAULT_RATIO: Record<Preset, AspectRatio> = {
  social_post: "1:1",
  poster: "4:5",
  product_photo: "1:1",
  thumbnail: "16:9",
};

export const QUALITIES = ["standard", "high"] as const;
export type Quality = (typeof QUALITIES)[number];
export const QUALITY_LABELS: Record<Quality, string> = { standard: "Tiêu chuẩn", high: "Cao" };

export const MIN_COUNT = 1;
export const MAX_COUNT = 4;
export const MAX_PROMPT_LENGTH = 4000;
export const MAX_REFERENCES = 4;
export const MAX_REFERENCE_BYTES = 20 * 1024 * 1024;
export const REFERENCE_MIME_TYPES = ["image/png", "image/jpeg", "image/webp"] as const;

export type Operation = "generate" | "edit" | "variation";

export interface Cost extends Money {
  isEstimate: boolean;
}

export interface ImageFile {
  fileId: ID;
  role: "primary" | "variant";
  sortOrder: number;
  mimeType: string;
  width: number;
  height: number;
  parentFileId: ID | null;
  url: string;
  thumbnailUrl: string;
}

export interface BatchAsset {
  id: ID;
  title: string;
  status: AssetStatus;
  versionNo: number;
  currentVersionNo: number;
  requiresTypographyCheck: boolean;
  images: ImageFile[];
}

export interface BatchJob {
  id: ID;
  status: JobStatus;
  progress: number;
  errorCode: string | null;
  errorKind: string | null;
  errorMessage: string | null;
  costEstimate: Cost | null;
  costActual: Cost | null;
}

/** One gallery entry ("đợt tạo"). */
export interface ImageBatch {
  id: ID;
  operation: Operation;
  campaignId: ID | null;
  brandId: ID;
  prompt: string;
  preset: Preset;
  aspectRatio: string;
  quality: string;
  count: number;
  modelCode: string;
  sourceFileId: ID | null;
  createdAt: ISODateTime;
  job: BatchJob;
  asset: BatchAsset | null;
}

export interface ImageEstimate {
  total: Cost;
  perJobLimit: Money;
  campaignRemaining: Money | null;
  globalRemaining: Money;
  requiresOverride: boolean;
  blocked: boolean;
  pricingVersion: string;
}

export type ReferenceSource = "brand" | "library" | "upload";

export interface ReferenceImage {
  fileId: ID;
  source: ReferenceSource;
  label: string;
  thumbnailUrl: string;
}

/** Where generated images go: a campaign, or a brand for an independent task. */
export type StudioTarget = { campaignId: ID; brandId?: ID } | { campaignId?: undefined; brandId: ID };

export interface GenerationOptions {
  prompt: string;
  preset: Preset;
  aspectRatio: AspectRatio;
  quality: Quality;
  count: number;
  referenceFileIds: ID[];
  budgetOverride: boolean;
}

export const defaultOptions = (preset: Preset = "social_post"): GenerationOptions => ({
  prompt: "",
  preset,
  aspectRatio: PRESET_DEFAULT_RATIO[preset],
  quality: "standard",
  count: 1,
  referenceFileIds: [],
  budgetOverride: false,
});

export type FieldErrors = Partial<Record<"prompt" | "count" | "reference_file_ids" | "aspect_ratio" | "quality" | "target", string>>;

/** Client-side validation mirroring the backend's 400 rules. */
export function validateOptions(o: GenerationOptions, hasTarget: boolean): FieldErrors {
  const errors: FieldErrors = {};
  const prompt = o.prompt.trim();
  if (!prompt) errors.prompt = "Nhập mô tả ảnh.";
  else if (prompt.length > MAX_PROMPT_LENGTH) errors.prompt = `Mô tả tối đa ${MAX_PROMPT_LENGTH} ký tự.`;
  if (!Number.isInteger(o.count) || o.count < MIN_COUNT || o.count > MAX_COUNT) errors.count = `Số ảnh từ ${MIN_COUNT} đến ${MAX_COUNT}.`;
  if (o.referenceFileIds.length > MAX_REFERENCES) errors.reference_file_ids = `Tối đa ${MAX_REFERENCES} ảnh tham chiếu.`;
  if (!hasTarget) errors.target = "Chọn campaign hoặc brand.";
  return errors;
}

/**
 * Maps `422 CAPABILITY_NOT_SUPPORTED` details (`[{field, supported}]`) to field messages.
 * The backend never changes options silently, so the user must pick a supported value.
 */
export function capabilityErrors(details: ApiErrorDetail[]): FieldErrors {
  const out: FieldErrors = {};
  for (const d of details) {
    const field = d.field as keyof FieldErrors | undefined;
    if (!field) continue;
    const supported = Array.isArray(d.supported) ? (d.supported as unknown[]).map(String) : [];
    if (field === ("file_id" as string) || field === "reference_file_ids") {
      out.reference_file_ids = "Model hiện tại không hỗ trợ ảnh tham chiếu.";
    } else if (supported.length > 0) {
      out[field] = `Model không hỗ trợ giá trị này. Giá trị hợp lệ: ${supported.join(", ")}.`;
    } else {
      out[field] = "Model không hỗ trợ giá trị này.";
    }
  }
  return out;
}

export const isBatchActive = (b: ImageBatch): boolean => b.job.status === "queued" || b.job.status === "running";

/** CSS aspect-ratio value for "4:5" → "4 / 5". */
export function cssAspect(ratio: string): string {
  const [w, h] = ratio.split(":").map(Number);
  return w && h ? `${w} / ${h}` : "1 / 1";
}

const OPERATION_LABELS: Record<Operation, string> = {
  generate: "Sinh ảnh",
  edit: "Chỉnh sửa bằng prompt",
  variation: "Tạo biến thể",
};
export const operationLabel = (op: Operation): string => OPERATION_LABELS[op];

/** Step text for JobProgress, from the job progress the worker reports. */
export function progressLabel(b: ImageBatch): string {
  if (b.job.status === "queued") return "Đang chờ xử lý";
  const p = b.job.progress;
  if (p < 25) return "Đang chuẩn bị ảnh đầu vào";
  if (p < 70) return b.count > 1 ? `Đang sinh ${b.count} ảnh` : "Đang sinh ảnh";
  if (p < 90) return "Đang lưu ảnh";
  return "Đang tạo phiên bản asset";
}

/** FR-IMG-06: a poster needs the "đã kiểm tra chữ" checkbox before review. */
export const needsTypographyCheck = (asset: Pick<BatchAsset, "requiresTypographyCheck">): boolean => asset.requiresTypographyCheck;

/** Only draft / changes_requested assets can be (re)submitted for review. */
export const canSubmitForReview = (status: AssetStatus): boolean => status === "draft" || status === "changes_requested";

export function validateReferenceFile(file: { type: string; size: number }): string | null {
  if (!(REFERENCE_MIME_TYPES as readonly string[]).includes(file.type)) return "Chỉ nhận ảnh PNG, JPEG hoặc WebP.";
  if (file.size > MAX_REFERENCE_BYTES) return "Ảnh tham chiếu tối đa 20 MB.";
  return null;
}
