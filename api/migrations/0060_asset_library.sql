-- Asset module additions (ACS-080, ACS-082, ACS-085).
-- +goose Up
-- FR-AST-06: free-text usage-rights note (music source, reference image licence).
ALTER TABLE content_assets ADD COLUMN usage_rights TEXT;
-- Latest submit-review time: review queue order (oldest first) and dashboard.
ALTER TABLE content_assets ADD COLUMN submitted_at TIMESTAMPTZ;

-- Library keyset pagination (created_at DESC, id DESC) over live assets.
CREATE INDEX idx_assets_live_created ON content_assets (created_at DESC, id DESC) WHERE deleted_at IS NULL;
-- Review queue: pending assets by submission time.
CREATE INDEX idx_assets_review_queue ON content_assets (submitted_at, id) WHERE status = 'pending_review' AND deleted_at IS NULL;
-- "My assets" filters / dashboard counters.
CREATE INDEX idx_assets_creator_status ON content_assets (created_by, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_asset_versions_job ON asset_versions (generated_by_job_id) WHERE generated_by_job_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_asset_versions_job;
DROP INDEX IF EXISTS idx_assets_creator_status;
DROP INDEX IF EXISTS idx_assets_review_queue;
DROP INDEX IF EXISTS idx_assets_live_created;
ALTER TABLE content_assets DROP COLUMN IF EXISTS submitted_at;
ALTER TABLE content_assets DROP COLUMN IF EXISTS usage_rights;
