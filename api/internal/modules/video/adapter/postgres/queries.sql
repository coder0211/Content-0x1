-- name: CreateProject :exec
INSERT INTO video_projects (asset_id, brand_id, campaign_id, title, prompt, script, settings, current_job_id,
    voice_json, subtitle_json, output_json, created_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13);

-- name: GetProject :one
SELECT * FROM video_projects WHERE asset_id = $1;

-- name: GetProjectForUpdate :one
SELECT * FROM video_projects WHERE asset_id = $1 FOR UPDATE;

-- name: UpdateProject :exec
UPDATE video_projects
SET title = $2, prompt = $3, script = $4, settings = $5, current_job_id = $6,
    voice_json = $7, subtitle_json = $8, output_json = $9
WHERE asset_id = $1;

-- name: ListProjects :many
SELECT * FROM video_projects
WHERE (sqlc.narg('created_by')::uuid IS NULL OR created_by = sqlc.narg('created_by')::uuid)
  AND (sqlc.narg('campaign_id')::uuid IS NULL OR campaign_id = sqlc.narg('campaign_id')::uuid)
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
       OR (created_at, asset_id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY created_at DESC, asset_id DESC
LIMIT sqlc.arg('lim');

-- name: ListScenes :many
SELECT * FROM video_scenes WHERE asset_id = $1 AND deleted_at IS NULL ORDER BY sequence_no;

-- name: GetScene :one
SELECT * FROM video_scenes WHERE id = $1 AND deleted_at IS NULL;

-- name: OffsetSceneSequences :exec
UPDATE video_scenes SET sequence_no = sequence_no + 1000 WHERE asset_id = $1 AND deleted_at IS NULL;

-- name: InsertScene :exec
INSERT INTO video_scenes (id, asset_id, sequence_no, duration_ms, visual_prompt, narration, subtitle_text,
    source_file_id, clip_file_id, status, attempt_count, last_error, provider_job_id, usage_recorded_for,
    thumbnail_file_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $16);

-- name: UpdateScene :exec
UPDATE video_scenes
SET sequence_no = $2, duration_ms = $3, visual_prompt = $4, narration = $5, subtitle_text = $6,
    source_file_id = $7, clip_file_id = $8, status = $9, attempt_count = $10, last_error = $11,
    provider_job_id = $12, usage_recorded_for = $13, thumbnail_file_id = $14
WHERE id = $1 AND deleted_at IS NULL;

-- name: SoftDeleteScene :exec
UPDATE video_scenes SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL;
