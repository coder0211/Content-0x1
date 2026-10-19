import { budgetTone, currentMonthRange, parseAmount, parseThresholds, sumRows, totalsMatchRows } from "./budget";

describe("budget domain", () => {
  it("parses thresholds", () => {
    expect(parseThresholds("100, 80")).toEqual({ ok: true, value: [80, 100] });
    expect(parseThresholds("")).toMatchObject({ ok: false });
    expect(parseThresholds("0")).toMatchObject({ ok: false });
    expect(parseThresholds("80 80")).toMatchObject({ ok: false });
    expect(parseThresholds("10,20,30,40,50,60")).toMatchObject({ ok: false });
    expect(parseThresholds("80.5")).toMatchObject({ ok: false });
  });

  it("parses amounts", () => {
    expect(parseAmount("12,5")).toBe(12.5);
    expect(parseAmount("1000")).toBe(1000);
    expect(parseAmount("-1")).toBeNull();
    expect(parseAmount("abc")).toBeNull();
  });

  it("sums rows to the micro-dollar and checks totals", () => {
    const rows = [
      { providerReported: 0.1, calculated: 0.2, estimated: 0, total: 0.3, count: 2 },
      { providerReported: 0, calculated: 0.000003, estimated: 0.7, total: 0.700003, count: 3 },
    ];
    const s = sumRows(rows);
    expect(s.total).toBe(1.000003);
    expect(s.count).toBe(5);
    expect(totalsMatchRows({ rows: rows.map((r) => ({ ...r, key: null, label: "" })), totals: s })).toBe(true);
    expect(totalsMatchRows({ rows: [], totals: s })).toBe(false);
  });

  it("maps percent to tone and builds the month range", () => {
    expect(budgetTone(10)).toBe("ok");
    expect(budgetTone(80)).toBe("warning");
    expect(budgetTone(100)).toBe("danger");
    expect(currentMonthRange(new Date("2026-10-19T09:00:00Z"))).toEqual({ from: "2026-10-01", to: "2026-10-19" });
  });
});
