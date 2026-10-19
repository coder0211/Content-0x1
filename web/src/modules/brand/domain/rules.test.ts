import { activeOffers, businessToday, emptyFacts, isHexColor, offerStatus, validateProfile, emptyProfile } from "./rules";

describe("brand rules", () => {
  it("validates hex colors like the backend", () => {
    expect(isHexColor("#0055FF")).toBe(true);
    expect(isHexColor("#fff")).toBe(true);
    expect(isHexColor("0055FF")).toBe(false);
    expect(isHexColor("#GGG")).toBe(false);
    expect(isHexColor("blue")).toBe(false);
  });

  it("computes offer status with inclusive dates", () => {
    expect(offerStatus({ validFrom: null, validTo: "2026-10-18" }, "2026-10-19")).toBe("expired");
    expect(offerStatus({ validFrom: null, validTo: "2026-10-19" }, "2026-10-19")).toBe("active");
    expect(offerStatus({ validFrom: "2026-10-20", validTo: null }, "2026-10-19")).toBe("upcoming");
    expect(offerStatus({ validFrom: null, validTo: null }, "2026-10-19")).toBe("active");
  });

  it("keeps only active offers", () => {
    const facts = {
      ...emptyFacts(),
      offers: [
        { text: "A", validFrom: null, validTo: "2026-01-01" },
        { text: "B", validFrom: null, validTo: null },
      ],
    };
    expect(activeOffers(facts, "2026-10-19").map((o) => o.text)).toEqual(["B"]);
  });

  it("uses the Vietnam business day", () => {
    expect(businessToday(new Date("2026-10-19T18:00:00Z"))).toBe("2026-10-20");
  });

  it("reports profile issues by server field path", () => {
    const p = { ...emptyProfile(), colors: [{ name: "", hex: "red" }], preferredWords: ["rẻ"], forbiddenWords: ["Rẻ"] };
    expect(validateProfile(p)).toEqual({
      "profile.colors[0].name": "required",
      "profile.colors[0].hex": "invalid_hex",
      "profile.preferred_words": "conflicts_with_forbidden_words",
    });
  });
});
