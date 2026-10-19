-- name: ListProviders :many
SELECT id, code, name, kind, status, secret_ref, config_json, created_at, updated_at
FROM ai_providers ORDER BY code;

-- name: GetProvider :one
SELECT id, code, name, kind, status, secret_ref, config_json, created_at, updated_at
FROM ai_providers WHERE id = $1;

-- name: GetProviderForUpdate :one
SELECT id, code, name, kind, status, secret_ref, config_json, created_at, updated_at
FROM ai_providers WHERE id = $1 FOR UPDATE;

-- name: UpdateProvider :execrows
UPDATE ai_providers SET status = $2, secret_ref = $3, config_json = $4 WHERE id = $1;

-- name: ListModels :many
SELECT m.id, m.provider_id, p.code AS provider_code, p.status AS provider_status, m.model_code, m.kind,
       m.capabilities, m.concurrency_limit, m.is_default, m.status
FROM ai_models m JOIN ai_providers p ON p.id = m.provider_id
ORDER BY m.kind, p.code, m.model_code;

-- name: GetModel :one
SELECT m.id, m.provider_id, p.code AS provider_code, p.status AS provider_status, m.model_code, m.kind,
       m.capabilities, m.concurrency_limit, m.is_default, m.status
FROM ai_models m JOIN ai_providers p ON p.id = m.provider_id
WHERE m.id = $1;

-- name: GetModelForUpdate :one
SELECT m.id, m.provider_id, p.code AS provider_code, p.status AS provider_status, m.model_code, m.kind,
       m.capabilities, m.concurrency_limit, m.is_default, m.status
FROM ai_models m JOIN ai_providers p ON p.id = m.provider_id
WHERE m.id = $1 FOR UPDATE OF m;

-- name: CreateModel :exec
INSERT INTO ai_models (id, provider_id, model_code, kind, capabilities, concurrency_limit, is_default, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateModel :execrows
UPDATE ai_models SET capabilities = $2, concurrency_limit = $3, is_default = $4, status = $5 WHERE id = $1;

-- name: ClearDefault :exec
UPDATE ai_models SET is_default = false WHERE kind = $1 AND id <> $2 AND is_default;

-- name: ListPrices :many
SELECT id, model_id, pricing_version, unit, unit_price, currency, effective_from, effective_to
FROM model_pricing WHERE model_id = $1 ORDER BY effective_from DESC, unit;

-- name: PricingVersionExists :one
SELECT EXISTS (SELECT 1 FROM model_pricing WHERE model_id = $1 AND pricing_version = $2);

-- name: ClosePrices :exec
UPDATE model_pricing SET effective_to = sqlc.arg(at)
WHERE model_id = sqlc.arg(model_id) AND effective_to IS NULL AND effective_from < sqlc.arg(at);

-- name: InsertPrice :exec
INSERT INTO model_pricing (id, model_id, pricing_version, unit, unit_price, currency, effective_from)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListTemplates :many
SELECT id, code, version, system_prompt, user_template, output_schema, status, created_by, created_at
FROM prompt_templates
WHERE (sqlc.arg(code)::text = '' OR code = sqlc.arg(code)::text)
ORDER BY code, version DESC;

-- name: GetTemplate :one
SELECT id, code, version, system_prompt, user_template, output_schema, status, created_by, created_at
FROM prompt_templates WHERE id = $1;

-- name: GetTemplateForUpdate :one
SELECT id, code, version, system_prompt, user_template, output_schema, status, created_by, created_at
FROM prompt_templates WHERE id = $1 FOR UPDATE;

-- name: ActiveTemplate :one
SELECT id, code, version, system_prompt, user_template, output_schema, status, created_by, created_at
FROM prompt_templates WHERE code = $1 AND status = 'active'
ORDER BY version DESC LIMIT 1;

-- name: LockTemplateCode :exec
SELECT pg_advisory_xact_lock(hashtext('prompt_template:' || sqlc.arg(code)::text));

-- name: MaxTemplateVersion :one
SELECT COALESCE(MAX(version), 0)::int AS max_version FROM prompt_templates WHERE code = $1;

-- name: CreateTemplate :exec
INSERT INTO prompt_templates (id, code, version, system_prompt, user_template, output_schema, status, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: SetTemplateStatus :execrows
UPDATE prompt_templates SET status = $2 WHERE id = $1;
