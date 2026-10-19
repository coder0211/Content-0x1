/** Word-level text diff for the review pane (ACS-081): previous version → current version. */

export type DiffKind = "same" | "added" | "removed";

export interface DiffSegment {
  kind: DiffKind;
  text: string;
}

/** Above this many token pairs the LCS table is too large; fall back to "all removed + all added". */
const MAX_CELLS = 4_000_000;

/** Splits into words and the whitespace between them, so joining tokens restores the text. */
export function tokenize(text: string): string[] {
  return text.split(/(\s+)/).filter((t) => t !== "");
}

/** LCS diff over word/whitespace tokens; adjacent segments of the same kind are merged. */
export function diffWords(before: string, after: string): DiffSegment[] {
  const a = tokenize(before);
  const b = tokenize(after);
  const out: DiffSegment[] = [];
  const push = (kind: DiffKind, text: string) => {
    const last = out[out.length - 1];
    if (last && last.kind === kind) last.text += text;
    else out.push({ kind, text });
  };
  if (a.length * b.length > MAX_CELLS) {
    if (before) push("removed", before);
    if (after) push("added", after);
    return out;
  }
  // lcs[i][j] = LCS length of a[i:] and b[j:]
  const w = b.length + 1;
  const lcs = new Uint32Array((a.length + 1) * w);
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      lcs[i * w + j] = a[i] === b[j] ? lcs[(i + 1) * w + j + 1]! + 1 : Math.max(lcs[(i + 1) * w + j]!, lcs[i * w + j + 1]!);
    }
  }
  let i = 0;
  let j = 0;
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      push("same", a[i]!);
      i++;
      j++;
    } else if (lcs[(i + 1) * w + j]! >= lcs[i * w + j + 1]!) {
      push("removed", a[i]!);
      i++;
    } else {
      push("added", b[j]!);
      j++;
    }
  }
  while (i < a.length) push("removed", a[i++]!);
  while (j < b.length) push("added", b[j++]!);
  return coalesce(out);
}

/**
 * Whitespace-only "same" segments between changes split one edit into many tiny ones
 * ("giao" + " " + "tận" …). Fold such runs into one removed + one added segment.
 */
function coalesce(segs: DiffSegment[]): DiffSegment[] {
  const out: DiffSegment[] = [];
  let k = 0;
  while (k < segs.length) {
    const s = segs[k]!;
    if (s.kind === "same") {
      out.push(s);
      k++;
      continue;
    }
    let removed = "";
    let added = "";
    let end = k;
    // Extend the run over changes and whitespace-only "same" gaps followed by another change.
    while (end < segs.length) {
      const cur = segs[end]!;
      if (cur.kind === "removed") removed += cur.text;
      else if (cur.kind === "added") added += cur.text;
      else if (/^\s+$/.test(cur.text) && segs[end + 1] && segs[end + 1]!.kind !== "same") {
        removed += cur.text;
        added += cur.text;
      } else break;
      end++;
    }
    if (removed) out.push({ kind: "removed", text: removed });
    if (added) out.push({ kind: "added", text: added });
    k = end;
  }
  return out;
}

/** Counts changed words (whitespace ignored) for the "N thay đổi" summary. */
export function diffStats(segments: DiffSegment[]): { added: number; removed: number } {
  const words = (s: string) => s.split(/\s+/).filter(Boolean).length;
  return segments.reduce(
    (acc, s) => {
      if (s.kind === "added") acc.added += words(s.text);
      if (s.kind === "removed") acc.removed += words(s.text);
      return acc;
    },
    { added: 0, removed: 0 },
  );
}
