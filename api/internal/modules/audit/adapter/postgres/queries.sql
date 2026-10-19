-- name: InsertAuditLog :exec
INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, before_json, after_json, ip, request_id)
VALUES (@actor_id, @action, @resource_type, @resource_id, @before_json, @after_json, @ip, @request_id);

-- name: ListAuditLogs :many
SELECT a.id, a.actor_id, u.email AS actor_email, a.action, a.resource_type, a.resource_id,
       a.before_json, a.after_json, COALESCE(host(a.ip), '')::text AS ip, a.request_id, a.created_at
FROM audit_logs a
LEFT JOIN users u ON u.id = a.actor_id
WHERE (sqlc.narg(actor_id)::uuid IS NULL OR a.actor_id = sqlc.narg(actor_id))
  AND (sqlc.narg(resource_type)::text IS NULL OR a.resource_type = sqlc.narg(resource_type))
  AND (sqlc.narg(resource_id)::text IS NULL OR a.resource_id = sqlc.narg(resource_id))
  AND (sqlc.narg(action)::text IS NULL OR a.action = sqlc.narg(action))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR a.created_at >= sqlc.narg(from_at))
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR a.created_at < sqlc.narg(to_at))
  AND (sqlc.narg(before_id)::bigint IS NULL OR a.id < sqlc.narg(before_id))
ORDER BY a.id DESC
LIMIT @lim;

-- name: InsertNotifications :exec
INSERT INTO notifications (user_id, type, payload)
SELECT unnest(@user_ids::uuid[]), @type, @payload;

-- name: ListNotifications :many
SELECT id, user_id, type, payload, read_at, created_at
FROM notifications
WHERE user_id = @user_id
  AND (NOT @unread_only::bool OR read_at IS NULL)
  AND (sqlc.narg(cursor_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(cursor_at), sqlc.narg(cursor_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE user_id = @user_id AND read_at IS NULL;

-- name: MarkNotificationRead :execrows
UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id = @id AND user_id = @user_id;
