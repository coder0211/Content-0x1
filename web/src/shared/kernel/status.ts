/** State machines from SPEC-03 (CHECK constraints). StatusBadge maps these 1-1. */
export type AssetStatus = "draft" | "pending_review" | "changes_requested" | "approved" | "archived";
export type CampaignStatus = "draft" | "in_progress" | "review" | "completed" | "archived";
export type JobStatus = "queued" | "running" | "succeeded" | "failed" | "cancelled";
export type JobStepStatus =
  | "pending"
  | "queued"
  | "running"
  | "waiting_provider"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "skipped";
export type SceneStatus = "draft" | "generating" | "ready" | "failed";
export type UserStatus = "active" | "locked" | "disabled";
export type ActiveStatus = "active" | "disabled";
export type BrandStatus = "active" | "archived";

export interface StatusKinds {
  asset: AssetStatus;
  campaign: CampaignStatus;
  job: JobStatus;
  step: JobStepStatus;
  scene: SceneStatus;
  user: UserStatus;
  provider: ActiveStatus;
  brand: BrandStatus;
}

export type StatusKind = keyof StatusKinds;

/** Provider error classification (SPEC-02) surfaced on failed jobs/steps. */
/** Step `error_kind` values (JobStep.error_kind in api/openapi.yaml). */
export type ErrorKind =
  | "ContentPolicy"
  | "RateLimited"
  | "Transient"
  | "InvalidRequest"
  | "Permanent"
  | "Auth"
  | "Unknown"
  | "Timeout"
  | "Domain"
  | "Internal"
  | "LeaseExpired";
