import type { ID, Page } from "@/shared/kernel";
import type { AddAssetInput, Brand, BrandAsset, BrandProfile, BrandVersion, Product, ProductInput, ProductStatus } from "../domain/types";

export interface BrandQuery {
  q?: string;
  cursor?: string | null;
  limit?: number;
}

export interface UpdateBrandInput {
  name?: string;
  profile?: BrandProfile;
  expectedVersionNo: number;
}

export interface BrandRepository {
  list(q: BrandQuery): Promise<Page<Brand>>;
  get(id: ID): Promise<Brand>;
  create(input: { name: string; profile: BrandProfile }): Promise<Brand>;
  update(id: ID, input: UpdateBrandInput): Promise<Brand>;
  versions(id: ID, cursor?: string | null): Promise<Page<BrandVersion>>;
  products(brandId: ID, status?: ProductStatus): Promise<Product[]>;
  createProduct(brandId: ID, input: ProductInput): Promise<Product>;
  updateProduct(id: ID, input: ProductInput): Promise<Product>;
  assets(brandId: ID): Promise<BrandAsset[]>;
  /** Uploads the file (presigned PUT via the file module) and attaches it to the brand. */
  addAsset(brandId: ID, input: AddAssetInput): Promise<BrandAsset>;
  deleteAsset(brandId: ID, assetId: ID): Promise<void>;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    brand: BrandRepository;
  }
}
