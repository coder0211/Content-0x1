-- +goose Up
CREATE TABLE ai_usage_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES ai_jobs(id),
    step_id UUID REFERENCES ai_job_steps(id),
    campaign_id UUID REFERENCES campaigns(id),
    user_id UUID NOT NULL REFERENCES users(id),
    provider_code TEXT NOT NULL,
    model_code TEXT NOT NULL,
    usage_json JSONB NOT NULL,                -- {input_tokens, output_tokens} | {images, size, quality} | {seconds, resolution} | {chars}
    cost_amount NUMERIC(14,6) NOT NULL,
    cost_source TEXT NOT NULL CHECK (cost_source IN ('provider_reported','calculated','estimated')),
    currency CHAR(3) NOT NULL DEFAULT 'USD',
    pricing_version TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_usage_occurred ON ai_usage_records (occurred_at);
CREATE INDEX idx_usage_campaign ON ai_usage_records (campaign_id, occurred_at);
CREATE INDEX idx_usage_job ON ai_usage_records (job_id);

CREATE TABLE budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope TEXT NOT NULL CHECK (scope IN ('global','campaign')),
    campaign_id UUID REFERENCES campaigns(id),
    period_start DATE NOT NULL,               -- global: first day of month; campaign: campaign start
    period_end DATE NOT NULL,
    limit_amount NUMERIC(14,6) NOT NULL CHECK (limit_amount >= 0),
    reserved_amount NUMERIC(14,6) NOT NULL DEFAULT 0,
    spent_amount NUMERIC(14,6) NOT NULL DEFAULT 0,
    currency CHAR(3) NOT NULL DEFAULT 'USD',
    alert_thresholds INT[] NOT NULL DEFAULT '{80,100}',
    alerts_sent INT[] NOT NULL DEFAULT '{}',
    hard_stop BOOLEAN NOT NULL DEFAULT true,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((scope = 'global' AND campaign_id IS NULL) OR (scope = 'campaign' AND campaign_id IS NOT NULL))
);
CREATE UNIQUE INDEX uq_budget_global_period ON budgets (period_start) WHERE scope = 'global';
CREATE UNIQUE INDEX uq_budget_campaign ON budgets (campaign_id) WHERE scope = 'campaign';

-- global_monthly_limit / global_alert_thresholds / global_hard_stop are the
-- template for each month's lazily created global budget row (OQ-03).
CREATE TABLE budget_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    per_job_limit_text NUMERIC(14,6) NOT NULL,
    per_job_limit_image NUMERIC(14,6) NOT NULL,
    per_job_limit_video NUMERIC(14,6) NOT NULL,
    global_monthly_limit NUMERIC(14,6) NOT NULL DEFAULT 1000,
    global_alert_thresholds INT[] NOT NULL DEFAULT '{80,100}',
    global_hard_stop BOOLEAN NOT NULL DEFAULT true,
    max_concurrent_video_jobs_per_user INT NOT NULL DEFAULT 3,
    currency CHAR(3) NOT NULL DEFAULT 'USD',
    updated_by UUID REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE budget_reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    budget_id UUID NOT NULL REFERENCES budgets(id),
    job_id UUID NOT NULL REFERENCES ai_jobs(id),
    amount NUMERIC(14,6) NOT NULL,
    status TEXT NOT NULL DEFAULT 'held' CHECK (status IN ('held','settled','released')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_at TIMESTAMPTZ,
    UNIQUE (budget_id, job_id)
);
CREATE INDEX idx_reservations_job ON budget_reservations (job_id);

-- +goose Down
DROP TABLE IF EXISTS budget_reservations;
DROP TABLE IF EXISTS budget_settings;
DROP TABLE IF EXISTS budgets;
DROP TABLE IF EXISTS ai_usage_records;
