-- +goose Up
-- Budget module (Agent 5): settlement bookkeeping, usage reporting columns and
-- the alert e-mail outbox (ACS-041/042/044/092).

-- settled_at marks usage already moved into budgets.spent_amount, so Settle is
-- idempotent and a retried job only adds the usage of its new attempt.
ALTER TABLE ai_usage_records ADD COLUMN settled_at TIMESTAMPTZ;
-- job_type is denormalised for GET /usage/summary?group_by=job_type.
ALTER TABLE ai_usage_records ADD COLUMN job_type TEXT;
CREATE INDEX idx_usage_job_unsettled ON ai_usage_records (job_id) WHERE settled_at IS NULL;
CREATE INDEX idx_usage_user ON ai_usage_records (user_id, occurred_at);

CREATE INDEX idx_reservations_held ON budget_reservations (job_id) WHERE status = 'held';

-- Budget alert e-mails are queued in the same transaction that marks the
-- threshold as sent (budgets.alerts_sent) and delivered by a worker runner,
-- so a rolled-back reservation never sends mail.
CREATE TABLE budget_alert_emails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    budget_id UUID NOT NULL REFERENCES budgets(id),
    threshold INT NOT NULL,
    recipients TEXT[] NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_budget_alert_emails_pending ON budget_alert_emails (created_at) WHERE sent_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS budget_alert_emails;
DROP INDEX IF EXISTS idx_reservations_held;
DROP INDEX IF EXISTS idx_usage_user;
DROP INDEX IF EXISTS idx_usage_job_unsettled;
ALTER TABLE ai_usage_records DROP COLUMN IF EXISTS job_type;
ALTER TABLE ai_usage_records DROP COLUMN IF EXISTS settled_at;
