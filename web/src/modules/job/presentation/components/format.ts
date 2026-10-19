import type { JobSummary } from "../../domain/job";

/** "openai · gpt-4o-mini" or "—". */
export function providerText(j: Pick<JobSummary, "providerCode" | "modelCode">): string {
  if (!j.providerCode) return "—";
  return j.modelCode ? `${j.providerCode} · ${j.modelCode}` : j.providerCode;
}

/** Creator cell: "Bạn" for the viewer, else the creator's name (short id when unknown). */
export function creatorText(j: Pick<JobSummary, "createdBy" | "createdByName">, currentUserId: string | undefined): string {
  if (j.createdBy === currentUserId) return "Bạn";
  return j.createdByName || j.createdBy.slice(0, 8);
}
