import type { ID, Page } from "@/shared/kernel";
import { http } from "@/shared/api/http";
import { openEventStream } from "@/shared/api/sse";
import type { JobRepository, JobStreamHandlers } from "../application/ports";
import type { Job, JobFilter, JobHistoryEvent, JobStreamEvent, JobSummary } from "../domain/job";
import {
  toHistoryEvent,
  toJob,
  toJobSummary,
  type CancelAcceptedDto,
  type JobAcceptedDto,
  type JobDto,
  type JobHistoryEventDto,
  type JobSummaryDto,
} from "./dto";

export const JOB_POLL_INTERVAL_MS = 3000;

export class HttpJobRepository implements JobRepository {
  async list(filter: JobFilter, cursor?: string | null): Promise<Page<JobSummary>> {
    const res = await http.page<JobSummaryDto>("/jobs", {
      query: {
        status: filter.status,
        type: filter.type,
        provider_code: filter.providerCode,
        created_by: filter.createdBy,
        limit: 20,
        cursor: cursor ?? undefined,
      },
    });
    return { items: res.items.map(toJobSummary), nextCursor: res.nextCursor };
  }

  async get(id: ID): Promise<Job> {
    return toJob(await http.get<JobDto>(`/jobs/${id}`));
  }

  async history(id: ID): Promise<JobHistoryEvent[]> {
    const res = await http.page<JobHistoryEventDto>(`/jobs/${id}/history`);
    return res.items.map(toHistoryEvent);
  }

  async cancel(id: ID) {
    const d = await http.post<CancelAcceptedDto>(`/jobs/${id}/cancel`);
    return { jobId: d.job_id, status: d.status, cancelRequested: d.cancel_requested };
  }

  async retry(id: ID, force = false) {
    const d = await http.post<JobAcceptedDto>(`/jobs/${id}/retry`, force ? { force: true } : undefined);
    return { jobId: d.job_id, status: d.status };
  }

  subscribe(id: ID, h: JobStreamHandlers): () => void {
    const on =
      (type: JobStreamEvent["type"]) =>
      (data: unknown): void =>
        h.onEvent({ type, data: (data ?? {}) as never } as JobStreamEvent);
    const stream = openEventStream(`/jobs/${id}/events`, {
      events: {
        status: on("status"),
        progress: on("progress"),
        step: on("step"),
        done: (data) => {
          on("done")(data);
          // The server ends the stream after `done`; close before EventSource reconnects.
          stream.close();
          h.onModeChange?.("closed");
        },
      },
      fallbackPoll: h.onPoll,
      pollIntervalMs: JOB_POLL_INTERVAL_MS,
      onFallback: () => h.onModeChange?.("polling"),
    });
    h.onModeChange?.(stream.mode);
    return () => stream.close();
  }
}
