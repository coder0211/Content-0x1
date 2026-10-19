-- +goose Up
CREATE TABLE ai_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type TEXT NOT NULL CHECK (job_type IN (
        'content_generation','content_rewrite','content_transform','content_plan',
        'image_generation','image_edit','image_prompt_suggest',
        'video_storyboard','video_production','video_scene_regenerate','video_render')),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','running','succeeded','failed','cancelled')),
    campaign_id UUID REFERENCES campaigns(id),
    asset_id UUID REFERENCES content_assets(id),
    brand_version_id UUID REFERENCES brand_versions(id),
    prompt_template_id UUID REFERENCES prompt_templates(id),
    provider_code TEXT,
    model_code TEXT,
    request_json JSONB NOT NULL,              -- validated parameters
    result_json JSONB,
    idempotency_key TEXT,
    request_hash TEXT,
    cost_estimate NUMERIC(14,6),
    cost_actual NUMERIC(14,6),
    cost_source TEXT CHECK (cost_source IN ('provider_reported','calculated','estimated')),
    currency CHAR(3) NOT NULL DEFAULT 'USD',
    pricing_version TEXT,
    budget_override_by UUID REFERENCES users(id),
    cancel_requested BOOLEAN NOT NULL DEFAULT false,
    needs_reconciliation BOOLEAN NOT NULL DEFAULT false,
    progress SMALLINT NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    error_code TEXT,
    error_message TEXT,
    created_by UUID NOT NULL REFERENCES users(id),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (created_by, idempotency_key)
);
CREATE INDEX idx_jobs_status_created ON ai_jobs (status, created_at);
CREATE INDEX idx_jobs_campaign ON ai_jobs (campaign_id, created_at DESC);
CREATE INDEX idx_jobs_asset ON ai_jobs (asset_id);
CREATE INDEX idx_jobs_reconcile ON ai_jobs (id) WHERE needs_reconciliation;

ALTER TABLE asset_versions ADD CONSTRAINT fk_versions_job
    FOREIGN KEY (generated_by_job_id) REFERENCES ai_jobs(id);

CREATE TABLE ai_job_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES ai_jobs(id) ON DELETE CASCADE,
    step_type TEXT NOT NULL,                  -- script_generation, scene_generation, video_render...
    step_key TEXT NOT NULL,                   -- unique within the job, e.g. scene_generation:3
    depends_on TEXT[] NOT NULL DEFAULT '{}',  -- step_keys
    scene_id UUID REFERENCES video_scenes(id),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN
        ('pending','queued','running','waiting_provider','succeeded','failed','cancelled','skipped')),
    input_json JSONB NOT NULL DEFAULT '{}',
    output_json JSONB,
    provider_job_id TEXT,
    attempt_count INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    next_attempt_at TIMESTAMPTZ,
    lease_owner TEXT,
    lease_expires_at TIMESTAMPTZ,
    error_kind TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (job_id, step_key)
);
CREATE INDEX idx_steps_lease ON ai_job_steps (lease_expires_at) WHERE status IN ('running','waiting_provider');

CREATE TABLE ai_job_events (
    id BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES ai_jobs(id) ON DELETE CASCADE,
    step_id UUID REFERENCES ai_job_steps(id),
    event_type TEXT NOT NULL,                 -- status_changed, progress, retry_scheduled, provider_error...
    payload JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_job_events_job ON ai_job_events (job_id, id);

CREATE TABLE outbox_events (
    id BIGSERIAL PRIMARY KEY,
    topic TEXT NOT NULL,                      -- queue name
    payload JSONB NOT NULL,                   -- {job_id, step_id}
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_outbox_unpublished ON outbox_events (available_at) WHERE published_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS ai_job_events;
DROP TABLE IF EXISTS ai_job_steps;
ALTER TABLE asset_versions DROP CONSTRAINT IF EXISTS fk_versions_job;
DROP TABLE IF EXISTS ai_jobs;
