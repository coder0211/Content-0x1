-- +goose Up
-- Image Studio read model: one row per image request ("đợt tạo") linking the
-- job to the asset/version it produced (ACS-051). No FKs to ai_jobs /
-- content_assets / campaigns: those rows belong to other modules (and may be
-- produced by placeholder modules during development).
CREATE TABLE image_batches (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL UNIQUE,
    operation TEXT NOT NULL CHECK (operation IN ('generate','edit','variation')),
    campaign_id UUID,
    brand_id UUID NOT NULL,
    asset_id UUID,
    version_no INT,
    source_file_id UUID,
    prompt TEXT NOT NULL DEFAULT '',
    preset TEXT NOT NULL,
    aspect_ratio TEXT NOT NULL,
    quality TEXT NOT NULL,
    image_count INT NOT NULL CHECK (image_count BETWEEN 1 AND 4),
    model_code TEXT NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_image_batches_user ON image_batches (created_by, created_at DESC, id DESC);
CREATE INDEX idx_image_batches_campaign ON image_batches (campaign_id, created_at DESC, id DESC) WHERE campaign_id IS NOT NULL;
CREATE INDEX idx_image_batches_asset ON image_batches (asset_id, created_at DESC, id DESC) WHERE asset_id IS NOT NULL;

-- +goose Down
DROP TABLE image_batches;
