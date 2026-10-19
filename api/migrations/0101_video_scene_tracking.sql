-- +goose Up
-- Scene-level provider tracking: the submission of the current attempt, the
-- submission whose usage was already recorded (a retried step never bills
-- twice) and a preview thumbnail.
ALTER TABLE video_scenes
    ADD COLUMN provider_job_id TEXT,
    ADD COLUMN usage_recorded_for TEXT,
    ADD COLUMN thumbnail_file_id UUID REFERENCES asset_files(id);
CREATE INDEX idx_video_scenes_asset ON video_scenes (asset_id, sequence_no);

-- +goose Down
DROP INDEX IF EXISTS idx_video_scenes_asset;
ALTER TABLE video_scenes
    DROP COLUMN IF EXISTS thumbnail_file_id,
    DROP COLUMN IF EXISTS usage_recorded_for,
    DROP COLUMN IF EXISTS provider_job_id;
