import type { AssetStatus } from "@/shared/kernel";
import type {
  Approval,
  ApprovalDecision,
  AssetDetail,
  AssetFileRole,
  AssetSummary,
  AssetType,
  AssetVersion,
  DownloadLink,
  FileKind,
  Provenance,
  VersionFile,
} from "../domain/asset";

/** snake_case DTOs mirroring api/openapi/asset.yaml. */
export interface AssetVersionFileDto {
  file_id: string;
  role: AssetFileRole;
  sort_order: number;
  mime_type: string;
  kind: FileKind;
  width: number | null;
  height: number | null;
  duration_ms: number | null;
  url: string | null;
}

export interface AssetProvenanceDto {
  job_id: string;
  job_type: string;
  provider_code: string | null;
  model_code: string | null;
  prompt_template_code: string | null;
  prompt_template_version: number | null;
  brand_version_id: string | null;
  brand_version_no: number | null;
}

export interface AssetVersionDto {
  id: string;
  version_no: number;
  content_json: Record<string, unknown>;
  change_note: string | null;
  generated_by_job_id: string | null;
  created_by: string;
  created_by_name: string;
  created_at: string;
  files: AssetVersionFileDto[];
  provenance: AssetProvenanceDto | null;
}

export interface AssetSummaryDto {
  id: string;
  campaign_id: string | null;
  brand_id: string;
  type: AssetType;
  subtype: string | null;
  title: string;
  status: AssetStatus;
  tags: string[];
  usage_rights: string | null;
  source_asset_id: string | null;
  created_by: string;
  created_by_name: string;
  current_version_no: number;
  thumbnail_url: string | null;
  submitted_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface AssetApprovalDto {
  id: string;
  version_id: string;
  version_no: number;
  reviewer_id: string;
  reviewer_name: string;
  decision: ApprovalDecision;
  comment: string | null;
  checklist: Record<string, unknown>;
  created_at: string;
}

export interface AssetDetailDto extends AssetSummaryDto {
  current_version: AssetVersionDto;
  approvals: AssetApprovalDto[];
  permissions: { can_edit: boolean; can_submit: boolean; can_review: boolean; can_download: boolean };
}

export interface AssetDownloadUrlDto {
  url: string;
  expires_at: string;
  file_id: string;
}

export interface CampaignOptionDto {
  id: string;
  name: string;
}

export const toFile = (f: AssetVersionFileDto): VersionFile => ({
  fileId: f.file_id,
  role: f.role,
  sortOrder: f.sort_order,
  mimeType: f.mime_type,
  kind: f.kind,
  width: f.width,
  height: f.height,
  durationMs: f.duration_ms,
  url: f.url,
});

const toProvenance = (p: AssetProvenanceDto): Provenance => ({
  jobId: p.job_id,
  jobType: p.job_type,
  providerCode: p.provider_code,
  modelCode: p.model_code,
  promptTemplateCode: p.prompt_template_code,
  promptTemplateVersion: p.prompt_template_version,
  brandVersionId: p.brand_version_id,
  brandVersionNo: p.brand_version_no,
});

export const toVersion = (v: AssetVersionDto): AssetVersion => ({
  id: v.id,
  versionNo: v.version_no,
  content: v.content_json ?? {},
  changeNote: v.change_note,
  generatedByJobId: v.generated_by_job_id,
  createdBy: v.created_by,
  createdByName: v.created_by_name,
  createdAt: v.created_at,
  files: (v.files ?? []).map(toFile),
  provenance: v.provenance ? toProvenance(v.provenance) : null,
});

export const toSummary = (a: AssetSummaryDto): AssetSummary => ({
  id: a.id,
  campaignId: a.campaign_id,
  brandId: a.brand_id,
  type: a.type,
  subtype: a.subtype,
  title: a.title,
  status: a.status,
  tags: a.tags ?? [],
  usageRights: a.usage_rights,
  sourceAssetId: a.source_asset_id,
  createdBy: a.created_by,
  createdByName: a.created_by_name,
  currentVersionNo: a.current_version_no,
  thumbnailUrl: a.thumbnail_url,
  submittedAt: a.submitted_at,
  createdAt: a.created_at,
  updatedAt: a.updated_at,
});

const toApproval = (a: AssetApprovalDto): Approval => ({
  id: a.id,
  versionId: a.version_id,
  versionNo: a.version_no,
  reviewerId: a.reviewer_id,
  reviewerName: a.reviewer_name,
  decision: a.decision,
  comment: a.comment,
  checklist: a.checklist ?? {},
  createdAt: a.created_at,
});

export const toDetail = (d: AssetDetailDto): AssetDetail => ({
  ...toSummary(d),
  currentVersion: toVersion(d.current_version),
  approvals: (d.approvals ?? []).map(toApproval),
  permissions: {
    canEdit: d.permissions.can_edit,
    canSubmit: d.permissions.can_submit,
    canReview: d.permissions.can_review,
    canDownload: d.permissions.can_download,
  },
});

export const toDownload = (d: AssetDownloadUrlDto): DownloadLink => ({ url: d.url, expiresAt: d.expires_at, fileId: d.file_id });
