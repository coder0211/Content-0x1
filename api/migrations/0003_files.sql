-- +goose Up
CREATE TABLE asset_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    storage_key TEXT NOT NULL UNIQUE,         -- s3 key, no sensitive information
    original_name TEXT,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    checksum_sha256 TEXT,
    width INT, height INT, duration_ms INT,
    kind TEXT NOT NULL CHECK (kind IN ('image','video','audio','subtitle','document','other')),
    upload_status TEXT NOT NULL DEFAULT 'pending' CHECK (upload_status IN ('pending','ready','rejected')),
    source TEXT NOT NULL CHECK (source IN ('upload','generated','rendered')),
    parent_file_id UUID REFERENCES asset_files(id), -- variant/edit of which file
    license_json JSONB NOT NULL DEFAULT '{}',  -- source, license, expiry
    metadata JSONB NOT NULL DEFAULT '{}',
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_files_status_created ON asset_files (upload_status, created_at);

-- +goose Down
DROP TABLE IF EXISTS asset_files;
