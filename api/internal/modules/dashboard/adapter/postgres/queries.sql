-- Dashboard read model (ACS-095). Read-only queries; this module never
-- writes to the tables below (owned by campaign, job and budget).

-- name: ActiveCampaignCount :one
SELECT COUNT(*)::int FROM campaigns WHERE deleted_at IS NULL AND status IN ('in_progress', 'review');

-- name: RecentActiveCampaigns :many
SELECT id, name, status, updated_at FROM campaigns
WHERE deleted_at IS NULL AND status IN ('in_progress', 'review')
ORDER BY updated_at DESC, id DESC
LIMIT @lim;

-- name: JobCounts :one
SELECT COUNT(*) FILTER (WHERE status = 'queued')::int AS queued,
       COUNT(*) FILTER (WHERE status = 'running')::int AS running,
       COUNT(*) FILTER (WHERE status = 'failed' AND finished_at >= @failed_from::timestamptz)::int AS failed_24h
FROM ai_jobs
WHERE (sqlc.narg(created_by)::uuid IS NULL OR created_by = sqlc.narg(created_by)::uuid)
  AND (status IN ('queued', 'running') OR (status = 'failed' AND finished_at >= @failed_from::timestamptz));

-- name: RecentJobs :many
SELECT id, job_type, status, progress, error_code, created_at FROM ai_jobs
WHERE (sqlc.narg(created_by)::uuid IS NULL OR created_by = sqlc.narg(created_by)::uuid)
  AND (status IN ('queued', 'running') OR (status = 'failed' AND finished_at >= @failed_from::timestamptz))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: SpentSince :one
SELECT COALESCE(SUM(cost_amount), 0)::numeric AS spent FROM ai_usage_records WHERE occurred_at >= @from_at;

-- name: GlobalMonthlyLimit :one
SELECT COALESCE(
    (SELECT limit_amount FROM budgets WHERE scope = 'global' AND period_start = @month_start::date),
    (SELECT global_monthly_limit FROM budget_settings WHERE id = 1),
    -1 -- no limit configured
)::numeric AS monthly_limit;
