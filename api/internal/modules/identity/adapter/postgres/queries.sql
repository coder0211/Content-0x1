-- name: CreateUser :exec
INSERT INTO users (id, email, name, password_hash, status, created_at, updated_at)
VALUES (@id, @email, @name, @password_hash, @status, @created_at, @created_at);

-- name: GetUserByID :one
SELECT u.*, COALESCE(array_agg(r.code ORDER BY r.code) FILTER (WHERE r.code IS NOT NULL), '{}')::text[] AS roles
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
WHERE u.id = @id
GROUP BY u.id;

-- name: GetUserByEmail :one
SELECT u.*, COALESCE(array_agg(r.code ORDER BY r.code) FILTER (WHERE r.code IS NOT NULL), '{}')::text[] AS roles
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
WHERE u.email = @email
GROUP BY u.id;

-- name: LockUser :exec
SELECT 1 FROM users WHERE id = @id FOR UPDATE;

-- name: UpdateUserLoginState :exec
UPDATE users
SET failed_login_count = @failed_login_count, last_failed_login_at = @last_failed_login_at,
    locked_until = @locked_until, last_login_at = @last_login_at
WHERE id = @id;

-- name: UpdateUserProfile :exec
UPDATE users SET name = @name, status = @status WHERE id = @id;

-- name: DeleteUserRoles :exec
DELETE FROM user_roles WHERE user_id = @user_id;

-- name: InsertUserRoles :execrows
INSERT INTO user_roles (user_id, role_id)
SELECT @user_id, r.id FROM roles r WHERE r.code = ANY(@codes::text[]);

-- name: ListUsers :many
SELECT u.*, COALESCE(array_agg(r.code ORDER BY r.code) FILTER (WHERE r.code IS NOT NULL), '{}')::text[] AS roles
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
WHERE (sqlc.narg(query)::text IS NULL OR u.email ILIKE '%' || sqlc.narg(query) || '%' OR u.name ILIKE '%' || sqlc.narg(query) || '%')
  AND (sqlc.narg(status)::text IS NULL OR u.status = sqlc.narg(status))
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (u.created_at, u.id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
GROUP BY u.id
ORDER BY u.created_at DESC, u.id DESC
LIMIT @lim;

-- name: ListUsersByIDs :many
SELECT u.*, COALESCE(array_agg(r.code ORDER BY r.code) FILTER (WHERE r.code IS NOT NULL), '{}')::text[] AS roles
FROM users u
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
WHERE u.id = ANY(@ids::uuid[])
GROUP BY u.id;

-- name: GetUserPermissions :many
SELECT DISTINCT rp.permission
FROM user_roles ur JOIN role_permissions rp ON rp.role_id = ur.role_id
WHERE ur.user_id = @user_id
ORDER BY rp.permission;

-- name: ListUsersWithPermission :many
SELECT u.*, COALESCE(array_agg(r.code ORDER BY r.code) FILTER (WHERE r.code IS NOT NULL), '{}')::text[] AS roles
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN roles r ON r.id = ur.role_id
WHERE u.status = 'active'
  AND EXISTS (SELECT 1 FROM user_roles ur2 JOIN role_permissions rp ON rp.role_id = ur2.role_id
              WHERE ur2.user_id = u.id AND rp.permission = @permission)
GROUP BY u.id
ORDER BY u.name;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, token_hash, csrf_token, ip, user_agent, last_seen_at, expires_at, created_at)
VALUES (@id, @user_id, @token_hash, @csrf_token, @ip, @user_agent, @last_seen_at, @expires_at, @created_at);

-- name: GetSessionByTokenHash :one
SELECT id, user_id, token_hash, csrf_token, COALESCE(host(ip), '')::text AS ip, user_agent, last_seen_at, expires_at, revoked_at, created_at
FROM sessions WHERE token_hash = @token_hash;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = @at WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = @at WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeUserSessions :exec
UPDATE sessions SET revoked_at = @at WHERE user_id = @user_id AND revoked_at IS NULL;
