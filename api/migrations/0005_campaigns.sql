-- +goose Up
-- brief: {product_ids[], target_audience, key_message, verified_offer, channels[], formats[{type,count}], deadline, notes}
CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id UUID NOT NULL REFERENCES brands(id),
    name TEXT NOT NULL,
    goal TEXT NOT NULL CHECK (goal IN ('awareness','engagement','lead','conversion')),
    brief JSONB NOT NULL DEFAULT '{}',
    content_plan JSONB,                       -- AI plan approved by the user
    status TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft','in_progress','review','completed','archived')),
    start_date DATE, end_date DATE,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_campaigns_brand_created ON campaigns (brand_id, created_at DESC);
CREATE INDEX idx_campaigns_status ON campaigns (status) WHERE deleted_at IS NULL;

CREATE TABLE campaign_members (
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    PRIMARY KEY (campaign_id, user_id)
);

CREATE TRIGGER trg_campaigns_updated BEFORE UPDATE ON campaigns FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS campaign_members;
DROP TABLE IF EXISTS campaigns;
