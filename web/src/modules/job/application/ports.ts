import type { ID, JobStatus, Page } from "@/shared/kernel";
import type { Job, JobFilter, JobHistoryEvent, JobStreamEvent, JobSummary } from "../domain/job";

export type JobStreamMode = "sse" | "polling" | "closed";

export interface JobStreamHandlers {
  /** One SSE event (status | progress | step | done). */
  onEvent: (event: JobStreamEvent) => void;
  /** Called every 3s once SSE is unavailable: refetch the job. */
  onPoll: () => void;
  onModeChange?: (mode: JobStreamMode) => void;
}

export interface JobRepository {
  list(filter: JobFilter, cursor?: string | null): Promise<Page<JobSummary>>;
  get(id: ID): Promise<Job>;
  history(id: ID): Promise<JobHistoryEvent[]>;
  cancel(id: ID): Promise<{ jobId: ID; status: JobStatus; cancelRequested: boolean }>;
  retry(id: ID, force?: boolean): Promise<{ jobId: ID; status: JobStatus }>;
  /** Opens the SSE stream with automatic polling fallback; returns a close function. */
  subscribe(id: ID, handlers: JobStreamHandlers): () => void;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    job: JobRepository;
  }
}
