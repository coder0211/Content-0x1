/** Content Studio domain: options, content version shape, editor helpers (pure, no React). */
import type {
  AssetStatus,
  ID,
  ISODateTime,
  JobStatus,
  Money,
} from "@/shared/kernel";

export const PLATFORMS = [
  { value: "facebook", label: "Facebook" },
  { value: "instagram", label: "Instagram" },
  { value: "tiktok", label: "TikTok" },
  { value: "zalo", label: "Zalo" },
  { value: "email", label: "Email" },
] as const;
export type Platform = (typeof PLATFORMS)[number]["value"];

export const FORMATS = [
  { value: "facebook_post", label: "Bài Facebook" },
  { value: "tiktok_script", label: "Kịch bản TikTok" },
  { value: "caption", label: "Caption" },
  { value: "email", label: "Email" },
  { value: "video_script", label: "Kịch bản video" },
] as const;
export type ContentFormat = (typeof FORMATS)[number]["value"];

/** Formats an existing post can be converted to (FR-CNT-03, FR-CNT-05). */
export const TRANSFORM_TARGETS: readonly ContentFormat[] = [
  "tiktok_script",
  "caption",
  "email",
  "video_script",
];

export const CONTENT_TYPES = [
  { value: "promotional_post", label: "Quảng bá" },
  { value: "educational_post", label: "Chia sẻ kiến thức" },
  { value: "engagement_post", label: "Tương tác" },
  { value: "announcement", label: "Thông báo" },
  { value: "testimonial", label: "Cảm nhận học viên" },
] as const;
export type ContentType = (typeof CONTENT_TYPES)[number]["value"];

export const FRAMEWORKS = [
  { value: "none", label: "Tự do" },
  { value: "AIDA", label: "AIDA" },
  { value: "PAS", label: "PAS" },
  { value: "Storytelling", label: "Storytelling" },
  { value: "SEO", label: "SEO" },
] as const;
export type Framework = (typeof FRAMEWORKS)[number]["value"];

export const LENGTHS = [
  { value: "short", label: "Ngắn" },
  { value: "medium", label: "Vừa" },
  { value: "long", label: "Dài" },
] as const;
export type Length = (typeof LENGTHS)[number]["value"];

export const OBJECTIVES = [
  { value: "awareness", label: "Nhận biết" },
  { value: "engagement", label: "Tương tác" },
  { value: "lead", label: "Thu lead" },
  { value: "conversion", label: "Chuyển đổi" },
] as const;
export type Objective = (typeof OBJECTIVES)[number]["value"];

export const REWRITES = [
  { value: "shorten", label: "Rút gọn" },
  { value: "expand", label: "Mở rộng" },
  { value: "change_tone", label: "Đổi tone" },
  { value: "custom", label: "Yêu cầu khác" },
] as const;
export type RewriteInstruction = (typeof REWRITES)[number]["value"];

export const MIN_VARIANTS = 1;
export const MAX_VARIANTS = 5;

export function labelOf<T extends string>(
  options: readonly { value: T; label: string }[],
  value: T | string | null | undefined,
): string {
  return options.find((o) => o.value === value)?.label ?? value ?? "";
}

/** Default output format of a platform (mirrors the backend). */
export function defaultFormat(p: Platform): ContentFormat {
  switch (p) {
    case "tiktok":
      return "tiktok_script";
    case "instagram":
    case "zalo":
      return "caption";
    case "email":
      return "email";
    default:
      return "facebook_post";
  }
}

export interface GenerateInput {
  campaignId: ID | null;
  brandId: ID | null;
  platform: Platform;
  contentType: ContentType;
  objective: Objective | null;
  framework: Framework;
  length: Length;
  language: "vi" | "en";
  tone: string[];
  brief: {
    productId: ID | null;
    targetAudience: string;
    keyMessage: string;
    cta: string;
    referenceText: string;
  };
  variants: number;
}

export interface RewriteInput {
  instruction: RewriteInstruction;
  tone?: string[];
  customPrompt?: string;
}

export interface JobAccepted {
  jobId: ID;
  status: JobStatus;
  estimatedCost: Money | null;
}

export interface JobSnapshot {
  id: ID;
  type: string;
  status: JobStatus;
  progress: number;
  assetIds: ID[];
  errorKind: string | null;
  errorMessage: string | null;
  costEstimate: number | null;
  costActual: number | null;
}

export type WarningType = "forbidden_word" | "unverified_claim";
export interface ContentWarning {
  type: WarningType;
  text: string;
}

export interface Scene {
  narration: string;
  visualHint: string;
  durationSec: number;
}

export interface Provenance {
  jobId: ID | null;
  providerCode: string;
  modelCode: string;
  promptTemplateCode: string;
  promptTemplateVersion: number | null;
  brandVersionNo: number | null;
}

/** asset_versions.content_json of a content asset (SPEC-04 §2.4). */
export interface ContentBody {
  title: string;
  hook: string;
  bodyText: string;
  bodyHtml: string;
  cta: string;
  hashtags: string[];
  platform: string;
  format: string;
  language: string;
  scenes: Scene[];
  warnings: ContentWarning[];
  provenance: Provenance | null;
}

export interface ContentVersion {
  versionNo: number;
  changeNote: string;
  createdAt: ISODateTime;
  generatedByJobId: ID | null;
  content: ContentBody | null;
}

export interface ContentAsset {
  id: ID;
  title: string;
  status: AssetStatus;
  subtype: string;
  campaignId: ID | null;
  brandId: ID | null;
  sourceAssetId: ID | null;
  currentVersion: ContentVersion | null;
}

export interface SaveVersionInput {
  content: ContentBody;
  changeNote: string;
  baseVersionNo: number;
}

export interface BrandOption {
  id: ID;
  name: string;
}
export interface ProductOption {
  id: ID;
  name: string;
}
export interface CampaignSummary {
  id: ID;
  name: string;
  brandId: ID;
  goal: string | null;
  status: string;
}

/** "A", "B", "C"… for variant tabs. */
export function variantLabel(index: number): string {
  return String.fromCharCode(65 + (index % 26));
}

/** Parse a comma separated tone list ("thân thiện, tin cậy"). */
export function parseTones(s: string): string[] {
  return [
    ...new Set(
      s
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean),
    ),
  ].slice(0, 5);
}

/** Parse hashtags typed as "#a #b, c" → ["#a", "#b", "#c"]. */
export function parseHashtags(s: string): string[] {
  const out: string[] = [];
  for (const raw of s.split(/[\s,]+/)) {
    const t = raw.trim();
    if (!t || t === "#") continue;
    const tag = t.startsWith("#") ? t : `#${t}`;
    if (!out.includes(tag)) out.push(tag);
  }
  return out;
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&#34;")
    .replace(/'/g, "&#39;");
}

/** Plain text → safe HTML paragraphs (same rules as the backend RenderHTML). */
export function renderHtml(text: string): string {
  const t = text.replace(/\r\n/g, "\n").trim();
  if (!t) return "";
  return t
    .split("\n\n")
    .map((p) => p.trim())
    .filter(Boolean)
    .map(
      (p) =>
        `<p>${p
          .split("\n")
          .map((l) => escapeHtml(l.trim()))
          .join("<br>")}</p>`,
    )
    .join("");
}

/** Every user-visible text of a version (for warning matching). */
export function contentTexts(
  c: Pick<
    ContentBody,
    "title" | "hook" | "bodyText" | "cta" | "hashtags" | "scenes"
  >,
): string[] {
  return [
    c.title,
    c.hook,
    c.bodyText,
    c.cta,
    c.hashtags.join(" "),
    ...c.scenes.flatMap((s) => [s.narration, s.visualHint]),
  ];
}

/**
 * Warnings still present in the draft: a warning disappears once its text
 * no longer occurs (case-insensitive) after the user edits it out.
 */
export function activeWarnings(
  warnings: ContentWarning[],
  texts: string[],
): ContentWarning[] {
  const joined = texts.join("\n").toLowerCase();
  return warnings.filter((w) => joined.includes(w.text.toLowerCase()));
}

/** Position of a warning in a text, for "click to jump" (case-insensitive). */
export function findWarning(
  text: string,
  warning: ContentWarning,
): { start: number; end: number } | null {
  const i = text.toLowerCase().indexOf(warning.text.toLowerCase());
  return i < 0 ? null : { start: i, end: i + warning.text.length };
}

export const WARNING_LABELS: Record<WarningType, string> = {
  forbidden_word: "Từ cấm",
  unverified_claim: "Số liệu chưa xác thực",
};

export function isDirty(a: ContentBody, b: ContentBody): boolean {
  return (
    a.title !== b.title ||
    a.hook !== b.hook ||
    a.bodyText !== b.bodyText ||
    a.cta !== b.cta ||
    a.hashtags.join(" ") !== b.hashtags.join(" ")
  );
}

/** Apply editor fields to a version body; keeps format, provenance and the still-valid warnings. */
export function applyDraft(
  base: ContentBody,
  draft: Pick<ContentBody, "title" | "hook" | "bodyText" | "cta" | "hashtags">,
): ContentBody {
  const next: ContentBody = {
    ...base,
    ...draft,
    bodyHtml: renderHtml(draft.bodyText),
  };
  return {
    ...next,
    warnings: activeWarnings(base.warnings, contentTexts(next)),
  };
}

export function isTerminal(status: JobStatus): boolean {
  return (
    status === "succeeded" || status === "failed" || status === "cancelled"
  );
}

/** Video Studio deep link ("Gửi sang Video Studio"); keeps the script's campaign so the video joins it. */
export function videoStudioLink(assetId: ID, campaignId?: ID | null): string {
  const q = new URLSearchParams({ script_from_asset_id: assetId });
  if (campaignId) q.set("campaign_id", campaignId);
  return `/videos?${q.toString()}`;
}
