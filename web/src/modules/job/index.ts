import { lazyPage, type AppRoute } from "@/shared/lib/module";
import type { RepositoryFactories } from "@/shared/lib/di";
import { HttpJobRepository } from "./infrastructure/HttpJobRepository";

/** /jobs has no sidebar entry (docs/DECISIONS.md); everyone may open it and sees their own jobs. */
export const routes: AppRoute[] = [
  {
    path: "jobs",
    lazy: lazyPage(() => import("./presentation/pages/JobsPage")),
    handle: { crumb: "Job" },
  },
  {
    path: "jobs/:jobId",
    lazy: lazyPage(() => import("./presentation/pages/JobDetailPage")),
    handle: { crumb: "Chi tiết job" },
  },
];

export const repositories: RepositoryFactories = { job: () => new HttpJobRepository() };

// Public API for other modules (ACS-019).
export { useJob, type UseJobOptions, type UseJobResult } from "./application/useJob";
export { useJobProgress, type UseJobProgressResult } from "./application/useJobProgress";
export { jobKeys } from "./application/keys";
export type { JobRepository } from "./application/ports";
export { JobProgressPanel, type JobProgressPanelProps } from "./presentation/components/JobProgressPanel";
export {
  canCancel,
  canRetry,
  isTerminal,
  JOB_TYPE_LABELS,
  progressLabel,
  type Job,
  type JobStep,
  type JobSummary,
  type JobType,
} from "./domain/job";
