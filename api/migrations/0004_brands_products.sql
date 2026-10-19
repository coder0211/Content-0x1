-- +goose Up
CREATE TABLE brands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    current_version_id UUID,                  -- FK added after brand_versions
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_brands_name ON brands (lower(name)) WHERE deleted_at IS NULL;

-- profile: {description, colors[], typography{}, tone_of_voice[], preferred_words[], forbidden_words[], default_ctas[], audience_personas[], notes}
CREATE TABLE brand_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id UUID NOT NULL REFERENCES brands(id),
    version_no INT NOT NULL,
    profile JSONB NOT NULL,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (brand_id, version_no)
);
ALTER TABLE brands ADD CONSTRAINT fk_brands_current_version
    FOREIGN KEY (current_version_id) REFERENCES brand_versions(id);

CREATE TABLE brand_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id UUID NOT NULL REFERENCES brands(id),
    file_id UUID NOT NULL REFERENCES asset_files(id),
    asset_type TEXT NOT NULL CHECK (asset_type IN ('logo','reference_image','font','document','sample_content','music')),
    label TEXT,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_brand_assets_brand ON brand_assets (brand_id, asset_type);

-- facts: {usp[], price{amount,currency,note}, offers[{text,valid_from,valid_to}], policies[], proof[{type,text,source,verified}]}
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id UUID NOT NULL REFERENCES brands(id),
    name TEXT NOT NULL,
    description TEXT,
    facts JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_products_brand ON products (brand_id) WHERE status = 'active';

CREATE TRIGGER trg_brands_updated BEFORE UPDATE ON brands FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_products_updated BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS brand_assets;
ALTER TABLE brands DROP CONSTRAINT IF EXISTS fk_brands_current_version;
DROP TABLE IF EXISTS brand_versions;
DROP TABLE IF EXISTS brands;
