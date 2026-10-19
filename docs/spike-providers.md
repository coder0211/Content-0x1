# Spike: AI providers (ACS-015, OQ-01)

Status: **desk research + adapter POC (recorded fixtures).** Prices, limits and ToS below are assumptions taken from public vendor pages as of 2026-10-18. **The TL must verify each one live** (run `go test -tags live ./internal/providers/...` with real keys) and the PO + TL must sign off before S2 day 1 (ACS-015 AC).

## Summary of choices

| Type | Chosen (adapter code / model) | Alternatives evaluated | Fallback |
|---|---|---|---|
| Text | `anthropic` / `claude-sonnet-5-5` (Messages API, structured output via JSON schema) | OpenAI GPT (Responses API), Google Gemini 2.x | `fake` (dev/CI only) |
| Image | `openai` / `gpt-image-1` (generate + edit with reference images) | Google Imagen (Gemini API), Stability / Flux (via API) | `fake` |
| Video | `veo` / `veo-3.1-generate-preview`, `veo-3.1-fast-generate-preview` (Gemini API, async long-running op) | Runway Gen-4 (image→video), Kling, Luma | **`kenburns` / `kenburns-zoompan`** (internal FFmpeg, cost 0) |
| TTS | `gcp_tts` / `neural2`, `wavenet` (vi-VN, SSML marks → word timepoints) | Azure Speech (vi-VN neural), ElevenLabs multilingual, OpenAI TTS | `fake` |

All adapters use plain `net/http` (`internal/providers/vendorhttp`), read keys from `ai_providers.secret_ref` (an env var name) on every call, never log or return keys, and map vendor errors to `ports.ProviderError` per SPEC-02 §4.2.

## Text

| | Anthropic Claude Sonnet 5.5 (chosen) | OpenAI GPT | Google Gemini |
|---|---|---|---|
| Vietnamese quality | Very good, natural tone; follows brand rules well | Very good | Good |
| Structured output | `output_config.format` JSON schema (guaranteed parse) | JSON schema response format | responseSchema |
| Price assumption | $2 / MTok in, $10 / MTok out, cache read $0.20 | comparable | lower |
| Risks | Rate limits per org tier; refusal (`stop_reason=refusal`) → ContentPolicy | — | — |

Usage units: `input_token`, `output_token`, `cache_read_token`, `cache_write_token`.

## Image

| | OpenAI gpt-image-1 (chosen) | Google Imagen | Flux / Stability |
|---|---|---|---|
| Reference image / edit | Yes (`/images/edits`, multi-image) | Limited | Yes (varies) |
| Native sizes | 1024×1024, 1024×1536, 1536×1024 | several aspect ratios | flexible |
| Price assumption (per image, 1024×1536 tier) | standard $0.063, high $0.25 | ~$0.04 | ~$0.03–0.06 |
| Risks | Moderation block → ContentPolicy (no retry); 4:5/9:16/16:9 produced by centre-crop of nearest native size | | |

Unsupported params (aspect ratio, quality, variants > 4) are rejected with a validation error listing valid values (→ 422).

## Video

| | Google Veo 3.1 (chosen) | Runway Gen-4 | Ken Burns (fallback) |
|---|---|---|---|
| Image→video | Yes (reference image) | Yes | Yes (zoompan on the still) |
| Clip length | 4 / 6 / 8 s (shorter scenes trimmed in normalize) | 5 / 10 s | 2–8 s, any |
| Aspect ratios | 16:9, 9:16 (1:1 via crop in normalize) | several | 9:16, 1:1, 16:9 |
| Async API | submit → poll operation → download; **no cancel** (`ErrCancelUnsupported`) | submit / poll / cancel | synchronous render, cancel supported |
| Price assumption (per second) | standard $0.40, fast $0.15 | ~$0.05–0.12 | $0 (our FFmpeg workers) |
| Risks | Preview model names may change; RAI filter → ContentPolicy (no retry); latency 1–6 min/clip | | Lower visual quality |

### Capabilities for OQ-07

OQ-07 (≤ 60 s, ≤ 10 scenes, 2–8 s per scene, 9:16 / 1:1 / 16:9, 30 fps H.264/AAC) is satisfiable: Veo covers 2–8 s scenes (rounded up to 4/6/8 s, trimmed) in 16:9 and 9:16; 1:1 is produced by the normalize step crop; Ken Burns covers every preset at zero cost. Budget worst case per video: 60 s × $0.40 = $24 (standard) or $9 (fast).

## TTS

| | Google Cloud TTS (chosen) | Azure Speech | ElevenLabs |
|---|---|---|---|
| vi-VN voices | Neural2 A/D, WaveNet A–D | several neural vi-VN | multilingual (accent varies) |
| Word timings for subtitles | SSML `<mark>` → timepoints (v1beta1) | word boundary events | character alignment |
| Price assumption | $16 / 1M chars | ~$16 / 1M chars | much higher |
| Auth | API key **or** service-account JSON (JWT → access token) | key | key |

Usage unit: `char` (plain text, marks not counted). Output: LINEAR16 WAV.

## Error mapping (SPEC-02 §4.2)

429 / quota → `RateLimited` with `RetryAfter` (`Retry-After` header, default 30 s); 401/403 → `Auth`; vendor policy/moderation/RAI/refusal signals → `ContentPolicy` (never retried); 408/409/5xx / network / timeout → `Transient` (retryable); other 4xx → `InvalidRequest`. Shared mapping: `vendorhttp.ByStatus`, refined per adapter.

## Testing

- Contract tests: `httptest` servers replay recorded fixtures in each adapter's `testdata/`.
- Fakes (`internal/providers/fake`): configurable delay and error injection; outputs are a real PNG, a playable MP4 (per aspect ratio) and a WAV.
- Live tests: `//go:build live`, skipped when the key env var is missing.

## Open risks / TL checklist

1. Re-verify all prices above; correct them by adding a new pricing version in Settings → AI (pricing rows are immutable).
2. Confirm commercial ToS for generated media of each vendor.
3. Confirm Veo model names and whether a cancel endpoint has become available.
4. Measure real latency per clip to size the video queue concurrency (`ai_models.concurrency_limit`).
5. If Veo is not approved in time, set `kenburns-zoompan` as the default video model (ACS-015 fallback).
