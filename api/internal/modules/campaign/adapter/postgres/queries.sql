-- name: InsertCampaign :exec
INSERT INTO campaigns (id, brand_id, name, goal, brief, status, start_date, end_date, created_by, created_at, updated_at)
VALUES (@id, @brand_id, @name, @goal, @brief, @status, @start_date, @end_date, @created_by, @created_at, @created_at);

-- name: GetCampaign :one
SELECT * FROM campaigns WHERE id = @id AND deleted_at IS NULL;

-- name: GetCampaignForUpdate :one
SELECT * FROM campaigns WHERE id = @id AND deleted_at IS NULL FOR UPDATE;

-- name: UpdateCampaign :exec
UPDATE campaigns
SET name = @name, goal = @goal, brief = @brief, status = @status, start_date = @start_date, end_date = @end_date,
    content_plan = @content_plan, content_plan_proposal = @content_plan_proposal,
    content_plan_proposed_at = @content_plan_proposed_at, content_plan_approved_by = @content_plan_approved_by,
    content_plan_approved_at = @content_plan_approved_at
WHERE id = @id AND deleted_at IS NULL;

-- name: ListCampaigns :many
SELECT c.* FROM campaigns c
WHERE c.deleted_at IS NULL
  AND (sqlc.narg('brand_id')::uuid IS NULL OR c.brand_id = sqlc.narg('brand_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR c.status = sqlc.narg('status')::text)
  AND (sqlc.narg('q')::text IS NULL OR c.name ILIKE '%' || sqlc.narg('q')::text || '%')
  AND (sqlc.narg('member_id')::uuid IS NULL OR c.created_by = sqlc.narg('member_id')::uuid
       OR EXISTS (SELECT 1 FROM campaign_members m WHERE m.campaign_id = c.id AND m.user_id = sqlc.narg('member_id')::uuid))
  AND (sqlc.narg('cursor_at')::timestamptz IS NULL
       OR (c.created_at, c.id) < (sqlc.narg('cursor_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @lim;

-- name: ListMembers :many
SELECT campaign_id, user_id FROM campaign_members WHERE campaign_id = ANY(@ids::uuid[]) ORDER BY user_id;

-- name: DeleteMembers :exec
DELETE FROM campaign_members WHERE campaign_id = @campaign_id;

-- name: InsertMember :exec
INSERT INTO campaign_members (campaign_id, user_id) VALUES (@campaign_id, @user_id) ON CONFLICT DO NOTHING;
