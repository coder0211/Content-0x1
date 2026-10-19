import { budgetTone, jobTypeLabel } from "./summary";

describe("dashboard domain", () => {
  it("maps budget usage to a tone", () => {
    expect(budgetTone(null)).toBe("neutral");
    expect(budgetTone(42)).toBe("neutral");
    expect(budgetTone(80)).toBe("warning");
    expect(budgetTone(120)).toBe("danger");
  });

  it("labels known job types and passes unknown ones through", () => {
    expect(jobTypeLabel("image_generation")).toBe("Sinh ảnh");
    expect(jobTypeLabel("custom_x")).toBe("custom_x");
  });
});
