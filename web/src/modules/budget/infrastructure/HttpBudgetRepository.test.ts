import { isApiError } from "@/shared/kernel";
import { mockSession } from "@/shared/api/mock";
import { HttpBudgetRepository } from "./HttpBudgetRepository";
import { resetBudgetMocks } from "./mocks";

afterEach(resetBudgetMocks);

describe("HttpBudgetRepository ↔ mocks contract", () => {
  const repo = new HttpBudgetRepository();

  it("estimates video with breakdown and limits", async () => {
    mockSession.signInAs("creator@acs.local");
    const e = await repo.estimate({ kind: "video", clipSeconds: 20, resolution: "1080p", voiceoverChars: 420 });
    expect(e.lines.map((l) => l.item)).toEqual(["scene_clips", "voiceover", "render"]);
    expect(e.total).toBeCloseTo(3.00672, 6);
    expect(e.limits.perJobLimit).toBe(8);
    expect(e.requiresOverride).toBe(false);
  });

  it("maps PRICING_MISSING to an ApiError with status 503", async () => {
    mockSession.signInAs("creator@acs.local");
    const err = await repo.estimate({ kind: "image", quality: "high", size: "4096" }).catch((e: unknown) => e);
    expect(isApiError(err) && err.status === 503 && err.code === "PRICING_MISSING").toBe(true);
  });

  it("returns a usage summary whose totals equal the sum of rows", async () => {
    mockSession.signInAs("lead@acs.local");
    const s = await repo.usageSummary({ from: "2000-01-01", to: "2100-01-01", groupBy: "provider" });
    const sum = s.rows.reduce((a, r) => a + r.total, 0);
    expect(sum).toBeCloseTo(s.totals.total, 6);
  });

  it("updates settings and campaign budgets", async () => {
    mockSession.signInAs("admin@acs.local");
    const st = await repo.updateSettings({ perJobLimits: { video: 10 }, global: { limitAmount: 500 } });
    expect(st.perJobLimits.video).toBe(10);
    expect(st.global.limitAmount).toBe(500);
    const b = await repo.putCampaignBudget("c0000000-0000-4000-8000-000000000002", { limitAmount: 75, alertThresholds: [50, 100] });
    expect(b.limitAmount).toBe(75);
    expect((await repo.getSettings()).campaigns).toHaveLength(2);
    expect((await repo.getCampaignBudget("c0000000-0000-4000-8000-000000000002"))?.limitAmount).toBe(75);
  });

  it("returns null for a campaign without a budget", async () => {
    mockSession.signInAs("lead@acs.local");
    expect(await repo.getCampaignBudget("c0000000-0000-4000-8000-0000000000ff")).toBeNull();
  });
});
