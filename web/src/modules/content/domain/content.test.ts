import {
  activeWarnings,
  applyDraft,
  defaultFormat,
  findWarning,
  isDirty,
  parseHashtags,
  parseTones,
  renderHtml,
  variantLabel,
  videoStudioLink,
  type ContentBody,
} from "./content";

const base: ContentBody = {
  title: "Tiêu đề",
  hook: "Hook",
  bodyText: "Giảm 50% hôm nay. Rẻ nhất Hà Nội.",
  bodyHtml: "",
  cta: "Đăng ký",
  hashtags: ["#a"],
  platform: "facebook",
  format: "facebook_post",
  language: "vi",
  scenes: [],
  warnings: [
    { type: "unverified_claim", text: "giảm 50%" },
    { type: "forbidden_word", text: "rẻ nhất" },
  ],
  provenance: null,
};

describe("content domain", () => {
  it("labels variants A, B, C", () => {
    expect([0, 1, 2].map(variantLabel)).toEqual(["A", "B", "C"]);
  });

  it("maps platform to default format", () => {
    expect(defaultFormat("tiktok")).toBe("tiktok_script");
    expect(defaultFormat("zalo")).toBe("caption");
    expect(defaultFormat("facebook")).toBe("facebook_post");
  });

  it("parses tones and hashtags", () => {
    expect(parseTones("thân thiện, tin cậy, , thân thiện")).toEqual([
      "thân thiện",
      "tin cậy",
    ]);
    expect(parseHashtags("#a b,#a  c")).toEqual(["#a", "#b", "#c"]);
  });

  it("renders escaped HTML paragraphs", () => {
    expect(renderHtml("Xin <b>chào</b>\ndòng 2\n\nđoạn 2")).toBe(
      "<p>Xin &lt;b&gt;chào&lt;/b&gt;<br>dòng 2</p><p>đoạn 2</p>",
    );
  });

  it("drops warnings that the user edited out", () => {
    expect(activeWarnings(base.warnings, [base.bodyText])).toHaveLength(2);
    const next = applyDraft(base, {
      ...base,
      bodyText: "Ưu đãi hôm nay. Rẻ nhất Hà Nội.",
    });
    expect(next.warnings).toEqual([
      { type: "forbidden_word", text: "rẻ nhất" },
    ]);
    expect(next.bodyHtml).toBe("<p>Ưu đãi hôm nay. Rẻ nhất Hà Nội.</p>");
  });

  it("finds a warning position case-insensitively", () => {
    expect(findWarning(base.bodyText, base.warnings[0]!)).toEqual({
      start: 0,
      end: 8,
    });
    expect(findWarning("không có", base.warnings[0]!)).toBeNull();
  });

  it("detects dirty drafts", () => {
    expect(isDirty(base, { ...base })).toBe(false);
    expect(isDirty(base, { ...base, hashtags: ["#b"] })).toBe(true);
  });

  it("builds the Video Studio link", () => {
    expect(videoStudioLink("abc")).toBe("/videos?script_from_asset_id=abc");
    expect(videoStudioLink("abc", "c1")).toBe("/videos?script_from_asset_id=abc&campaign_id=c1");
  });
});
