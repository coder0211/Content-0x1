import { http } from "@/shared/api/http";
import type { ID, Page } from "@/shared/kernel";
import type { BrandQuery, BrandRepository, UpdateBrandInput } from "../application/ports";
import { ASSET_UPLOAD } from "../domain/rules";
import type { AddAssetInput, Brand, BrandAsset, BrandProfile, BrandVersion, Product, ProductInput, ProductStatus } from "../domain/types";
import {
  fromLicense,
  fromProductInput,
  fromProfile,
  toAsset,
  toBrand,
  toProduct,
  toVersion,
  type BrandAssetDto,
  type BrandDto,
  type BrandVersionDto,
  type ProductDto,
  type UploadTicketDto,
} from "./dto";

export class HttpBrandRepository implements BrandRepository {
  async list(q: BrandQuery): Promise<Page<Brand>> {
    const p = await http.page<BrandDto>("/brands", { query: { q: q.q, cursor: q.cursor, limit: q.limit ?? 20 } });
    return { items: p.items.map(toBrand), nextCursor: p.nextCursor };
  }

  async get(id: ID): Promise<Brand> {
    return toBrand(await http.get<BrandDto>(`/brands/${id}`));
  }

  async create(input: { name: string; profile: BrandProfile }): Promise<Brand> {
    return toBrand(await http.post<BrandDto>("/brands", { name: input.name, profile: fromProfile(input.profile) }));
  }

  async update(id: ID, input: UpdateBrandInput): Promise<Brand> {
    const body: Record<string, unknown> = { expected_version_no: input.expectedVersionNo };
    if (input.name !== undefined) body.name = input.name;
    if (input.profile) body.profile = fromProfile(input.profile);
    return toBrand(await http.patch<BrandDto>(`/brands/${id}`, body));
  }

  async versions(id: ID, cursor?: string | null): Promise<Page<BrandVersion>> {
    const p = await http.page<BrandVersionDto>(`/brands/${id}/versions`, { query: { cursor, limit: 50 } });
    return { items: p.items.map(toVersion), nextCursor: p.nextCursor };
  }

  async products(brandId: ID, status?: ProductStatus): Promise<Product[]> {
    const p = await http.page<ProductDto>(`/brands/${brandId}/products`, { query: { status, limit: 100 } });
    return p.items.map(toProduct);
  }

  async createProduct(brandId: ID, input: ProductInput): Promise<Product> {
    return toProduct(await http.post<ProductDto>(`/brands/${brandId}/products`, fromProductInput(input)));
  }

  async updateProduct(id: ID, input: ProductInput): Promise<Product> {
    return toProduct(await http.patch<ProductDto>(`/products/${id}`, fromProductInput(input)));
  }

  async assets(brandId: ID): Promise<BrandAsset[]> {
    const p = await http.page<BrandAssetDto>(`/brands/${brandId}/assets`);
    return p.items.map(toAsset);
  }

  async addAsset(brandId: ID, input: AddAssetInput): Promise<BrandAsset> {
    const ticket = await http.post<UploadTicketDto>("/files/upload-url", {
      filename: input.file.name,
      mime_type: input.file.type || "application/octet-stream",
      size_bytes: input.file.size,
      purpose: ASSET_UPLOAD[input.assetType].purpose,
    });
    const put = await fetch(ticket.url, { method: ticket.method, headers: ticket.headers, body: input.file });
    if (!put.ok) throw new Error(`upload failed: ${put.status}`);
    await http.post(`/files/${ticket.file_id}/complete`);
    const body: Record<string, unknown> = { file_id: ticket.file_id, asset_type: input.assetType, label: input.label };
    if (input.license) body.license = fromLicense(input.license);
    return toAsset(await http.post<BrandAssetDto>(`/brands/${brandId}/assets`, body));
  }

  async deleteAsset(brandId: ID, assetId: ID): Promise<void> {
    await http.delete(`/brands/${brandId}/assets/${assetId}`);
  }
}
