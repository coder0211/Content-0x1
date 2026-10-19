-- +goose Up
-- Scenes removed from a storyboard may still be referenced by past job
-- steps (ai_job_steps.scene_id), so they are soft-deleted. Sequence numbers
-- are unique among live scenes only; the repository reorders in two phases
-- (offset, then final numbers) inside one transaction.
ALTER TABLE video_scenes ADD COLUMN deleted_at TIMESTAMPTZ;
ALTER TABLE video_scenes DROP CONSTRAINT IF EXISTS video_scenes_asset_id_sequence_no_key;
CREATE UNIQUE INDEX uq_video_scenes_live_seq ON video_scenes (asset_id, sequence_no) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS uq_video_scenes_live_seq;
UPDATE video_scenes SET sequence_no = -sequence_no - 1000 * (1 + (abs(hashtext(id::text)) % 100000)) WHERE deleted_at IS NOT NULL;
ALTER TABLE video_scenes ADD CONSTRAINT video_scenes_asset_id_sequence_no_key UNIQUE (asset_id, sequence_no) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE video_scenes DROP COLUMN deleted_at;
