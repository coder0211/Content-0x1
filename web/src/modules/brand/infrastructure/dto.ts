import type {
  Brand,
  BrandAsset,
  BrandAssetType,
  BrandProfile,
  BrandVersion,
  MusicLicense,
  OfferStatus,
  Product,
  ProductFacts,
  ProductInput,
  ProductStatus,
  ProofType,
} from "../domain/types";

export interface BrandProfileDto {
  description: string;
  colors: { name: string; hex: string }[];
  typography: { heading: string; body: string };
  tone_of_voice: string[];
  preferred_words: string[];
  forbidden_words: string[];
  default_ctas: string[];
  audience_personas: { name: string; pain_points: string[]; motivations: string[] }[];
  notes: string;
}

export interface BrandVersionDto {
  id: string;
  version_no: number;
  profile: BrandProfileDto;
  created_by: string | null;
  created_at: string;
}

export interface BrandDto {
  id: string;
  name: string;
  current_version: BrandVersionDto;
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface ProductFactsDto {
  usp: string[];
  price: { amount: number; currency: string; note: string } | null;
  offers: { text: string; valid_from: string | null; valid_to: string | null; status?: OfferStatus }[];
  policies: string[];
  proof: { type: string; text: string; source: string; verified: boolean }[];
}

export interface ProductDto {
  id: string;
  brand_id: string;
  name: string;
  description: string;
  facts: ProductFactsDto;
  status: ProductStatus;
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface MusicLicenseDto {
  source: string;
  license_type: string;
  expires_at?: string;
  attribution?: string;
  notes?: string;
}

export interface BrandAssetDto {
  id: string;
  brand_id: string;
  file_id: string;
  asset_type: BrandAssetType;
  label: string;
  license: MusicLicenseDto | null;
  license_valid: boolean;
  file: {
    mime_type: string;
    original_name: string;
    size_bytes: number;
    width?: number;
    height?: number;
    duration_ms?: number;
    url: string | null;
    url_expires_at: string | null;
  } | null;
  created_by: string | null;
  created_at: string;
}

export interface UploadTicketDto {
  file_id: string;
  url: string;
  method: "PUT";
  headers: Record<string, string>;
  expires_at: string;
}

const arr = <T>(v: T[] | null | undefined): T[] => v ?? [];

export function toProfile(d: Partial<BrandProfileDto> | null | undefined): BrandProfile {
  return {
    description: d?.description ?? "",
    colors: arr(d?.colors).map((c) => ({ name: c.name, hex: c.hex })),
    typography: { heading: d?.typography?.heading ?? "", body: d?.typography?.body ?? "" },
    toneOfVoice: arr(d?.tone_of_voice),
    preferredWords: arr(d?.preferred_words),
    forbiddenWords: arr(d?.forbidden_words),
    defaultCtas: arr(d?.default_ctas),
    audiencePersonas: arr(d?.audience_personas).map((p) => ({
      name: p.name,
      painPoints: arr(p.pain_points),
      motivations: arr(p.motivations),
    })),
    notes: d?.notes ?? "",
  };
}

export function fromProfile(p: BrandProfile): BrandProfileDto {
  return {
    description: p.description,
    colors: p.colors.map((c) => ({ name: c.name, hex: c.hex })),
    typography: { heading: p.typography.heading, body: p.typography.body },
    tone_of_voice: p.toneOfVoice,
    preferred_words: p.preferredWords,
    forbidden_words: p.forbiddenWords,
    default_ctas: p.defaultCtas,
    audience_personas: p.audiencePersonas.map((x) => ({ name: x.name, pain_points: x.painPoints, motivations: x.motivations })),
    notes: p.notes,
  };
}

export function toVersion(d: BrandVersionDto): BrandVersion {
  return { id: d.id, versionNo: d.version_no, profile: toProfile(d.profile), createdBy: d.created_by, createdAt: d.created_at };
}

export function toBrand(d: BrandDto): Brand {
  return { id: d.id, name: d.name, currentVersion: toVersion(d.current_version), createdAt: d.created_at, updatedAt: d.updated_at };
}

export function toFacts(d: Partial<ProductFactsDto> | null | undefined): ProductFacts {
  return {
    usp: arr(d?.usp),
    price: d?.price ? { amount: Number(d.price.amount), currency: d.price.currency, note: d.price.note ?? "" } : null,
    offers: arr(d?.offers).map((o) => ({ text: o.text, validFrom: o.valid_from ?? null, validTo: o.valid_to ?? null, status: o.status })),
    policies: arr(d?.policies),
    proof: arr(d?.proof).map((p) => ({ type: (p.type || "") as ProofType | "", text: p.text, source: p.source ?? "", verified: !!p.verified })),
  };
}

export function fromFacts(f: ProductFacts): ProductFactsDto {
  return {
    usp: f.usp,
    price: f.price ? { amount: f.price.amount, currency: f.price.currency, note: f.price.note } : null,
    offers: f.offers.map((o) => ({ text: o.text, valid_from: o.validFrom || null, valid_to: o.validTo || null })),
    policies: f.policies,
    proof: f.proof.map((p) => ({ type: p.type, text: p.text, source: p.source, verified: p.verified })),
  };
}

export function toProduct(d: ProductDto): Product {
  return {
    id: d.id,
    brandId: d.brand_id,
    name: d.name,
    description: d.description ?? "",
    facts: toFacts(d.facts),
    status: d.status,
    createdAt: d.created_at,
    updatedAt: d.updated_at,
  };
}

export function fromProductInput(i: ProductInput): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (i.name !== undefined) out.name = i.name;
  if (i.description !== undefined) out.description = i.description;
  if (i.facts !== undefined) out.facts = fromFacts(i.facts);
  if (i.status !== undefined) out.status = i.status;
  return out;
}

export function toLicense(d: MusicLicenseDto | null): MusicLicense | null {
  if (!d) return null;
  return { source: d.source, licenseType: d.license_type, expiresAt: d.expires_at, attribution: d.attribution, notes: d.notes };
}

export function fromLicense(l: MusicLicense): MusicLicenseDto {
  const out: MusicLicenseDto = { source: l.source, license_type: l.licenseType };
  if (l.expiresAt) out.expires_at = l.expiresAt;
  if (l.attribution) out.attribution = l.attribution;
  if (l.notes) out.notes = l.notes;
  return out;
}

export function toAsset(d: BrandAssetDto): BrandAsset {
  return {
    id: d.id,
    brandId: d.brand_id,
    fileId: d.file_id,
    assetType: d.asset_type,
    label: d.label ?? "",
    license: toLicense(d.license),
    licenseValid: d.license_valid,
    file: d.file ? { mimeType: d.file.mime_type, originalName: d.file.original_name, sizeBytes: d.file.size_bytes, url: d.file.url } : null,
    createdAt: d.created_at,
  };
}
