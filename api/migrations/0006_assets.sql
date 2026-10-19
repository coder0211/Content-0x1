-- +goose Up
CREATE TABLE content_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID REFERENCES campaigns(id),
    brand_id UUID NOT NULL REFERENCES brands(id),
    asset_type TEXT NOT NULL CHECK (asset_type IN ('content','image','video')),
    subtype TEXT,                              -- facebook_post, tiktok_script, poster, social_vertical...
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft','pending_review','changes_requested','approved','archived')),
    current_version_id UUID,
    source_asset_id UUID REFERENCES content_assets(id), -- transformed from which asset
    tags TEXT[] NOT NULL DEFAULT '{}',
    search_tsv TSVECTOR,                       -- updated when a version is created
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_assets_campaign_status ON content_assets (campaign_id, status);
CREATE INDEX idx_assets_type_created ON content_assets (asset_type, created_at DESC);
CREATE INDEX idx_assets_tags ON content_assets USING GIN (tags);
CREATE INDEX idx_assets_search ON content_assets USING GIN (search_tsv);

-- content_json per asset_type:
--  content: {title, hook, body_html, body_text, cta, hashtags[], platform, warnings[]}
--  image:   {prompt, preset, aspect_ratio, file_ids[]}
--  video:   {prompt, preset, aspect_ratio, script, scene_ids[], final_file_id, subtitle_file_id}
CREATE TABLE asset_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id UUID NOT NULL REFERENCES content_assets(id),
    version_no INT NOT NULL,
    content_json JSONB NOT NULL,
    change_note TEXT,
    generated_by_job_id UUID,                 -- FK added in 0008
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (asset_id, version_no)
);
ALTER TABLE content_assets ADD CONSTRAINT fk_assets_current_version
    FOREIGN KEY (current_version_id) REFERENCES asset_versions(id);

CREATE TABLE asset_version_files (
    version_id UUID NOT NULL REFERENCES asset_versions(id),
    file_id UUID NOT NULL REFERENCES asset_files(id),
    role TEXT NOT NULL CHECK (role IN ('primary','variant','thumbnail','subtitle','audio','preview')),
    sort_order INT NOT NULL DEFAULT 0,
    PRIMARY KEY (version_id, file_id)
);

CREATE TABLE video_scenes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id UUID NOT NULL REFERENCES content_assets(id),
    sequence_no INT NOT NULL,
    duration_ms INT NOT NULL CHECK (duration_ms > 0),
    visual_prompt TEXT NOT NULL,
    narration TEXT,
    subtitle_text TEXT,
    source_file_id UUID REFERENCES asset_files(id),   -- input image (image-to-video)
    clip_file_id UUID REFERENCES asset_files(id),     -- generated + normalized clip
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','generating','ready','failed')),
    attempt_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (asset_id, sequence_no) DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE approvals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id UUID NOT NULL REFERENCES content_assets(id),
    version_id UUID NOT NULL REFERENCES asset_versions(id),
    reviewer_id UUID NOT NULL REFERENCES users(id),
    decision TEXT NOT NULL CHECK (decision IN ('submitted','approved','changes_requested')),
    comment TEXT,
    checklist JSONB NOT NULL DEFAULT '{}',    -- e.g. {"typography_checked": true}
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_approvals_asset ON approvals (asset_id, created_at DESC);

CREATE TRIGGER trg_assets_updated BEFORE UPDATE ON content_assets FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_scenes_updated BEFORE UPDATE ON video_scenes FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS approvals;
DROP TABLE IF EXISTS video_scenes;
DROP TABLE IF EXISTS asset_version_files;
ALTER TABLE content_assets DROP CONSTRAINT IF EXISTS fk_assets_current_version;
DROP TABLE IF EXISTS asset_versions;
DROP TABLE IF EXISTS content_assets;
