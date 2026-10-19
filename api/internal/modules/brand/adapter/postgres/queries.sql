-- name: InsertBrand :exec
INSERT INTO brands (id, name, created_by, created_at, updated_at)
VALUES (@id, @name, @created_by, @created_at, @created_at);

-- name: InsertBrandVersion :exec
INSERT INTO brand_versions (id, brand_id, version_no, profile, created_by, created_at)
VALUES (@id, @brand_id, @version_no, @profile, @created_by, @created_at);

-- name: SetBrandCurrentVersion :exec
UPDATE brands SET current_version_id = @version_id WHERE id = @id;

-- name: RenameBrand :exec
UPDATE brands SET name = @name WHERE id = @id AND deleted_at IS NULL;

-- name: SoftDeleteBrand :exec
UPDATE brands SET deleted_at = now() WHERE id = @id AND deleted_at IS NULL;

-- name: GetBrand :one
SELECT b.id, b.name, b.created_by, b.created_at, b.updated_at,
       v.id AS version_id, v.version_no, v.profile, v.created_by AS version_created_by, v.created_at AS version_created_at
FROM brands b JOIN brand_versions v ON v.id = b.current_version_id
WHERE b.id = @id AND b.deleted_at IS NULL;

-- name: GetBrandForUpdate :one
SELECT b.id, b.name, b.created_by, b.created_at, b.updated_at,
       v.id AS version_id, v.version_no, v.profile, v.created_by AS version_created_by, v.created_at AS version_created_at
FROM brands b JOIN brand_versions v ON v.id = b.current_version_id
WHERE b.id = @id AND b.deleted_at IS NULL
FOR UPDATE OF b;

-- name: ListBrands :many
SELECT b.id, b.name, b.created_by, b.created_at, b.updated_at,
       v.id AS version_id, v.version_no, v.profile, v.created_by AS version_created_by, v.created_at AS version_created_at
FROM brands b JOIN brand_versions v ON v.id = b.current_version_id
WHERE b.deleted_at IS NULL
  AND (sqlc.narg('q')::text IS NULL OR b.name ILIKE '%' || sqlc.narg('q')::text || '%')
  AND (sqlc.narg('cursor_at')::timestamptz IS NULL
       OR (b.created_at, b.id) < (sqlc.narg('cursor_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY b.created_at DESC, b.id DESC
LIMIT @lim;

-- name: ListBrandVersions :many
SELECT * FROM brand_versions
WHERE brand_id = @brand_id AND (sqlc.narg('before_no')::int IS NULL OR version_no < sqlc.narg('before_no')::int)
ORDER BY version_no DESC
LIMIT @lim;

-- name: GetBrandVersion :one
SELECT * FROM brand_versions WHERE id = @id;

-- name: InsertProduct :exec
INSERT INTO products (id, brand_id, name, description, facts, status, created_by, created_at, updated_at)
VALUES (@id, @brand_id, @name, @description, @facts, @status, @created_by, @created_at, @created_at);

-- name: GetProduct :one
SELECT p.* FROM products p JOIN brands b ON b.id = p.brand_id
WHERE p.id = @id AND b.deleted_at IS NULL;

-- name: UpdateProduct :exec
UPDATE products SET name = @name, description = @description, facts = @facts, status = @status
WHERE id = @id;

-- name: ListProducts :many
SELECT * FROM products
WHERE brand_id = @brand_id
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('cursor_at')::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg('cursor_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: ListActiveProducts :many
SELECT * FROM products WHERE brand_id = @brand_id AND status = 'active' ORDER BY created_at, id;

-- name: InsertBrandAsset :exec
INSERT INTO brand_assets (id, brand_id, file_id, asset_type, label, license_json, created_by, created_at)
VALUES (@id, @brand_id, @file_id, @asset_type, @label, @license_json, @created_by, @created_at);

-- name: GetBrandAsset :one
SELECT a.* FROM brand_assets a JOIN brands b ON b.id = a.brand_id
WHERE a.id = @id AND b.deleted_at IS NULL;

-- name: ListBrandAssets :many
SELECT * FROM brand_assets
WHERE brand_id = @brand_id
  AND (cardinality(@types::text[]) = 0 OR asset_type = ANY(@types::text[]))
ORDER BY created_at DESC, id DESC;

-- name: DeleteBrandAsset :execrows
DELETE FROM brand_assets WHERE id = @id AND brand_id = @brand_id;
