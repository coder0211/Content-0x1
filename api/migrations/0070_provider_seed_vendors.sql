-- +goose Up
-- Real providers of OQ-01 with their models and pricing version '2026-10'.
-- Keys are NOT stored: secret_ref names the env var read by config.Secret.
-- Prices are public list prices assumed on 2026-10-18 (docs/spike-providers.md)
-- and must be re-verified by the TL before go-live; change them by adding a
-- new pricing version through the admin API, never by editing these rows.
-- The fake models of 0011 stay the defaults so local/CI runs need no key;
-- production sets defaults in Settings → AI (or relies on the fallback in
-- ResolveModel, which skips models whose adapter is not registered).
INSERT INTO ai_providers (code, name, kind, status, secret_ref, config_json) VALUES
 ('anthropic', 'Anthropic Claude',            '{text}',  'active', 'ANTHROPIC_API_KEY',      '{"effort":"medium"}'),
 ('openai',    'OpenAI Images',               '{image}', 'active', 'OPENAI_API_KEY',         '{"moderation":"auto"}'),
 ('veo',       'Google Veo (Gemini API)',     '{video}', 'active', 'GEMINI_API_KEY',         '{"person_generation":"allow_adult"}'),
 ('kenburns',  'Ken Burns (FFmpeg nội bộ)',   '{video}', 'active', 'none',                   '{}'),
 ('gcp_tts',   'Google Cloud Text-to-Speech', '{voice}', 'active', 'GOOGLE_TTS_CREDENTIALS', '{}');

INSERT INTO ai_models (provider_id, model_code, kind, capabilities, concurrency_limit, is_default)
SELECT p.id, m.model_code, m.kind, m.caps::jsonb, m.conc, false
FROM ai_providers p JOIN (VALUES
 ('anthropic', 'claude-sonnet-5-5', 'text',
   '{"languages":["vi","en"],"max_output_tokens":64000}', 8),
 ('openai', 'gpt-image-1', 'image',
   '{"aspect_ratios":["1:1","4:5","9:16","16:9","2:3","3:2"],"qualities":["standard","high"],"max_variants":4,"supports_ref_image":true,"supports_edit":true}', 4),
 ('veo', 'veo-3.1-generate-preview', 'video',
   '{"aspect_ratios":["16:9","9:16"],"resolutions":["720p","1080p"],"min_duration_sec":2,"max_duration_sec":8,"supports_ref_image":true,"supports_cancel":false}', 2),
 ('veo', 'veo-3.1-fast-generate-preview', 'video',
   '{"aspect_ratios":["16:9","9:16"],"resolutions":["720p","1080p"],"min_duration_sec":2,"max_duration_sec":8,"supports_ref_image":true,"supports_cancel":false}', 2),
 ('kenburns', 'kenburns-zoompan', 'video',
   '{"aspect_ratios":["9:16","1:1","16:9"],"resolutions":["720p","1080p"],"min_duration_sec":2,"max_duration_sec":8,"supports_ref_image":true,"supports_cancel":true}', 2),
 ('gcp_tts', 'neural2', 'voice',
   '{"languages":["vi-VN"],"voices":["vi-VN-Neural2-A","vi-VN-Neural2-D"]}', 4),
 ('gcp_tts', 'wavenet', 'voice',
   '{"languages":["vi-VN"],"voices":["vi-VN-Wavenet-A","vi-VN-Wavenet-B","vi-VN-Wavenet-C","vi-VN-Wavenet-D"]}', 4)
) AS m(provider_code, model_code, kind, caps, conc) ON m.provider_code = p.code;

INSERT INTO model_pricing (model_id, pricing_version, unit, unit_price, currency, effective_from)
SELECT m.id, '2026-10', x.unit, x.unit_price, 'USD', '2026-10-01T00:00:00Z'
FROM ai_models m JOIN ai_providers p ON p.id = m.provider_id JOIN (VALUES
 -- Claude Sonnet 5.5: $2 / $10 per MTok, cache read $0.20, cache write (5 min) 1.25x input.
 ('anthropic', 'claude-sonnet-5-5', 'input_token',       0.00000200),
 ('anthropic', 'claude-sonnet-5-5', 'output_token',      0.00001000),
 ('anthropic', 'claude-sonnet-5-5', 'cache_read_token',  0.00000020),
 ('anthropic', 'claude-sonnet-5-5', 'cache_write_token', 0.00000250),
 -- gpt-image-1 per image; priced at the portrait/landscape (1024x1536) tier
 -- so non-square outputs are never under-estimated: medium $0.063, high $0.25.
 ('openai', 'gpt-image-1', 'image_1024_standard', 0.06300000),
 ('openai', 'gpt-image-1', 'image_1024_high',     0.25000000),
 -- Veo 3.1 per generated second (with audio): standard $0.40, fast $0.15.
 ('veo', 'veo-3.1-generate-preview',      'video_second_720p',  0.40000000),
 ('veo', 'veo-3.1-generate-preview',      'video_second_1080p', 0.40000000),
 ('veo', 'veo-3.1-fast-generate-preview', 'video_second_720p',  0.15000000),
 ('veo', 'veo-3.1-fast-generate-preview', 'video_second_1080p', 0.15000000),
 -- Ken Burns runs on our own FFmpeg workers: no vendor cost.
 ('kenburns', 'kenburns-zoompan', 'video_second_720p',  0),
 ('kenburns', 'kenburns-zoompan', 'video_second_1080p', 0),
 -- Cloud TTS per character: Neural2 and WaveNet $16 / 1M chars.
 ('gcp_tts', 'neural2', 'char', 0.00001600),
 ('gcp_tts', 'wavenet', 'char', 0.00001600)
) AS x(provider_code, model_code, unit, unit_price) ON x.provider_code = p.code AND x.model_code = m.model_code;

-- +goose Down
DELETE FROM model_pricing WHERE model_id IN (
  SELECT m.id FROM ai_models m JOIN ai_providers p ON p.id = m.provider_id
  WHERE p.code IN ('anthropic','openai','veo','kenburns','gcp_tts'));
DELETE FROM ai_models WHERE provider_id IN (
  SELECT id FROM ai_providers WHERE code IN ('anthropic','openai','veo','kenburns','gcp_tts'));
DELETE FROM ai_providers WHERE code IN ('anthropic','openai','veo','kenburns','gcp_tts');
