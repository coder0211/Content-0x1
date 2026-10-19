import type { ActiveStatus, ID, ISODateTime } from "@/shared/kernel";

export type ModelKind = "text" | "image" | "video" | "voice";

export const MODEL_KINDS: readonly ModelKind[] = ["text", "image", "video", "voice"];

export const KIND_LABELS: Record<ModelKind, string> = {
  text: "Văn bản",
  image: "Ảnh",
  video: "Video",
  voice: "Giọng đọc",
};

/** ai_providers (SPEC-03). Never carries a secret value — only its reference name. */
export interface Provider {
  id: ID;
  code: string;
  name: string;
  kinds: ModelKind[];
  status: ActiveStatus;
  secretRef: string;
  /** `null` when the API does not report whether the referenced secret exists. */
  secretConfigured: boolean | null;
  config: Record<string, unknown>;
  updatedAt: ISODateTime | null;
}

export interface UpdateProviderInput {
  status?: ActiveStatus;
  secretRef?: string;
  config?: Record<string, unknown>;
}

/** ai_models (SPEC-03). */
export interface AiModel {
  id: ID;
  providerId: ID;
  providerCode: string | null;
  modelCode: string;
  kind: ModelKind;
  capabilities: Record<string, unknown>;
  concurrencyLimit: number;
  isDefault: boolean;
  status: ActiveStatus;
}

export interface UpdateModelInput {
  capabilities?: Record<string, unknown>;
  concurrencyLimit?: number;
  isDefault?: boolean;
  status?: ActiveStatus;
}

export interface CreateModelInput {
  providerId: ID;
  modelCode: string;
  kind: ModelKind;
  capabilities: Record<string, unknown>;
  concurrencyLimit: number;
  isDefault: boolean;
}

/** One model_pricing row (one unit of one pricing version). */
export interface PricingRow {
  id: ID;
  modelId: ID;
  pricingVersion: string;
  unit: string;
  unitPrice: number;
  currency: string;
  effectiveFrom: ISODateTime;
  effectiveTo: ISODateTime | null;
  /** Server-computed `is_current` (effective now); falls back to `effectiveTo === null`. */
  isCurrent?: boolean;
}

export interface PricingVersion {
  pricingVersion: string;
  effectiveFrom: ISODateTime;
  effectiveTo: ISODateTime | null;
  currency: string;
  rows: PricingRow[];
  /** effective_to is null → the version currently in force. */
  current: boolean;
}

export interface NewPricingInput {
  pricingVersion: string;
  /** RFC3339. */
  effectiveFrom: string;
  currency: string;
  prices: { unit: string; unitPrice: number }[];
}

/** Groups rows into versions, newest effective_from first. Old versions are read-only. */
export function groupPricing(rows: PricingRow[]): PricingVersion[] {
  const byVersion = new Map<string, PricingRow[]>();
  for (const r of rows) {
    const list = byVersion.get(r.pricingVersion) ?? [];
    list.push(r);
    byVersion.set(r.pricingVersion, list);
  }
  return [...byVersion.entries()]
    .map(([pricingVersion, list]) => {
      const first = list[0]!;
      return {
        pricingVersion,
        effectiveFrom: first.effectiveFrom,
        effectiveTo: first.effectiveTo,
        currency: first.currency,
        rows: [...list].sort((a, b) => a.unit.localeCompare(b.unit)),
        current: list.every((r) => r.isCurrent ?? r.effectiveTo === null),
      };
    })
    .sort((a, b) => b.effectiveFrom.localeCompare(a.effectiveFrom));
}

/** Suggested units per model kind (SPEC-03 model_pricing.unit). */
export const UNIT_SUGGESTIONS: Record<ModelKind, string[]> = {
  text: ["input_token", "output_token"],
  image: ["image_1024_standard", "image_1024_hd"],
  video: ["video_second_720p", "video_second_1080p"],
  voice: ["char"],
};

export interface PricingDraftRow {
  unit: string;
  unitPrice: string;
}

export interface PricingDraft {
  pricingVersion: string;
  effectiveFrom: string; // YYYY-MM-DD
  currency: string;
  rows: PricingDraftRow[];
}

export type PricingDraftErrors = Partial<Record<"pricing_version" | "effective_from" | "rows", string>>;

export function validatePricingDraft(d: PricingDraft, existingVersions: readonly string[]): PricingDraftErrors {
  const e: PricingDraftErrors = {};
  const v = d.pricingVersion.trim();
  if (!v) e.pricing_version = "Nhập mã phiên bản giá.";
  else if (existingVersions.includes(v)) e.pricing_version = "Phiên bản này đã tồn tại. Bảng giá cũ không sửa được — hãy đặt mã mới.";
  if (!d.effectiveFrom) e.effective_from = "Chọn ngày hiệu lực.";
  const units = d.rows.map((r) => r.unit.trim());
  if (d.rows.length === 0) e.rows = "Thêm ít nhất một đơn giá.";
  else if (units.some((u) => !u)) e.rows = "Mỗi dòng cần đơn vị tính.";
  else if (new Set(units).size !== units.length) e.rows = "Đơn vị tính bị trùng.";
  else if (d.rows.some((r) => !isNonNegativeNumber(r.unitPrice))) e.rows = "Đơn giá phải là số không âm.";
  return e;
}

export function toNewPricingInput(d: PricingDraft): NewPricingInput {
  return {
    pricingVersion: d.pricingVersion.trim(),
    effectiveFrom: new Date(`${d.effectiveFrom}T00:00:00`).toISOString(),
    currency: d.currency,
    prices: d.rows.map((r) => ({ unit: r.unit.trim(), unitPrice: Number(r.unitPrice) })),
  };
}

function isNonNegativeNumber(s: string): boolean {
  if (s.trim() === "") return false;
  const n = Number(s);
  return Number.isFinite(n) && n >= 0;
}

const SECRET_KEY_RE = /(api[_-]?key|secret|token|password|credential)/i;

/**
 * Parses the provider config editor. config_json must never hold secrets (SPEC-03):
 * keys that look like credentials are rejected — use secret_ref instead.
 */
export function parseConfigJson(text: string): { ok: true; value: Record<string, unknown> } | { ok: false; error: string } {
  let v: unknown;
  try {
    v = text.trim() === "" ? {} : JSON.parse(text);
  } catch {
    return { ok: false, error: "JSON không hợp lệ." };
  }
  if (typeof v !== "object" || v === null || Array.isArray(v)) return { ok: false, error: "Cấu hình phải là một object JSON." };
  const bad = findSecretKey(v as Record<string, unknown>);
  if (bad) return { ok: false, error: `Không lưu khóa bí mật trong cấu hình (trường "${bad}"). Dùng secret_ref.` };
  return { ok: true, value: v as Record<string, unknown> };
}

function findSecretKey(o: Record<string, unknown>): string | null {
  for (const [k, val] of Object.entries(o)) {
    if (SECRET_KEY_RE.test(k)) return k;
    if (typeof val === "object" && val !== null && !Array.isArray(val)) {
      const nested = findSecretKey(val as Record<string, unknown>);
      if (nested) return `${k}.${nested}`;
    }
  }
  return null;
}

/** secret_ref is a reference NAME (e.g. PROVIDER_OPENAI_API_KEY), never a key value. */
export function validateSecretRef(ref: string): string | null {
  const r = ref.trim();
  if (!r) return "Nhập tên secret.";
  if (/^sk-|^AIza/.test(r)) return "Có vẻ đây là giá trị khóa. Chỉ nhập tên secret.";
  if (!/^[A-Za-z][A-Za-z0-9_./:-]{1,127}$/.test(r)) return "Chỉ nhập tên biến/secret (chữ, số, _ . / : -), không dán giá trị khóa.";
  return null;
}

export function parseCapabilities(text: string): { ok: true; value: Record<string, unknown> } | { ok: false; error: string } {
  try {
    const v: unknown = text.trim() === "" ? {} : JSON.parse(text);
    if (typeof v !== "object" || v === null || Array.isArray(v)) return { ok: false, error: "Capabilities phải là một object JSON." };
    return { ok: true, value: v as Record<string, unknown> };
  } catch {
    return { ok: false, error: "JSON không hợp lệ." };
  }
}
