-- +goose Up
-- ACS-023: music licenses live on the brand asset (the brand module may not
-- write asset_files, which belongs to the file module). Music must carry
-- {source, license_type, ...}; videos refuse tracks without it.
ALTER TABLE brand_assets ADD COLUMN license_json JSONB;
ALTER TABLE brand_assets ADD CONSTRAINT chk_brand_assets_music_license
    CHECK (asset_type <> 'music' OR (license_json IS NOT NULL AND license_json ? 'source' AND license_json ? 'license_type'));
CREATE INDEX idx_brands_created ON brands (created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_products_brand_created ON products (brand_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_products_brand_created;
DROP INDEX IF EXISTS idx_brands_created;
ALTER TABLE brand_assets DROP CONSTRAINT IF EXISTS chk_brand_assets_music_license;
ALTER TABLE brand_assets DROP COLUMN IF EXISTS license_json;
