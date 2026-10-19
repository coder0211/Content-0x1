-- name: CreateFile :exec
INSERT INTO asset_files (id, storage_key, original_name, mime_type, size_bytes, checksum_sha256,
                         width, height, duration_ms, kind, upload_status, source, parent_file_id,
                         license_json, metadata, created_by, created_at)
VALUES (@id, @storage_key, @original_name, @mime_type, @size_bytes, @checksum_sha256,
        @width, @height, @duration_ms, @kind, @upload_status, @source, @parent_file_id,
        @license_json, @metadata, @created_by, @created_at);

-- name: GetFile :one
SELECT * FROM asset_files WHERE id = @id;

-- name: GetFiles :many
SELECT * FROM asset_files WHERE id = ANY(@ids::uuid[]);

-- name: MarkFileReady :exec
UPDATE asset_files
SET upload_status = 'ready', mime_type = @mime_type, kind = @kind, size_bytes = @size_bytes,
    checksum_sha256 = @checksum_sha256, width = @width, height = @height, duration_ms = @duration_ms,
    -- verified bytes live under a key the browser never had a PUT URL for (ACS-101)
    storage_key = COALESCE(sqlc.narg('storage_key')::text, storage_key)
WHERE id = @id AND upload_status = 'pending';

-- name: MarkFileRejected :exec
UPDATE asset_files SET upload_status = 'rejected', metadata = metadata || @metadata
WHERE id = @id AND upload_status = 'pending';

-- name: ListStalePendingFiles :many
SELECT id, storage_key FROM asset_files
WHERE upload_status = 'pending' AND created_at < @before
ORDER BY created_at
LIMIT @lim;

-- name: DeleteFile :exec
DELETE FROM asset_files WHERE id = @id AND upload_status <> 'ready';
