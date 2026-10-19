import type { AssetStatus, ID, ISODateTime } from "@/shared/kernel";

/** Library domain (SPEC-04 §2.8, api/openapi/asset.yaml). Pure TS, no React/fetch. */
export type AssetType = "content" | "image" | "video";
export type AssetFileRole = "primary" | "variant" | "thumbnail" | "subtitle" | "audio" | "preview";
export type FileKind = "image" | "video" | "audio" | "document" | "subtitle";

export const ASSET_TYPES: readonly AssetType[] = ["content", "image", "video"];
export const ASSET_STATUSES: readonly AssetStatus[] = ["draft", "pending_review", "changes_requested", "approved", "archived"];

export const ASSET_TYPE_LABELS: Record<AssetType, string> = {
  content: "Content",
  image: "Ảnh",
  video: "Video",
};

export interface VersionFile {
  fileId: ID;
  role: AssetFileRole;
  sortOrder: number;
  mimeType: string;
  kind: FileKind;
  width: number | null;
  height: number | null;
  durationMs: number | null;
  url: string | null;
}

export interface Provenance {
  jobId: ID;
  jobType: string;
  providerCode: string | null;
  modelCode: string | null;
  promptTemplateCode: string | null;
  promptTemplateVersion: number | null;
  brandVersionId: ID | null;
  brandVersionNo: number | null;
}

export interface AssetVersion {
  id: ID;
  versionNo: number;
  content: Record<string, unknown>;
  changeNote: string | null;
  generatedByJobId: ID | null;
  createdBy: ID;
  createdByName: string;
  createdAt: ISODateTime;
  files: VersionFile[];
  provenance: Provenance | null;
}

export interface AssetSummary {
  id: ID;
  campaignId: ID | null;
  brandId: ID;
  type: AssetType;
  subtype: string | null;
  title: string;
  status: AssetStatus;
  tags: string[];
  usageRights: string | null;
  sourceAssetId: ID | null;
  createdBy: ID;
  createdByName: string;
  currentVersionNo: number;
  thumbnailUrl: string | null;
  submittedAt: ISODateTime | null;
  createdAt: ISODateTime;
  updatedAt: ISODateTime;
}

export type ApprovalDecision = "submitted" | "approved" | "changes_requested";

export interface Approval {
  id: ID;
  versionId: ID;
  versionNo: number;
  reviewerId: ID;
  reviewerName: string;
  decision: ApprovalDecision;
  comment: string | null;
  checklist: Record<string, unknown>;
  createdAt: ISODateTime;
}

export interface AssetPermissions {
  canEdit: boolean;
  canSubmit: boolean;
  canReview: boolean;
  canDownload: boolean;
}

export interface AssetDetail extends AssetSummary {
  currentVersion: AssetVersion;
  approvals: Approval[];
  permissions: AssetPermissions;
}

export interface AssetUpdate {
  title?: string;
  tags?: string[];
  usageRights?: string | null;
}

export interface SubmitReviewInput {
  comment?: string;
  typographyChecked?: boolean;
}

export interface DownloadLink {
  url: string;
  expiresAt: ISODateTime;
  fileId: ID;
}

export interface CampaignOption {
  id: ID;
  name: string;
}

/** GET /reviews/queue filters. */
export interface ReviewQueueFilters {
  type: AssetType | null;
  campaignId: ID | null;
}

// ---------------------------------------------------------------- filters

export interface AssetFilters {
  q: string;
  type: AssetType | null;
  statuses: AssetStatus[];
  campaignId: ID | null;
  createdBy: ID | null;
  tag: string;
  from: string; // YYYY-MM-DD
  to: string; // YYYY-MM-DD
}

export const EMPTY_FILTERS: AssetFilters = {
  q: "",
  type: null,
  statuses: [],
  campaignId: null,
  createdBy: null,
  tag: "",
  from: "",
  to: "",
};

const isType = (v: string | null): v is AssetType => v !== null && (ASSET_TYPES as readonly string[]).includes(v);
const isStatus = (v: string): v is AssetStatus => (ASSET_STATUSES as readonly string[]).includes(v);
const isDate = (v: string | null): v is string => v !== null && /^\d{4}-\d{2}-\d{2}$/.test(v);

/** URL search params → filters (unknown values are dropped). */
export function filtersFromParams(p: URLSearchParams): AssetFilters {
  const type = p.get("type");
  const from = p.get("from");
  const to = p.get("to");
  return {
    q: p.get("q") ?? "",
    type: isType(type) ? type : null,
    statuses: (p.get("status") ?? "").split(",").filter(isStatus),
    campaignId: p.get("campaign_id") || null,
    createdBy: p.get("created_by") || null,
    tag: p.get("tag") ?? "",
    from: isDate(from) ? from : "",
    to: isDate(to) ? to : "",
  };
}

/** Filters → URL search params (only non-empty values, same names as the API). */
export function filtersToParams(f: AssetFilters): URLSearchParams {
  const p = new URLSearchParams();
  if (f.q.trim()) p.set("q", f.q.trim());
  if (f.type) p.set("type", f.type);
  if (f.statuses.length) p.set("status", f.statuses.join(","));
  if (f.campaignId) p.set("campaign_id", f.campaignId);
  if (f.createdBy) p.set("created_by", f.createdBy);
  if (f.tag.trim()) p.set("tag", f.tag.trim());
  if (f.from) p.set("from", f.from);
  if (f.to) p.set("to", f.to);
  return p;
}

export function hasActiveFilters(f: AssetFilters): boolean {
  return filtersToParams(f).toString() !== "";
}

export function toggleStatus(list: AssetStatus[], s: AssetStatus): AssetStatus[] {
  return list.includes(s) ? list.filter((x) => x !== s) : [...list, s];
}

// ---------------------------------------------------------------- rules

/** Posters must confirm the typography checklist before review (SPEC-04 §2.8). */
export function requiresTypographyCheck(a: Pick<AssetSummary, "subtype">): boolean {
  return a.subtype === "poster";
}

/** Can be submitted from draft (changes_requested goes back to draft with a new version). */
export function canSubmitReview(a: Pick<AssetSummary, "status">): boolean {
  return a.status === "draft";
}

/** Tags: trimmed, lower-cased, de-duplicated, ≤ 20 items of ≤ 50 chars. */
export function normalizeTags(input: string | string[]): string[] {
  const raw = Array.isArray(input) ? input : input.split(",");
  const out: string[] = [];
  for (const t of raw) {
    const v = t.trim().toLowerCase().slice(0, 50);
    if (v && !out.includes(v)) out.push(v);
  }
  return out.slice(0, 20);
}

/** File to preview for a version: primary → preview → first variant → thumbnail → first with a URL. */
export function previewFile(files: VersionFile[]): VersionFile | null {
  const withUrl = files.filter((f) => f.url);
  for (const role of ["primary", "preview", "variant", "thumbnail"] as const) {
    const hit = withUrl.find((f) => f.role === role && (f.kind === "image" || f.kind === "video"));
    if (hit) return hit;
  }
  return withUrl.find((f) => f.kind === "image" || f.kind === "video") ?? null;
}

/** Full-size images of an image version (primary + variants, in order); thumbnails are not repeated. */
export function imageFiles(files: VersionFile[]): VersionFile[] {
  return files
    .filter((f) => f.url && f.kind === "image" && (f.role === "primary" || f.role === "variant"))
    .sort((a, b) => a.sortOrder - b.sortOrder);
}

const str = (v: unknown): string => (typeof v === "string" ? v : "");

/**
 * Plain text of a version, used for display and the review diff:
 * content → title/hook/body/cta/hashtags; image → prompt; video → prompt + script.
 */
export function versionText(type: AssetType, content: Record<string, unknown>): string {
  if (type === "content") {
    const hashtags = Array.isArray(content.hashtags) ? content.hashtags.filter((h) => typeof h === "string").join(" ") : "";
    return [str(content.title), str(content.hook), str(content.body_text), str(content.cta), hashtags]
      .filter((s) => s.trim())
      .join("\n\n");
  }
  if (type === "image") return str(content.prompt);
  return [str(content.prompt), str(content.script)].filter((s) => s.trim()).join("\n\n");
}
