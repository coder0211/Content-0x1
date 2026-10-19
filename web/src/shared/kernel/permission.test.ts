import { hasPermission } from "./permission";

describe("hasPermission", () => {
  const granted = ["brand.view", "generate.run"] as const;
  it("handles single, all-of and any-of requirements", () => {
    expect(hasPermission(granted, undefined)).toBe(true);
    expect(hasPermission(granted, "brand.view")).toBe(true);
    expect(hasPermission(granted, "usage.view")).toBe(false);
    expect(hasPermission(granted, ["brand.view", "generate.run"])).toBe(true);
    expect(hasPermission(granted, ["brand.view", "usage.view"])).toBe(false);
    expect(hasPermission(granted, { anyOf: ["usage.view", "generate.run"] })).toBe(true);
    expect(hasPermission(granted, { anyOf: ["usage.view", "audit.view"] })).toBe(false);
  });
});
