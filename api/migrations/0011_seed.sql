-- +goose Up
INSERT INTO roles (code, name) VALUES
 ('admin','Admin'), ('marketing_lead','Marketing Lead'),
 ('content_creator','Content Creator'), ('reviewer','Reviewer');

-- RBAC matrix of SPEC-01 §2 (kept in sync with kernel.RolePermissions by a test).
INSERT INTO role_permissions (role_id, permission)
SELECT r.id, p.permission FROM roles r JOIN (VALUES
 ('admin','user.manage'), ('admin','provider.manage'), ('admin','provider.view'),
 ('admin','budget.manage'), ('admin','brand.manage'), ('admin','brand.view'),
 ('admin','campaign.manage'), ('admin','generate.run'), ('admin','asset.review'),
 ('admin','asset.download'), ('admin','usage.view'), ('admin','audit.view'),
 ('marketing_lead','provider.view'), ('marketing_lead','budget.manage'),
 ('marketing_lead','brand.manage'), ('marketing_lead','brand.view'),
 ('marketing_lead','campaign.manage'), ('marketing_lead','generate.run'),
 ('marketing_lead','asset.review'), ('marketing_lead','asset.download'),
 ('marketing_lead','usage.view'),
 ('content_creator','brand.view'), ('content_creator','campaign.manage'),
 ('content_creator','generate.run'), ('content_creator','asset.download'),
 ('reviewer','brand.view'), ('reviewer','asset.review'), ('reviewer','asset.download')
) AS p(role_code, permission) ON p.role_code = r.code;

-- OQ-03: per-job limits text $0.50 / image $2 / video $8; global $1,000/month, alerts 80/100%, hard stop.
INSERT INTO budget_settings (id, per_job_limit_text, per_job_limit_image, per_job_limit_video,
                             global_monthly_limit, global_alert_thresholds, global_hard_stop,
                             max_concurrent_video_jobs_per_user, currency)
VALUES (1, 0.50, 2.00, 8.00, 1000, '{80,100}', true, 3, 'USD');

-- Fake provider for local/CI: default model for every kind so the full flow runs without keys.
INSERT INTO ai_providers (code, name, kind, status, secret_ref, config_json)
VALUES ('fake', 'Fake (local/test)', '{text,image,video,voice}', 'active', 'none', '{}');

INSERT INTO ai_models (provider_id, model_code, kind, capabilities, concurrency_limit, is_default)
SELECT p.id, m.model_code, m.kind, m.capabilities::jsonb, m.concurrency_limit, true
FROM ai_providers p, (VALUES
 ('fake-text',  'text',  '{"languages":["vi","en"],"max_output_tokens":4096}', 10),
 ('fake-image', 'image', '{"aspect_ratios":["1:1","4:5","9:16","16:9"],"qualities":["standard","high"],"max_variants":4,"supports_ref_image":true,"supports_edit":true}', 4),
 ('fake-video', 'video', '{"aspect_ratios":["9:16","1:1","16:9"],"resolutions":["720p","1080p"],"min_duration_sec":2,"max_duration_sec":8,"supports_ref_image":true,"supports_cancel":true}', 4),
 ('fake-voice', 'voice', '{"languages":["vi-VN","en-US"],"voices":["vi-VN-female-1","vi-VN-male-1"]}', 4)
) AS m(model_code, kind, capabilities, concurrency_limit)
WHERE p.code = 'fake';

INSERT INTO model_pricing (model_id, pricing_version, unit, unit_price, currency, effective_from)
SELECT m.id, 'fake-2026-10', x.unit, x.unit_price, 'USD', '2026-01-01T00:00:00Z'
FROM ai_models m JOIN (VALUES
 ('fake-text',  'input_token',         0.00000300),
 ('fake-text',  'output_token',        0.00001500),
 ('fake-image', 'image_1024_standard', 0.04000000),
 ('fake-image', 'image_1024_high',     0.17000000),
 ('fake-video', 'video_second_720p',   0.10000000),
 ('fake-video', 'video_second_1080p',  0.15000000),
 ('fake-voice', 'char',                0.00001600)
) AS x(model_code, unit, unit_price) ON x.model_code = m.model_code;

-- +goose Down
DELETE FROM model_pricing WHERE pricing_version = 'fake-2026-10';
DELETE FROM ai_models WHERE provider_id IN (SELECT id FROM ai_providers WHERE code = 'fake');
DELETE FROM ai_providers WHERE code = 'fake';
DELETE FROM budget_settings WHERE id = 1;
DELETE FROM role_permissions;
DELETE FROM roles WHERE code IN ('admin','marketing_lead','content_creator','reviewer');
