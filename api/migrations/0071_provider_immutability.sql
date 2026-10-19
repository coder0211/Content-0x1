-- +goose Up
-- ACS-090: pricing versions are append-only. A row's price, unit, version and
-- effective_from never change; effective_to may only be set once (NULL → a
-- value) when a newer version supersedes it. Rows are never deleted.
-- +goose StatementBegin
CREATE FUNCTION model_pricing_guard() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'model_pricing is append-only' USING ERRCODE = 'P0001';
    END IF;
    IF NEW.model_id IS DISTINCT FROM OLD.model_id
       OR NEW.pricing_version IS DISTINCT FROM OLD.pricing_version
       OR NEW.unit IS DISTINCT FROM OLD.unit
       OR NEW.unit_price IS DISTINCT FROM OLD.unit_price
       OR NEW.currency IS DISTINCT FROM OLD.currency
       OR NEW.effective_from IS DISTINCT FROM OLD.effective_from
       OR (OLD.effective_to IS NOT NULL AND NEW.effective_to IS DISTINCT FROM OLD.effective_to) THEN
        RAISE EXCEPTION 'model_pricing rows are immutable (only effective_to may be closed once)' USING ERRCODE = 'P0001';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_model_pricing_guard BEFORE UPDATE OR DELETE ON model_pricing
    FOR EACH ROW EXECUTE FUNCTION model_pricing_guard();

-- FR-CNT-08: a prompt template version is immutable once created (jobs keep
-- pointing at the exact text they used); only its status changes.
-- +goose StatementBegin
CREATE FUNCTION prompt_templates_guard() RETURNS trigger AS $$
BEGIN
    IF NEW.code IS DISTINCT FROM OLD.code OR NEW.version IS DISTINCT FROM OLD.version
       OR NEW.system_prompt IS DISTINCT FROM OLD.system_prompt
       OR NEW.user_template IS DISTINCT FROM OLD.user_template
       OR NEW.output_schema IS DISTINCT FROM OLD.output_schema
       OR NEW.created_by IS DISTINCT FROM OLD.created_by
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'prompt template versions are immutable' USING ERRCODE = 'P0001';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_prompt_templates_guard BEFORE UPDATE ON prompt_templates
    FOR EACH ROW EXECUTE FUNCTION prompt_templates_guard();

-- At most one active version per template code.
CREATE UNIQUE INDEX uq_prompt_templates_active ON prompt_templates (code) WHERE status = 'active';
CREATE INDEX idx_model_pricing_model ON model_pricing (model_id, effective_from DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_model_pricing_model;
DROP INDEX IF EXISTS uq_prompt_templates_active;
DROP TRIGGER IF EXISTS trg_prompt_templates_guard ON prompt_templates;
DROP FUNCTION IF EXISTS prompt_templates_guard();
DROP TRIGGER IF EXISTS trg_model_pricing_guard ON model_pricing;
DROP FUNCTION IF EXISTS model_pricing_guard();
