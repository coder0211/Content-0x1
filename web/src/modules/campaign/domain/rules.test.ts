import {
  acceptsGeneration,
  canEditCampaign,
  emptyBrief,
  emptyDraft,
  isHeaderField,
  newPlanItem,
  planProgress,
  studioLink,
  transitionLabel,
  validateBrief,
  validateHeader,
  validatePlan,
} from "./rules";

describe("campaign rules", () => {
  it("validates the header (step 1)", () => {
    expect(validateHeader(emptyDraft())).toEqual({ brand_id: "required", name: "required", goal: "required" });
    const d = { ...emptyDraft("b1"), name: "T11", goal: "lead" as const, startDate: "2026-11-10", endDate: "2026-11-01" };
    expect(validateHeader(d)).toEqual({ end_date: "before_start_date" });
  });

  it("validates the brief with one error per field (FR-CMP-01)", () => {
    expect(validateBrief(emptyBrief())).toEqual({
      "brief.target_audience": "required",
      "brief.key_message": "required",
      "brief.channels": "required",
      "brief.formats": "required",
      "brief.deadline": "required",
    });
    const b = {
      ...emptyBrief(),
      targetAudience: "SV",
      keyMessage: "Tự tin",
      channels: ["facebook"],
      formats: [
        { type: "poster", count: 2 },
        { type: "poster", count: 0 },
      ],
      deadline: "2026-13-01",
    };
    expect(validateBrief(b)).toEqual({
      "brief.formats[1].type": "duplicate",
      "brief.formats[1].count": "out_of_range",
      "brief.deadline": "invalid_date",
    });
  });

  it("validates a plan", () => {
    expect(validatePlan({ summary: "", themes: [], items: [] })).toEqual({ "plan.items": "required" });
    const item = { ...newPlanItem([]), topic: "", format: "poster", publishDate: "x" };
    expect(validatePlan({ summary: "", themes: [], items: [item] })).toEqual({
      "plan.items[0].topic": "required",
      "plan.items[0].publish_date": "invalid_date",
    });
  });

  it("generates unique plan item ids", () => {
    const a = newPlanItem([]);
    expect(a.id).toBe("item-1");
    expect(newPlanItem([{ ...a, id: "item-2" }]).id).toBe("item-3");
  });

  it("computes plan progress", () => {
    const i = newPlanItem([]);
    expect(planProgress(null)).toEqual({ done: 0, total: 0 });
    expect(planProgress({ summary: "", themes: [], items: [i, { ...i, id: "x", done: true }] })).toEqual({ done: 1, total: 2 });
  });

  it("builds studio links with the campaign preselected (ACS-026)", () => {
    expect(studioLink("content", "c1")).toBe("/content?campaign_id=c1");
    expect(studioLink("image", "c1")).toBe("/images?campaign_id=c1");
    expect(studioLink("video", "c1", "item-2")).toBe("/videos?campaign_id=c1&plan_item_id=item-2");
  });

  it("knows who may edit", () => {
    const c = { createdBy: "u1", memberIds: ["u2"], status: "draft" as const };
    const creator = { id: "u3", roles: ["content_creator" as const], permissions: ["campaign.manage"] };
    expect(canEditCampaign(c, creator)).toBe(false);
    expect(canEditCampaign(c, { ...creator, id: "u2" })).toBe(true);
    expect(canEditCampaign(c, { id: "u9", roles: ["marketing_lead"], permissions: ["campaign.manage"] })).toBe(true);
    expect(canEditCampaign(c, { id: "u1", roles: ["reviewer"], permissions: ["brand.view"] })).toBe(false);
    expect(canEditCampaign({ ...c, status: "archived" }, { ...creator, id: "u1" })).toBe(false);
  });

  it("labels transitions and gates generation", () => {
    expect(transitionLabel("review", "in_progress")).toBe("Trở lại thực hiện");
    expect(transitionLabel("draft", "in_progress")).toBe("Bắt đầu thực hiện");
    expect(acceptsGeneration("completed")).toBe(false);
    expect(acceptsGeneration("review")).toBe(true);
  });

  it("classifies wizard fields", () => {
    expect(isHeaderField("name")).toBe(true);
    expect(isHeaderField("brief.deadline")).toBe(false);
  });
});
