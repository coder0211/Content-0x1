-- +goose Up
-- ACS-037: the AI plan arrives as a proposal (job content_plan, written via
-- campaign.API.SaveContentPlanProposal); the user edits/approves it, which
-- stores campaigns.content_plan. A proposal never overwrites an approved plan.
ALTER TABLE campaigns
    ADD COLUMN content_plan_proposal JSONB,
    ADD COLUMN content_plan_proposed_at TIMESTAMPTZ,
    ADD COLUMN content_plan_approved_by UUID REFERENCES users(id),
    ADD COLUMN content_plan_approved_at TIMESTAMPTZ;
CREATE INDEX idx_campaigns_created ON campaigns (created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_campaign_members_user ON campaign_members (user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_campaign_members_user;
DROP INDEX IF EXISTS idx_campaigns_created;
ALTER TABLE campaigns
    DROP COLUMN IF EXISTS content_plan_approved_at,
    DROP COLUMN IF EXISTS content_plan_approved_by,
    DROP COLUMN IF EXISTS content_plan_proposed_at,
    DROP COLUMN IF EXISTS content_plan_proposal;
