import {
  EMPTY_FILTERS,
  filtersFromParams,
  filtersToParams,
  normalizeTags,
  imageFiles,
  previewFile,
  requiresTypographyCheck,
  toggleStatus,
  versionText,
  type VersionFile,
} from "./asset";

const file = (over: Partial<VersionFile>): VersionFile => ({
  fileId: "f",
  role: "primary",
  sortOrder: 0,
  mimeType: "image/png",
  kind: "image",
  width: null,
  height: null,
  durationMs: null,
  url: "https://s3/x",
  ...over,
});

describe("library filters", () => {
  it("round-trips combined filters through URL params with API names", () => {
    const f = { ...EMPTY_FILTERS, q: " tết ", type: "image" as const, statuses: ["draft" as const, "approved" as const], campaignId: "c1", tag: "tet", from: "2026-10-01", to: "2026-10-31" };
    const p = filtersToParams(f);
    expect(p.toString()).toBe("q=t%E1%BA%BFt&type=image&status=draft%2Capproved&campaign_id=c1&tag=tet&from=2026-10-01&to=2026-10-31");
    expect(filtersFromParams(p)).toEqual({ ...f, q: "tết" });
  });

  it("drops unknown values", () => {
    const f = filtersFromParams(new URLSearchParams("type=pdf&status=draft,bogus&from=hôm-qua"));
    expect(f.type).toBeNull();
    expect(f.statuses).toEqual(["draft"]);
    expect(f.from).toBe("");
  });

  it("toggles statuses", () => {
    expect(toggleStatus(["draft"], "approved")).toEqual(["draft", "approved"]);
    expect(toggleStatus(["draft", "approved"], "draft")).toEqual(["approved"]);
  });
});

describe("library rules", () => {
  it("requires typography check for posters only", () => {
    expect(requiresTypographyCheck({ subtype: "poster" })).toBe(true);
    expect(requiresTypographyCheck({ subtype: "facebook_post" })).toBe(false);
  });

  it("normalizes tags", () => {
    expect(normalizeTags(" Tết, tết ,Khuyến mãi,, ")).toEqual(["tết", "khuyến mãi"]);
  });

  it("picks the preview file", () => {
    expect(previewFile([file({ fileId: "t", role: "thumbnail" }), file({ fileId: "p" })])?.fileId).toBe("p");
    expect(previewFile([file({ url: null })])).toBeNull();
    expect(previewFile([file({ kind: "subtitle", role: "subtitle" })])).toBeNull();
    expect(previewFile([file({ fileId: "t", role: "thumbnail" }), file({ fileId: "v", role: "variant" })])?.fileId).toBe("v");
  });

  it("lists every full-size image of a version in order", () => {
    const files = [
      file({ fileId: "v2", role: "variant", sortOrder: 1 }),
      file({ fileId: "t1", role: "thumbnail", sortOrder: 0 }),
      file({ fileId: "v1", role: "variant", sortOrder: 0 }),
      file({ fileId: "x", role: "variant", sortOrder: 2, url: null }),
    ];
    expect(imageFiles(files).map((f) => f.fileId)).toEqual(["v1", "v2"]);
  });

  it("extracts version text per type", () => {
    expect(versionText("content", { title: "A", body_text: "B", hashtags: ["#x", "#y"] })).toBe("A\n\nB\n\n#x #y");
    expect(versionText("image", { prompt: "ảnh bánh" })).toBe("ảnh bánh");
    expect(versionText("video", { prompt: "p", script: "s" })).toBe("p\n\ns");
  });
});
