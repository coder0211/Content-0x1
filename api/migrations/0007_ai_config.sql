-- +goose Up
CREATE TABLE ai_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,                -- anthropic, openai, veo, kenburns, gcp_tts, fake
    name TEXT NOT NULL,
    kind TEXT[] NOT NULL,                     -- {text,image,video,voice}
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    secret_ref TEXT NOT NULL,                 -- name of the secret in env/secret manager, NOT the key
    config_json JSONB NOT NULL DEFAULT '{}',  -- base_url, region, timeout... never secrets
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ai_models (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES ai_providers(id),
    model_code TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('text','image','video','voice')),
    capabilities JSONB NOT NULL DEFAULT '{}', -- aspect_ratios, max_duration_sec, resolutions, max_variants, supports_ref_image, languages
    concurrency_limit INT NOT NULL DEFAULT 2,
    is_default BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    UNIQUE (provider_id, model_code)
);
CREATE UNIQUE INDEX uq_models_default_kind ON ai_models (kind) WHERE is_default AND status = 'active';

CREATE TABLE model_pricing (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_id UUID NOT NULL REFERENCES ai_models(id),
    pricing_version TEXT NOT NULL,
    unit TEXT NOT NULL,                       -- input_token, output_token, image_1024_standard, video_second_1080p, char
    unit_price NUMERIC(14,8) NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'USD',
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    UNIQUE (model_id, pricing_version, unit)
);

CREATE TABLE prompt_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL,                       -- content.facebook_post, image.prompt_suggest, video.storyboard
    version INT NOT NULL,
    system_prompt TEXT NOT NULL,
    user_template TEXT NOT NULL,
    output_schema JSONB,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('draft','active','retired')),
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (code, version)
);

CREATE TRIGGER trg_ai_providers_updated BEFORE UPDATE ON ai_providers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS prompt_templates;
DROP TABLE IF EXISTS model_pricing;
DROP TABLE IF EXISTS ai_models;
DROP TABLE IF EXISTS ai_providers;
