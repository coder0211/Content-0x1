import type { BrandAssetType, BrandProfile, Offer, OfferStatus, ProductFacts } from "./types";

const HEX = /^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i;

/** FR-BRD-01: #RGB or #RRGGBB (mirrors the backend). */
export function isHexColor(s: string): boolean {
  return HEX.test(s.trim());
}

export function emptyProfile(): BrandProfile {
  return {
    description: "",
    colors: [],
    typography: { heading: "", body: "" },
    toneOfVoice: [],
    preferredWords: [],
    forbiddenWords: [],
    defaultCtas: [],
    audiencePersonas: [],
    notes: "",
  };
}

export function emptyFacts(): ProductFacts {
  return { usp: [], price: null, offers: [], policies: [], proof: [] };
}

/** Business day "today" in Vietnam (UTC+7), YYYY-MM-DD — same rule as the backend. */
export function businessToday(now: Date = new Date()): string {
  return new Date(now.getTime() + 7 * 3600_000).toISOString().slice(0, 10);
}

/** active | expired | upcoming for inclusive YYYY-MM-DD bounds. */
export function offerStatus(o: Pick<Offer, "validFrom" | "validTo">, today: string = businessToday()): OfferStatus {
  if (o.validTo && o.validTo < today) return "expired";
  if (o.validFrom && o.validFrom > today) return "upcoming";
  return "active";
}

/** Offers that may be used in prompts / picked as the verified offer of a brief. */
export function activeOffers(facts: ProductFacts, today?: string): Offer[] {
  return facts.offers.filter((o) => (o.status ?? offerStatus(o, today)) === "active");
}

/** Client-side profile checks (the server re-validates everything). Keys follow server field paths. */
export function validateProfile(p: BrandProfile): Record<string, string> {
  const errors: Record<string, string> = {};
  p.colors.forEach((c, i) => {
    if (!c.name.trim()) errors[`profile.colors[${i}].name`] = "required";
    if (!isHexColor(c.hex)) errors[`profile.colors[${i}].hex`] = "invalid_hex";
  });
  p.audiencePersonas.forEach((ps, i) => {
    if (!ps.name.trim()) errors[`profile.audience_personas[${i}].name`] = "required";
  });
  const forbidden = new Set(p.forbiddenWords.map((w) => w.toLowerCase()));
  if (p.preferredWords.some((w) => forbidden.has(w.toLowerCase()))) {
    errors["profile.preferred_words"] = "conflicts_with_forbidden_words";
  }
  return errors;
}

/** Asset types offered in the upload form (fonts are not an accepted upload purpose yet). */
export const UPLOADABLE_ASSET_TYPES: BrandAssetType[] = ["logo", "reference_image", "document", "sample_content", "music"];

/** Upload purpose (file module) and accepted MIME types per brand asset type. */
export const ASSET_UPLOAD: Record<BrandAssetType, { purpose: string; accept: string }> = {
  logo: { purpose: "brand_logo", accept: "image/png,image/jpeg,image/svg+xml" },
  reference_image: { purpose: "reference_image", accept: "image/png,image/jpeg,image/webp" },
  document: { purpose: "document", accept: "application/pdf,image/png,image/jpeg" },
  sample_content: { purpose: "document", accept: "application/pdf,image/png,image/jpeg" },
  font: { purpose: "document", accept: ".ttf,.otf,.woff,.woff2" },
  music: { purpose: "music", accept: "audio/mpeg,audio/mp4,audio/wav,audio/x-wav" },
};

export const ASSET_TYPE_LABELS: Record<BrandAssetType, string> = {
  logo: "Logo",
  reference_image: "Ảnh tham chiếu",
  document: "Tài liệu",
  sample_content: "Content mẫu",
  font: "Font",
  music: "Nhạc",
};

export const ISSUE_MESSAGES: Record<string, string> = {
  required: "Bắt buộc nhập.",
  invalid_hex: "Mã màu phải dạng #RRGGBB.",
  too_long: "Quá dài.",
  too_many: "Quá nhiều mục.",
  duplicate: "Đã tồn tại.",
  invalid: "Không hợp lệ.",
  invalid_date: "Ngày không hợp lệ.",
  before_valid_from: "Ngày kết thúc phải sau ngày bắt đầu.",
  negative: "Không được âm.",
  conflicts_with_forbidden_words: "Có từ vừa nằm trong danh sách nên dùng vừa bị cấm.",
};

export function issueMessage(issue: string | undefined): string | null {
  if (!issue) return null;
  return ISSUE_MESSAGES[issue] ?? "Không hợp lệ.";
}
