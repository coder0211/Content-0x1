-- name: InsertBatch :exec
INSERT INTO image_batches (id, job_id, operation, campaign_id, brand_id, asset_id, version_no, source_file_id,
    prompt, preset, aspect_ratio, quality, image_count, model_code, created_by, created_at)
VALUES (@id, @job_id, @operation, sqlc.narg(campaign_id), @brand_id, sqlc.narg(asset_id), sqlc.narg(version_no),
    sqlc.narg(source_file_id), @prompt, @preset, @aspect_ratio, @quality, @image_count, @model_code, @created_by, @created_at);

-- name: GetBatch :one
SELECT * FROM image_batches WHERE id = @id;

-- name: SetBatchResult :execrows
UPDATE image_batches SET asset_id = @asset_id, version_no = @version_no WHERE id = @id;

-- name: ListBatches :many
SELECT * FROM image_batches
WHERE (sqlc.narg(campaign_id)::uuid IS NULL OR campaign_id = sqlc.narg(campaign_id))
  AND (sqlc.narg(asset_id)::uuid IS NULL OR asset_id = sqlc.narg(asset_id))
  AND (sqlc.narg(created_by)::uuid IS NULL OR created_by = sqlc.narg(created_by))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;
