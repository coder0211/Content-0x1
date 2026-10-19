-- name: InsertJob :execrows
INSERT INTO ai_jobs (id, job_type, status, campaign_id, asset_id, brand_version_id, prompt_template_id,
                     provider_code, model_code, request_json, idempotency_key, request_hash,
                     cost_estimate, currency, pricing_version, budget_override_by, progress, created_by, created_at)
VALUES (@id, @job_type, @status, @campaign_id, @asset_id, @brand_version_id, @prompt_template_id,
        @provider_code, @model_code, @request_json, @idempotency_key, @request_hash,
        @cost_estimate, @currency, @pricing_version, @budget_override_by, @progress, @created_by, @created_at)
ON CONFLICT (created_by, idempotency_key) DO NOTHING;

-- name: FindJobByIdempotency :one
SELECT * FROM ai_jobs WHERE created_by = @created_by AND idempotency_key = @idempotency_key;

-- name: GetJob :one
SELECT * FROM ai_jobs WHERE id = @id;

-- name: LockJob :one
SELECT * FROM ai_jobs WHERE id = @id FOR UPDATE;

-- name: UpdateJob :exec
UPDATE ai_jobs
SET status = @status, result_json = @result_json, cost_actual = @cost_actual, cost_source = @cost_source,
    cancel_requested = @cancel_requested, needs_reconciliation = @needs_reconciliation, progress = @progress,
    error_code = @error_code, error_message = @error_message, started_at = @started_at, finished_at = @finished_at
WHERE id = @id;

-- name: RaiseJobProgress :exec
UPDATE ai_jobs SET progress = GREATEST(progress, @progress::smallint)
WHERE id = @id AND status IN ('queued','running');

-- name: ListJobs :many
SELECT * FROM ai_jobs
WHERE (sqlc.narg(statuses)::text[] IS NULL OR status = ANY(sqlc.narg(statuses)::text[]))
  AND (sqlc.narg(types)::text[] IS NULL OR job_type = ANY(sqlc.narg(types)::text[]))
  AND (sqlc.narg(campaign_id)::uuid IS NULL OR campaign_id = sqlc.narg(campaign_id)::uuid)
  AND (sqlc.narg(created_by)::uuid IS NULL OR created_by = sqlc.narg(created_by)::uuid)
  AND (sqlc.narg(provider_code)::text IS NULL OR provider_code = sqlc.narg(provider_code)::text)
  AND (sqlc.narg(needs_reconciliation)::boolean IS NULL OR needs_reconciliation = sqlc.narg(needs_reconciliation)::boolean)
  AND (sqlc.narg(after_at)::timestamptz IS NULL
       OR (created_at, id) < (sqlc.narg(after_at)::timestamptz, sqlc.narg(after_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT @lim;

-- name: InsertStep :exec
INSERT INTO ai_job_steps (id, job_id, step_type, step_key, depends_on, scene_id, status, input_json,
                          max_attempts, next_attempt_at, created_at)
VALUES (@id, @job_id, @step_type, @step_key, @depends_on, @scene_id, @status, @input_json,
        @max_attempts, @next_attempt_at, @created_at);

-- name: ListSteps :many
SELECT * FROM ai_job_steps WHERE job_id = @job_id ORDER BY created_at, id;

-- name: GetStep :one
SELECT * FROM ai_job_steps WHERE id = @id;

-- name: UpdateStep :exec
UPDATE ai_job_steps
SET status = @status, output_json = @output_json, provider_job_id = @provider_job_id,
    attempt_count = @attempt_count, next_attempt_at = @next_attempt_at, lease_owner = @lease_owner,
    lease_expires_at = @lease_expires_at, error_kind = @error_kind, error_message = @error_message,
    started_at = @started_at, finished_at = @finished_at, generation = @generation
WHERE id = @id;

-- name: ExtendLease :execrows
UPDATE ai_job_steps SET lease_expires_at = @lease_expires_at
WHERE id = @id AND status = 'running' AND lease_owner = @lease_owner;

-- name: SaveStepOutput :execrows
UPDATE ai_job_steps SET output_json = @output_json
WHERE id = @id AND status = 'running' AND lease_owner = @lease_owner;

-- name: AppendEvent :one
INSERT INTO ai_job_events (job_id, step_id, event_type, payload, created_at)
VALUES (@job_id, @step_id, @event_type, @payload, @created_at)
RETURNING id;

-- name: ListEvents :many
SELECT * FROM (
    SELECT * FROM ai_job_events WHERE job_id = @job_id ORDER BY id DESC LIMIT @lim
) e ORDER BY id;

-- name: InsertOutbox :one
INSERT INTO outbox_events (topic, payload, available_at, created_at)
VALUES (@topic, @payload, @available_at, @created_at)
RETURNING id;

-- name: ClaimOutbox :many
SELECT * FROM outbox_events
WHERE published_at IS NULL AND available_at <= @now
ORDER BY available_at, id
LIMIT @lim
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxPublished :exec
UPDATE outbox_events SET published_at = @published_at WHERE id = ANY(@ids::bigint[]);

-- name: OldestUnpublishedOutbox :one
SELECT COALESCE(min(available_at), @now)::timestamptz AS oldest FROM outbox_events
WHERE published_at IS NULL AND available_at <= @now;

-- name: DeletePublishedOutbox :execrows
DELETE FROM outbox_events WHERE published_at IS NOT NULL AND published_at < @before;

-- name: ListExpiredLeases :many
SELECT * FROM ai_job_steps
WHERE status = 'running' AND lease_expires_at < @now
ORDER BY lease_expires_at
LIMIT @lim;

-- name: ListOverdueWaiting :many
SELECT * FROM ai_job_steps
WHERE status = 'waiting_provider' AND COALESCE(next_attempt_at, created_at) < @before
ORDER BY next_attempt_at
LIMIT @lim;

-- name: ListStaleQueued :many
SELECT s.* FROM ai_job_steps s
WHERE s.status = 'queued' AND COALESCE(s.next_attempt_at, s.created_at) < @before
  AND NOT EXISTS (
      SELECT 1 FROM outbox_events o
      WHERE o.published_at IS NULL AND (o.payload->>'step_id') = s.id::text)
ORDER BY s.next_attempt_at
LIMIT @lim;

-- name: CountNeedsReconciliation :one
SELECT count(*)::int FROM ai_jobs WHERE needs_reconciliation;

-- name: TryAdvisoryXactLock :one
SELECT pg_try_advisory_xact_lock(@key::bigint);
