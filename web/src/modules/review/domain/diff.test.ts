import { diffStats, diffWords, tokenize } from "./diff";

describe("diffWords", () => {
  it("keeps whitespace so segments rebuild both texts", () => {
    const before = "Bánh kem trà xanh mềm mịn, ít ngọt.";
    const after = "Bánh kem trà xanh mềm mịn, giao tận nhà trong 2 giờ.";
    const segs = diffWords(before, after);
    const rebuild = (skip: "added" | "removed") => segs.filter((s) => s.kind !== skip).map((s) => s.text).join("");
    expect(rebuild("added")).toBe(before);
    expect(rebuild("removed")).toBe(after);
    expect(segs[0]).toEqual({ kind: "same", text: "Bánh kem trà xanh mềm mịn, " });
  });

  it("folds whitespace gaps so one edit is one inserted segment", () => {
    const segs = diffWords("ít ngọt.", "ít ngọt, giao tận nhà.");
    expect(segs.filter((s) => s.kind === "added")).toEqual([{ kind: "added", text: "ngọt, giao tận nhà." }]);
  });

  it("reports identical text as one unchanged segment", () => {
    expect(diffWords("a b", "a b")).toEqual([{ kind: "same", text: "a b" }]);
  });

  it("handles empty sides", () => {
    expect(diffWords("", "mới")).toEqual([{ kind: "added", text: "mới" }]);
    expect(diffWords("cũ", "")).toEqual([{ kind: "removed", text: "cũ" }]);
    expect(diffWords("", "")).toEqual([]);
  });

  it("counts changed words", () => {
    expect(diffStats(diffWords("giảm 20% hôm nay", "giảm 30% đến 31/12"))).toEqual({ added: 3, removed: 3 });
  });

  it("tokenizes words and whitespace", () => {
    expect(tokenize("a  b\nc")).toEqual(["a", "  ", "b", "\n", "c"]);
  });
});
