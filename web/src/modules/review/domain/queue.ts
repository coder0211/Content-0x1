/**
 * After deciding on `decidedId`, select the next asset in queue order (or the previous one when
 * it was last); null when the queue becomes empty.
 */
export function nextSelection(ids: readonly string[], decidedId: string): string | null {
  const i = ids.indexOf(decidedId);
  const rest = ids.filter((id) => id !== decidedId);
  if (rest.length === 0) return null;
  if (i < 0) return rest[0]!;
  return rest[Math.min(i, rest.length - 1)]!;
}
