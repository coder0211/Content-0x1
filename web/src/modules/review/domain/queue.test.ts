import { nextSelection } from "./queue";

describe("nextSelection", () => {
  it("moves to the following asset, or the previous one at the end", () => {
    expect(nextSelection(["a", "b", "c"], "a")).toBe("b");
    expect(nextSelection(["a", "b", "c"], "b")).toBe("c");
    expect(nextSelection(["a", "b", "c"], "c")).toBe("b");
  });
  it("returns null when the queue empties and the head when the id is unknown", () => {
    expect(nextSelection(["a"], "a")).toBeNull();
    expect(nextSelection(["a", "b"], "x")).toBe("a");
  });
});
