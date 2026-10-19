/**
 * Shared in-memory asset store for MSW mocks (api/openapi/asset.yaml shapes).
 * The library module's mocks own every `/assets*` + `/reviews/queue` handler over this store;
 * studio mocks (content, image, video) publish the assets their jobs create here, so
 * submit-review → review queue → approve → download works across modules in mock mode.
 * Never imported by production code paths — only by mock handler files and tests.
 */
import type { AssetStatus } from "@/shared/kernel";
import { uuid } from "@/shared/lib/id";
import { MOCK_USERS } from "./mock";

export type MockAssetType = "content" | "image" | "video";

export interface MockAssetFileDto {
  file_id: string;
  role: "primary" | "variant" | "thumbnail" | "subtitle" | "audio" | "preview";
  sort_order: number;
  mime_type: string;
  kind: "image" | "video" | "audio" | "document" | "subtitle";
  width: number | null;
  height: number | null;
  duration_ms: number | null;
  url: string | null;
}

export interface MockAssetProvenanceDto {
  job_id: string;
  job_type: string;
  provider_code: string | null;
  model_code: string | null;
  prompt_template_code: string | null;
  prompt_template_version: number | null;
  brand_version_id: string | null;
  brand_version_no: number | null;
}

export interface MockAssetVersionDto {
  id: string;
  version_no: number;
  content_json: Record<string, unknown>;
  change_note: string | null;
  generated_by_job_id: string | null;
  created_by: string;
  created_by_name: string;
  created_at: string;
  files: MockAssetFileDto[];
  provenance: MockAssetProvenanceDto | null;
}

export interface MockAssetApprovalDto {
  id: string;
  version_id: string;
  version_no: number;
  reviewer_id: string;
  reviewer_name: string;
  decision: "submitted" | "approved" | "changes_requested";
  comment: string | null;
  checklist: Record<string, unknown>;
  created_at: string;
}

export interface MockAssetSummaryFields {
  id: string;
  campaign_id: string | null;
  brand_id: string;
  type: MockAssetType;
  subtype: string | null;
  title: string;
  status: AssetStatus;
  tags: string[];
  usage_rights: string | null;
  source_asset_id: string | null;
  created_by: string;
  created_by_name: string;
  submitted_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface StoredMockAsset {
  summary: MockAssetSummaryFields;
  versions: MockAssetVersionDto[];
  approvals: MockAssetApprovalDto[];
  deleted: boolean;
}

let store: StoredMockAsset[] = [];

const userName = (id: string) => MOCK_USERS.find((u) => u.id === id)?.name ?? "Người dùng";
const now = () => new Date().toISOString();

export const mockAssets = {
  /** Every stored asset (mutable, including soft-deleted ones). */
  all(): StoredMockAsset[] {
    return store;
  },
  /** Replace the store (library seed / tests). */
  replace(next: StoredMockAsset[]): void {
    store = next;
  },
  find(id: unknown): StoredMockAsset | undefined {
    return store.find((a) => a.summary.id === id && !a.deleted);
  },
  current(a: StoredMockAsset): MockAssetVersionDto {
    return a.versions[a.versions.length - 1]!;
  },
  /** Creates a draft asset with version 1 (what a studio job does on success). */
  create(input: {
    id?: string;
    type: MockAssetType;
    subtype?: string | null;
    title: string;
    campaignId?: string | null;
    brandId: string;
    createdBy: string;
    sourceAssetId?: string | null;
    content: Record<string, unknown>;
    files?: MockAssetFileDto[];
    jobId?: string | null;
    jobType?: string;
    changeNote?: string | null;
  }): StoredMockAsset {
    const at = now();
    const a: StoredMockAsset = {
      summary: {
        id: input.id ?? uuid(),
        campaign_id: input.campaignId ?? null,
        brand_id: input.brandId,
        type: input.type,
        subtype: input.subtype ?? null,
        title: input.title,
        status: "draft",
        tags: [],
        usage_rights: null,
        source_asset_id: input.sourceAssetId ?? null,
        created_by: input.createdBy,
        created_by_name: userName(input.createdBy),
        submitted_at: null,
        created_at: at,
        updated_at: at,
      },
      versions: [],
      approvals: [],
      deleted: false,
    };
    store.unshift(a);
    mockAssets.addVersion(a, {
      content: input.content,
      files: input.files,
      jobId: input.jobId,
      jobType: input.jobType,
      changeNote: input.changeNote ?? null,
      createdBy: input.createdBy,
    });
    return a;
  },
  /** Appends a version; like the backend, any new version moves the asset back to draft. */
  addVersion(
    a: StoredMockAsset,
    input: {
      content: Record<string, unknown>;
      files?: MockAssetFileDto[];
      jobId?: string | null;
      jobType?: string;
      changeNote?: string | null;
      createdBy: string;
    },
  ): MockAssetVersionDto {
    const prov = (input.content.provenance ?? null) as Partial<MockAssetProvenanceDto> | null;
    const v: MockAssetVersionDto = {
      id: uuid(),
      version_no: (a.versions[a.versions.length - 1]?.version_no ?? 0) + 1,
      content_json: input.content,
      change_note: input.changeNote ?? null,
      generated_by_job_id: input.jobId ?? null,
      created_by: input.createdBy,
      created_by_name: userName(input.createdBy),
      created_at: now(),
      files: input.files ?? [],
      provenance: input.jobId
        ? {
            job_id: input.jobId,
            job_type: input.jobType ?? `${a.summary.type}_generation`,
            provider_code: prov?.provider_code ?? "fake",
            model_code: prov?.model_code ?? "fake-model",
            prompt_template_code: prov?.prompt_template_code ?? null,
            prompt_template_version: prov?.prompt_template_version ?? null,
            brand_version_id: null,
            brand_version_no: prov?.brand_version_no ?? null,
          }
        : null,
    };
    a.versions.push(v);
    if (a.summary.status !== "archived") a.summary.status = "draft";
    a.summary.updated_at = v.created_at;
    return v;
  },
};
