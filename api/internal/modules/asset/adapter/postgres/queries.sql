-- Asset module queries (ACS-034, 080, 082, 084, 085).
-- Every asset read returns the same column list (asset + current version
-- number/author) so the generated row structs are convertible to GetAssetRow.

-- name: CreateAsset :exec
INSERT INTO content_assets (id, campaign_id, brand_id, asset_type, subtype, title, status, source_asset_id,
                            tags, usage_rights, created_by, created_at, updated_at)
VALUES (@id, @campaign_id, @brand_id, @asset_type, @subtype, @title, @status, @source_asset_id,
        @tags, @usage_rights, @created_by, @created_at, @updated_at);

-- name: GetAsset :one
SELECT a.id, a.campaign_id, a.brand_id, a.asset_type, a.subtype, a.title, a.status, a.current_version_id,
       a.source_asset_id, a.tags, a.usage_rights, a.submitted_at, a.created_by, a.created_at, a.updated_at, a.deleted_at,
       COALESCE(v.version_no, 0)::int AS current_version_no, v.created_by AS current_version_by
FROM content_assets a
LEFT JOIN asset_versions v ON v.id = a.current_version_id
WHERE a.id = @id AND a.deleted_at IS NULL;

-- name: LockAsset :one
SELECT a.id, a.campaign_id, a.brand_id, a.asset_type, a.subtype, a.title, a.status, a.current_version_id,
       a.source_asset_id, a.tags, a.usage_rights, a.submitted_at, a.created_by, a.created_at, a.updated_at, a.deleted_at,
       COALESCE(v.version_no, 0)::int AS current_version_no, v.created_by AS current_version_by
FROM content_assets a
LEFT JOIN asset_versions v ON v.id = a.current_version_id
WHERE a.id = @id AND a.deleted_at IS NULL
FOR UPDATE OF a;

-- name: SaveAsset :exec
UPDATE content_assets
SET title = @title, tags = @tags, usage_rights = @usage_rights, status = @status, submitted_at = @submitted_at,
    current_version_id = @current_version_id, deleted_at = @deleted_at
WHERE id = @id;

-- name: NextVersionNo :one
SELECT (COALESCE(MAX(version_no), 0) + 1)::int AS next FROM asset_versions WHERE asset_id = @asset_id;

-- name: InsertVersion :exec
INSERT INTO asset_versions (id, asset_id, version_no, content_json, change_note, generated_by_job_id, created_by, created_at)
VALUES (@id, @asset_id, @version_no, @content_json, @change_note, @generated_by_job_id, @created_by, @created_at);

-- name: InsertVersionFile :exec
INSERT INTO asset_version_files (version_id, file_id, role, sort_order)
VALUES (@version_id, @file_id, @role, @sort_order)
ON CONFLICT (version_id, file_id) DO UPDATE SET role = EXCLUDED.role, sort_order = EXCLUDED.sort_order;

-- name: SetSearch :exec
-- Title (A), version body (B), tags (C); accents removed so "uu dai" matches "ưu đãi".
UPDATE content_assets
SET search_tsv = setweight(to_tsvector('simple', immutable_unaccent(title)), 'A')
              || setweight(to_tsvector('simple', immutable_unaccent(@body::text)), 'B')
              || setweight(to_tsvector('simple', immutable_unaccent(array_to_string(tags, ' '))), 'C')
WHERE id = @id;

-- name: RefreshSearchTitle :exec
-- Recomputes title/tag lexemes and keeps the body (weight B) lexemes.
UPDATE content_assets
SET search_tsv = setweight(to_tsvector('simple', immutable_unaccent(title)), 'A')
              || COALESCE(ts_filter(search_tsv, '{b}'), ''::tsvector)
              || setweight(to_tsvector('simple', immutable_unaccent(array_to_string(tags, ' '))), 'C')
WHERE id = @id;

-- name: GetVersion :one
SELECT * FROM asset_versions WHERE asset_id = @asset_id AND version_no = @version_no;

-- name: GetVersionByID :one
SELECT * FROM asset_versions WHERE id = @id;

-- name: ListVersions :many
SELECT * FROM asset_versions WHERE asset_id = @asset_id ORDER BY version_no DESC;

-- name: VersionFiles :many
SELECT version_id, file_id, role, sort_order FROM asset_version_files
WHERE version_id = ANY(@version_ids::uuid[])
ORDER BY version_id, sort_order, file_id;

-- name: AddApproval :exec
INSERT INTO approvals (id, asset_id, version_id, reviewer_id, decision, comment, checklist, created_at)
VALUES (@id, @asset_id, @version_id, @reviewer_id, @decision, @comment, @checklist, @created_at);

-- name: ListApprovals :many
SELECT ap.id, ap.asset_id, ap.version_id, v.version_no, ap.reviewer_id, ap.decision, ap.comment, ap.checklist, ap.created_at
FROM approvals ap
JOIN asset_versions v ON v.id = ap.version_id
WHERE ap.asset_id = @asset_id
ORDER BY ap.created_at DESC, ap.id DESC;

-- name: SearchAssets :many
SELECT a.id, a.campaign_id, a.brand_id, a.asset_type, a.subtype, a.title, a.status, a.current_version_id,
       a.source_asset_id, a.tags, a.usage_rights, a.submitted_at, a.created_by, a.created_at, a.updated_at, a.deleted_at,
       COALESCE(v.version_no, 0)::int AS current_version_no, v.created_by AS current_version_by
FROM content_assets a
LEFT JOIN asset_versions v ON v.id = a.current_version_id
WHERE a.deleted_at IS NULL
  AND (@query::text = '' OR a.search_tsv @@ to_tsquery('simple', immutable_unaccent(@query::text)))
  AND (sqlc.narg(asset_type)::text IS NULL OR a.asset_type = sqlc.narg(asset_type)::text)
  AND (cardinality(@statuses::text[]) = 0 OR a.status = ANY(@statuses::text[]))
  AND (sqlc.narg(campaign_id)::uuid IS NULL OR a.campaign_id = sqlc.narg(campaign_id)::uuid)
  AND (sqlc.narg(brand_id)::uuid IS NULL OR a.brand_id = sqlc.narg(brand_id)::uuid)
  AND (sqlc.narg(created_by)::uuid IS NULL OR a.created_by = sqlc.narg(created_by)::uuid)
  AND (@tag::text = '' OR a.tags @> ARRAY[@tag::text])
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR a.created_at >= sqlc.narg(from_at)::timestamptz)
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR a.created_at < sqlc.narg(to_at)::timestamptz)
  AND (sqlc.narg(after_at)::timestamptz IS NULL
       OR (a.created_at, a.id) < (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::uuid))
ORDER BY a.created_at DESC, a.id DESC
LIMIT @lim;

-- name: ReviewQueue :many
SELECT a.id, a.campaign_id, a.brand_id, a.asset_type, a.subtype, a.title, a.status, a.current_version_id,
       a.source_asset_id, a.tags, a.usage_rights, a.submitted_at, a.created_by, a.created_at, a.updated_at, a.deleted_at,
       COALESCE(v.version_no, 0)::int AS current_version_no, v.created_by AS current_version_by
FROM content_assets a
LEFT JOIN asset_versions v ON v.id = a.current_version_id
WHERE a.deleted_at IS NULL AND a.status = 'pending_review'
  AND (sqlc.narg(asset_type)::text IS NULL OR a.asset_type = sqlc.narg(asset_type)::text)
  AND (sqlc.narg(campaign_id)::uuid IS NULL OR a.campaign_id = sqlc.narg(campaign_id)::uuid)
  AND (sqlc.narg(after_at)::timestamptz IS NULL
       OR (COALESCE(a.submitted_at, a.updated_at), a.id) > (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::uuid))
ORDER BY COALESCE(a.submitted_at, a.updated_at), a.id
LIMIT @lim;

-- name: CountAssets :many
-- Dashboard counters (asset API): per status, optionally for one creator.
SELECT status, COUNT(*)::int AS n FROM content_assets
WHERE deleted_at IS NULL AND (sqlc.narg(created_by)::uuid IS NULL OR created_by = sqlc.narg(created_by)::uuid)
GROUP BY status;

-- name: Provenance :many
-- Read-only provenance join (FR-AST-02) on the generating jobs.
SELECT j.id AS job_id, j.job_type, j.provider_code, j.model_code,
       pt.code AS prompt_template_code, pt.version AS prompt_template_version,
       j.brand_version_id, bv.version_no AS brand_version_no
FROM ai_jobs j
LEFT JOIN prompt_templates pt ON pt.id = j.prompt_template_id
LEFT JOIN brand_versions bv ON bv.id = j.brand_version_id
WHERE j.id = ANY(@job_ids::uuid[]);
