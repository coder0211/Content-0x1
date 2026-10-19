-- Budget module queries (sqlc). Pricing reads model_pricing/ai_models/
-- ai_providers (SPEC-02 §6.2); job cost columns of ai_jobs are written by
-- Settle per the budget.API contract (see docs/DECISIONS.md, Agent 5).

-- name: EffectivePrice :one
SELECT mp.unit_price, mp.currency::text AS currency, mp.pricing_version
FROM model_pricing mp
JOIN ai_models m ON m.id = mp.model_id
JOIN ai_providers p ON p.id = m.provider_id
WHERE p.code = @provider_code AND m.model_code = @model_code AND mp.unit = @unit
  AND mp.effective_from <= @at::timestamptz
  AND (mp.effective_to IS NULL OR mp.effective_to > @at::timestamptz)
ORDER BY mp.effective_from DESC
LIMIT 1;

-- name: DefaultModel :one
SELECT p.code AS provider_code, m.model_code
FROM ai_models m JOIN ai_providers p ON p.id = m.provider_id
WHERE m.kind = @kind AND m.is_default AND m.status = 'active' AND p.status = 'active'
LIMIT 1;

-- name: GetSettings :one
SELECT per_job_limit_text, per_job_limit_image, per_job_limit_video, global_monthly_limit,
       global_alert_thresholds, global_hard_stop, max_concurrent_video_jobs_per_user,
       currency::text AS currency, updated_by, updated_at
FROM budget_settings WHERE id = 1;

-- name: SaveSettings :exec
UPDATE budget_settings SET
    per_job_limit_text = @per_job_limit_text, per_job_limit_image = @per_job_limit_image,
    per_job_limit_video = @per_job_limit_video, global_monthly_limit = @global_monthly_limit,
    global_alert_thresholds = @global_alert_thresholds, global_hard_stop = @global_hard_stop,
    max_concurrent_video_jobs_per_user = @max_concurrent_video_jobs_per_user,
    updated_by = @updated_by, updated_at = @updated_at
WHERE id = 1;

-- name: EnsureGlobalBudget :exec
INSERT INTO budgets (id, scope, period_start, period_end, limit_amount, currency, alert_thresholds, alerts_sent, hard_stop, updated_at)
VALUES (@id, 'global', @period_start, @period_end, @limit_amount, @currency, @alert_thresholds, '{}', @hard_stop, @updated_at)
ON CONFLICT (period_start) WHERE scope = 'global' DO NOTHING;

-- name: GetGlobalBudget :one
SELECT id, scope, campaign_id, period_start, period_end, limit_amount, reserved_amount, spent_amount,
       currency::text AS currency, alert_thresholds, alerts_sent, hard_stop, updated_at
FROM budgets WHERE scope = 'global' AND period_start = @period_start;

-- name: GetGlobalBudgetForUpdate :one
SELECT id, scope, campaign_id, period_start, period_end, limit_amount, reserved_amount, spent_amount,
       currency::text AS currency, alert_thresholds, alerts_sent, hard_stop, updated_at
FROM budgets WHERE scope = 'global' AND period_start = @period_start
FOR UPDATE;

-- name: GetCampaignBudget :one
SELECT id, scope, campaign_id, period_start, period_end, limit_amount, reserved_amount, spent_amount,
       currency::text AS currency, alert_thresholds, alerts_sent, hard_stop, updated_at
FROM budgets WHERE scope = 'campaign' AND campaign_id = @campaign_id;

-- name: GetCampaignBudgetForUpdate :one
SELECT id, scope, campaign_id, period_start, period_end, limit_amount, reserved_amount, spent_amount,
       currency::text AS currency, alert_thresholds, alerts_sent, hard_stop, updated_at
FROM budgets WHERE scope = 'campaign' AND campaign_id = @campaign_id
FOR UPDATE;

-- name: GetBudget :one
SELECT id, scope, campaign_id, period_start, period_end, limit_amount, reserved_amount, spent_amount,
       currency::text AS currency, alert_thresholds, alerts_sent, hard_stop, updated_at
FROM budgets WHERE id = @id;

-- name: GetBudgetForUpdate :one
SELECT id, scope, campaign_id, period_start, period_end, limit_amount, reserved_amount, spent_amount,
       currency::text AS currency, alert_thresholds, alerts_sent, hard_stop, updated_at
FROM budgets WHERE id = @id
FOR UPDATE;

-- name: ListCampaignBudgets :many
SELECT id, scope, campaign_id, period_start, period_end, limit_amount, reserved_amount, spent_amount,
       currency::text AS currency, alert_thresholds, alerts_sent, hard_stop, updated_at
FROM budgets WHERE scope = 'campaign'
ORDER BY updated_at DESC;

-- name: InsertBudget :exec
INSERT INTO budgets (id, scope, campaign_id, period_start, period_end, limit_amount, reserved_amount, spent_amount,
                     currency, alert_thresholds, alerts_sent, hard_stop, updated_at)
VALUES (@id, @scope, @campaign_id, @period_start, @period_end, @limit_amount, @reserved_amount, @spent_amount,
        @currency, @alert_thresholds, @alerts_sent, @hard_stop, @updated_at);

-- name: SaveBudget :exec
UPDATE budgets SET
    period_start = @period_start, period_end = @period_end, limit_amount = @limit_amount,
    reserved_amount = @reserved_amount, spent_amount = @spent_amount, alert_thresholds = @alert_thresholds,
    alerts_sent = @alerts_sent, hard_stop = @hard_stop, updated_at = @updated_at
WHERE id = @id;

-- name: CampaignNames :many
SELECT id, name FROM campaigns WHERE id = ANY(@ids::uuid[]);

-- name: HeldReservations :many
SELECT r.id, r.budget_id, b.scope, r.job_id, r.amount, r.status
FROM budget_reservations r JOIN budgets b ON b.id = r.budget_id
WHERE r.job_id = @job_id AND r.status = 'held'
ORDER BY (b.scope = 'global') DESC, r.created_at;

-- name: UpsertReservation :exec
INSERT INTO budget_reservations (budget_id, job_id, amount, status)
VALUES (@budget_id, @job_id, @amount, 'held')
ON CONFLICT (budget_id, job_id) DO UPDATE
SET amount = EXCLUDED.amount, status = 'held', created_at = now(), settled_at = NULL;

-- name: SetReservationStatus :exec
UPDATE budget_reservations SET status = @status, settled_at = @settled_at WHERE id = @id;

-- name: InsertUsage :exec
INSERT INTO ai_usage_records (id, job_id, step_id, campaign_id, user_id, provider_code, model_code, job_type,
                              usage_json, cost_amount, cost_source, currency, pricing_version, occurred_at)
VALUES (@id, @job_id, @step_id, @campaign_id, @user_id, @provider_code, @model_code,
        -- job_type is denormalised for group_by=job_type; fall back to the job's type when a caller omits it.
        COALESCE(sqlc.narg('job_type')::text, (SELECT j.job_type FROM ai_jobs j WHERE j.id = @job_id)),
        @usage_json, @cost_amount, @cost_source, @currency, @pricing_version, @occurred_at);

-- name: UnsettledUsage :one
SELECT COALESCE(SUM(cost_amount), 0)::numeric AS total, COUNT(*) AS count
FROM ai_usage_records WHERE job_id = @job_id AND settled_at IS NULL;

-- name: UsageCampaigns :many
SELECT DISTINCT campaign_id FROM ai_usage_records
WHERE job_id = @job_id AND campaign_id IS NOT NULL;

-- name: MarkUsageSettled :exec
UPDATE ai_usage_records SET settled_at = @settled_at WHERE job_id = @job_id AND settled_at IS NULL;

-- name: JobUsageTotals :one
SELECT COALESCE(SUM(cost_amount), 0)::numeric AS total, COUNT(*) AS count,
       COUNT(*) FILTER (WHERE cost_source = 'provider_reported') AS provider_reported,
       COUNT(*) FILTER (WHERE cost_source = 'calculated') AS calculated,
       COUNT(*) FILTER (WHERE cost_source = 'estimated') AS estimated
FROM ai_usage_records WHERE job_id = @job_id;

-- name: UpdateJobCost :exec
UPDATE ai_jobs SET cost_actual = @cost_actual, cost_source = @cost_source,
                   needs_reconciliation = needs_reconciliation OR @needs_reconciliation::boolean
WHERE id = @id;

-- name: MarkJobNeedsReconciliation :exec
UPDATE ai_jobs SET needs_reconciliation = true WHERE id = @id;

-- name: UsageSummary :many
SELECT (CASE @group_by::text
            WHEN 'day' THEN to_char(occurred_at AT TIME ZONE 'UTC', 'YYYY-MM-DD')
            WHEN 'campaign' THEN COALESCE(campaign_id::text, '')
            WHEN 'provider' THEN provider_code
            WHEN 'model' THEN model_code
            WHEN 'user' THEN user_id::text
            ELSE COALESCE(job_type, '')
        END)::text AS key,
       cost_source,
       SUM(cost_amount)::numeric AS amount,
       COUNT(*) AS count
FROM ai_usage_records
WHERE occurred_at >= @from_at::timestamptz AND occurred_at < @to_at::timestamptz
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: QueueAlertEmail :exec
INSERT INTO budget_alert_emails (id, budget_id, threshold, recipients, subject, body)
VALUES (@id, @budget_id, @threshold, @recipients, @subject, @body);

-- name: PendingAlertEmails :many
SELECT id, budget_id, threshold, recipients, subject, body, attempts
FROM budget_alert_emails
WHERE sent_at IS NULL AND attempts < @max_attempts::int
ORDER BY created_at
LIMIT @lim::int
FOR UPDATE SKIP LOCKED;

-- name: MarkAlertEmailSent :exec
UPDATE budget_alert_emails SET sent_at = @sent_at, attempts = attempts + 1 WHERE id = @id;

-- name: MarkAlertEmailFailed :exec
UPDATE budget_alert_emails SET attempts = attempts + 1, last_error = @last_error WHERE id = @id;
