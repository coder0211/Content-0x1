import type { JobSummary } from "../../domain/job";

/** "openai · gpt-4o-mini" or "—". */
export function providerText(j: Pick<JobSummary, "providerCode" | "modelCode">): string {
  if (!j.providerCode) return "—";
  return j.modelCode ? `${j.providerCode} · ${j.modelCode}` : j.providerCode;
}

/** Creator cell: "Bạn" for the viewer, else a short id (no users API in this module). */
export function creatorText(createdBy: string, currentUserId: string | undefined): string {
  return createdBy === currentUserId ? "Bạn" : createdBy.slice(0, 8);
}
