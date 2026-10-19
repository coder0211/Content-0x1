import type { ID, ISODateTime } from "@/shared/kernel";

export interface BrandColor {
  name: string;
  hex: string;
}

export interface Persona {
  name: string;
  painPoints: string[];
  motivations: string[];
}

/** brand_versions.profile (SPEC-04 §2.2). */
export interface BrandProfile {
  description: string;
  colors: BrandColor[];
  typography: { heading: string; body: string };
  toneOfVoice: string[];
  preferredWords: string[];
  forbiddenWords: string[];
  defaultCtas: string[];
  audiencePersonas: Persona[];
  notes: string;
}

export interface BrandVersion {
  id: ID;
  versionNo: number;
  profile: BrandProfile;
  createdBy: ID | null;
  createdAt: ISODateTime;
}

export interface Brand {
  id: ID;
  name: string;
  currentVersion: BrandVersion;
  createdAt: ISODateTime;
  updatedAt: ISODateTime;
}

export type OfferStatus = "active" | "expired" | "upcoming";

export interface Offer {
  text: string;
  validFrom: string | null; // YYYY-MM-DD
  validTo: string | null;
  /** Computed by the server; never sent. */
  status?: OfferStatus;
}

export const PROOF_TYPES = ["testimonial", "statistic", "award", "certification", "media", "other"] as const;
export type ProofType = (typeof PROOF_TYPES)[number];

export interface Proof {
  type: ProofType | "";
  text: string;
  source: string;
  verified: boolean;
}

export interface ProductFacts {
  usp: string[];
  price: { amount: number; currency: string; note: string } | null;
  offers: Offer[];
  policies: string[];
  proof: Proof[];
}

export type ProductStatus = "active" | "archived";

export interface Product {
  id: ID;
  brandId: ID;
  name: string;
  description: string;
  facts: ProductFacts;
  status: ProductStatus;
  createdAt: ISODateTime;
  updatedAt: ISODateTime;
}

export const ASSET_TYPES = ["logo", "reference_image", "document", "sample_content", "font", "music"] as const;
export type BrandAssetType = (typeof ASSET_TYPES)[number];

export interface MusicLicense {
  source: string;
  licenseType: string;
  expiresAt?: string;
  attribution?: string;
  notes?: string;
}

export interface BrandAsset {
  id: ID;
  brandId: ID;
  fileId: ID;
  assetType: BrandAssetType;
  label: string;
  license: MusicLicense | null;
  licenseValid: boolean;
  file: {
    mimeType: string;
    originalName: string;
    sizeBytes: number;
    url: string | null;
  } | null;
  createdAt: ISODateTime;
}

export interface ProductInput {
  name?: string;
  description?: string;
  facts?: ProductFacts;
  status?: ProductStatus;
}

export interface AddAssetInput {
  file: File;
  assetType: BrandAssetType;
  label: string;
  license?: MusicLicense;
}
