-- Job engine indexes (Agent 3, ACS-011/012/018). Base tables live in 0008.
-- +goose Up
-- GET /jobs (newest first, optionally per user).
CREATE INDEX idx_jobs_created ON ai_jobs (created_at DESC, id DESC);
CREATE INDEX idx_jobs_created_by ON ai_jobs (created_by, created_at DESC, id DESC);
-- Steps of a job in creation order.
CREATE INDEX idx_steps_job_created ON ai_job_steps (job_id, created_at);
-- Reconciler: queued / waiting_provider steps by due time.
CREATE INDEX idx_steps_due ON ai_job_steps (status, next_attempt_at)
    WHERE status IN ('queued','waiting_provider');
-- Reconciler: unpublished outbox rows of a step; retention cleanup.
CREATE INDEX idx_outbox_step_unpublished ON outbox_events (((payload->>'step_id'))) WHERE published_at IS NULL;
CREATE INDEX idx_outbox_published ON outbox_events (published_at) WHERE published_at IS NOT NULL;
-- 0 = engine default for the handler's queue (3; 5 on the video queue).
ALTER TABLE ai_job_steps ALTER COLUMN max_attempts SET DEFAULT 0;
ALTER TABLE ai_job_steps ADD CONSTRAINT chk_steps_max_attempts CHECK (max_attempts >= 0);
-- Incremented each time POST /jobs/{id}/retry re-opens the step, so the
-- asynq unique task id (step_id:attempt[:g<generation>]) never collides
-- with a task of an earlier run that asynq still remembers.
ALTER TABLE ai_job_steps ADD COLUMN generation INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE ai_job_steps DROP COLUMN IF EXISTS generation;
ALTER TABLE ai_job_steps DROP CONSTRAINT IF EXISTS chk_steps_max_attempts;
ALTER TABLE ai_job_steps ALTER COLUMN max_attempts SET DEFAULT 3;
DROP INDEX IF EXISTS idx_outbox_published;
DROP INDEX IF EXISTS idx_outbox_step_unpublished;
DROP INDEX IF EXISTS idx_steps_due;
DROP INDEX IF EXISTS idx_steps_job_created;
DROP INDEX IF EXISTS idx_jobs_created_by;
DROP INDEX IF EXISTS idx_jobs_created;
