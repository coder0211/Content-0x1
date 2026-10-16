# SPEC-04 — API Contract (REST `/api/v1`)

Nguồn chuẩn khi code: `api/openapi.yaml` (sinh từ tài liệu này ở ACS-003). FE sinh TS client từ OpenAPI.

## 1. Quy ước chung

| Mục         | Quy ước                                                                                           |
| ----------- | ------------------------------------------------------------------------------------------------- |
| Base path   | `/api/v1`                                                                                         |
| Format      | JSON, `snake_case`, thời gian ISO-8601 UTC                                                        |
| Auth        | Cookie `acs_session`; mọi request ghi (POST/PATCH/PUT/DELETE) gửi header `X-CSRF-Token`           |
| Idempotency | `Idempotency-Key: <uuid>` **bắt buộc** với mọi POST tạo AI job; khuyến nghị cho POST tạo resource |
| Request ID  | Server trả `X-Request-Id`; client có thể gửi sẵn                                                  |
| Phân trang  | Cursor: `?limit=20&cursor=<opaque>` → `meta.next_cursor`                                          |
| AI job      | Tạo job trả `202 Accepted`, không chờ AI xong                                                     |

### 1.1. Envelope

Thành công:

```json
{ "data": { ... }, "meta": { "request_id": "uuid", "next_cursor": null } }
```

Lỗi:

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Dữ liệu không hợp lệ",
    "details": [{ "field": "brief.target_audience", "issue": "required" }]
  },
  "meta": { "request_id": "uuid" }
}
```

### 1.2. HTTP status & error code

| HTTP | `error.code`                                                                  | Khi nào                                               |
| ---- | ----------------------------------------------------------------------------- | ----------------------------------------------------- |
| 400  | `VALIDATION_FAILED`                                                           | Body/query sai schema                                 |
| 401  | `UNAUTHENTICATED`, `SESSION_EXPIRED`                                          | Chưa đăng nhập                                        |
| 403  | `FORBIDDEN`, `CSRF_INVALID`, `SELF_APPROVAL_NOT_ALLOWED`                      | Thiếu quyền                                           |
| 404  | `NOT_FOUND`                                                                   | Không có hoặc không được thấy resource                |
| 409  | `INVALID_STATE_TRANSITION`, `IDEMPOTENCY_CONFLICT`, `VERSION_CONFLICT`        | Xung đột trạng thái                                   |
| 413  | `FILE_TOO_LARGE`                                                              |                                                       |
| 415  | `UNSUPPORTED_MEDIA_TYPE`                                                      | MIME không cho phép                                   |
| 422  | `CAPABILITY_NOT_SUPPORTED`                                                    | Tham số model/provider không hỗ trợ (kèm `details`)   |
| 423  | `ACCOUNT_LOCKED`                                                              | Khóa do đăng nhập sai                                 |
| 429  | `RATE_LIMITED`, `BUDGET_EXCEEDED`, `JOB_BUDGET_EXCEEDED`, `CONCURRENCY_LIMIT` | Hạn mức                                               |
| 502  | `PROVIDER_ERROR`                                                              | Provider lỗi upstream (chỉ với call đồng bộ hiếm hoi) |
| 503  | `SERVICE_UNAVAILABLE`                                                         | Bảo trì/quá tải                                       |

FE xử lý theo `error.code`, không theo `message`.

## 2. Endpoint list

### 2.1. Auth & Users

| Method | Path           | Quyền         | Mô tả                                                     |
| ------ | -------------- | ------------- | --------------------------------------------------------- |
| POST   | `/auth/login`  | public        | `{email,password}` → set cookie, trả `{user, csrf_token}` |
| POST   | `/auth/logout` | auth          | Thu hồi session                                           |
| GET    | `/me`          | auth          | `{id,email,name,roles[],permissions[]}`                   |
| GET    | `/users`       | `user.manage` | Danh sách                                                 |
| POST   | `/users`       | `user.manage` | `{email,name,role_codes[],temp_password}`                 |
| PATCH  | `/users/{id}`  | `user.manage` | `{name?,status?,role_codes?}` — khóa → revoke sessions    |

### 2.2. Brand & Product

| Method    | Path                                 | Quyền          | Mô tả                                                                                                             |
| --------- | ------------------------------------ | -------------- | ----------------------------------------------------------------------------------------------------------------- |
| GET       | `/brands`                            | `brand.view`   |                                                                                                                   |
| POST      | `/brands`                            | `brand.manage` | `{name, profile}` → tạo version 1                                                                                 |
| GET       | `/brands/{id}`                       | `brand.view`   | Gồm `current_version`                                                                                             |
| PATCH     | `/brands/{id}`                       | `brand.manage` | `{name?, profile?, expected_version_no}` → profile đổi thì tạo version mới; lệch version → 409 `VERSION_CONFLICT` |
| GET       | `/brands/{id}/versions`              | `brand.view`   | Lịch sử version                                                                                                   |
| GET/POST  | `/brands/{id}/assets`                | view / manage  | Logo, ảnh tham chiếu, tài liệu `{file_id, asset_type, label}`                                                     |
| DELETE    | `/brands/{id}/assets/{brandAssetId}` | `brand.manage` |                                                                                                                   |
| GET/POST  | `/brands/{id}/products`              | view / manage  |                                                                                                                   |
| GET/PATCH | `/products/{id}`                     | view / manage  |                                                                                                                   |

Brand profile schema:

```json
{
  "description": "string",
  "colors": [{ "name": "primary", "hex": "#0055FF" }],
  "typography": { "heading": "Inter", "body": "Inter" },
  "tone_of_voice": ["friendly", "trustworthy"],
  "preferred_words": ["..."],
  "forbidden_words": ["cam kết 100%", "rẻ nhất"],
  "default_ctas": ["Đăng ký tư vấn"],
  "audience_personas": [{ "name": "Sinh viên", "pain_points": ["..."], "motivations": ["..."] }]
}
```

Product facts schema:

```json
{
  "usp": ["..."],
  "price": { "amount": 2990000, "currency": "VND", "note": "học phí trọn khóa" },
  "offers": [{ "text": "Giảm 20% đến 31/12", "valid_from": "2026-11-01", "valid_to": "2026-12-31" }],
  "policies": ["Hoàn phí trong 7 ngày"],
  "proof": [{ "type": "testimonial", "text": "...", "source": "...", "verified": true }]
}
```

### 2.3. Campaign

| Method | Path                               | Quyền             | Mô tả                                        |
| ------ | ---------------------------------- | ----------------- | -------------------------------------------- |
| GET    | `/campaigns?brand_id&status&q`     | `brand.view`      |                                              |
| POST   | `/campaigns`                       | `campaign.manage` | Tạo (xem body dưới)                          |
| GET    | `/campaigns/{id}`                  | `brand.view`      | Gồm thống kê asset theo type/status, chi phí |
| PATCH  | `/campaigns/{id}`                  | `campaign.manage` | Sửa brief, `status` (validate transition)    |
| POST   | `/campaigns/{id}/plan-generations` | `generate.run`    | 202 → job `content_plan`                     |
| PUT    | `/campaigns/{id}/plan`             | `campaign.manage` | Lưu plan đã duyệt/sửa                        |

```json
POST /campaigns
{
  "brand_id": "uuid",
  "name": "Khóa giao tiếp T11",
  "goal": "lead",
  "brief": {
    "product_ids": ["uuid"],
    "target_audience": "Sinh viên năm 2-4 tại HN, HCM",
    "key_message": "Tự tin giao tiếp sau 12 tuần",
    "verified_offer": "Giảm 20% học phí đến 31/12",
    "channels": ["facebook", "tiktok"],
    "formats": [{"type": "facebook_post", "count": 5}, {"type": "poster", "count": 3}, {"type": "short_video", "count": 2}],
    "deadline": "2026-11-15",
    "notes": ""
  },
  "start_date": "2026-11-01",
  "end_date": "2026-12-31",
  "budget": {"limit_amount": 50, "currency": "USD"}
}
```

### 2.4. Content

| Method | Path                            | Quyền          | Mô tả                                                                                                      |
| ------ | ------------------------------- | -------------- | ---------------------------------------------------------------------------------------------------------- |
| POST   | `/content/generations`          | `generate.run` | 202, sinh N biến thể → N asset `content`                                                                   |
| POST   | `/content/{assetId}/rewrites`   | `generate.run` | 202, `{instruction: "shorten"\|"expand"\|"change_tone"\|"custom", tone?, custom_prompt?}` → version mới    |
| POST   | `/content/{assetId}/transforms` | `generate.run` | 202, `{target_type: "tiktok_script"\|"caption"\|"email"\|"video_script"}` → asset mới có `source_asset_id` |

```json
POST /content/generations
{
  "campaign_id": "uuid",
  "brand_id": "uuid",
  "platform": "facebook",
  "content_type": "promotional_post",
  "objective": "lead",
  "framework": "PAS",
  "length": "medium",
  "language": "vi",
  "tone": ["friendly", "trustworthy"],
  "brief": {
    "product_id": "uuid",
    "target_audience": "Sinh viên",
    "key_message": "Luyện giao tiếp tiếng Anh",
    "cta": "Đăng ký tư vấn"
  },
  "variants": 3
}
```

Response `202`:

```json
{
  "data": {
    "job_id": "uuid",
    "status": "queued",
    "estimated_cost": { "amount": 0.02, "currency": "USD", "is_estimate": true }
  },
  "meta": { "request_id": "uuid" }
}
```

> Con số chỉ minh họa; backend tính từ `model_pricing` hiện hành.

Kết quả job (`GET /jobs/{id}` khi `succeeded`): `result.asset_ids[]`. Mỗi version content:

```json
{
  "title": "...",
  "hook": "...",
  "body_html": "<p>...</p>",
  "body_text": "...",
  "cta": "...",
  "hashtags": ["#..."],
  "platform": "facebook",
  "warnings": [
    { "type": "forbidden_word", "text": "rẻ nhất" },
    { "type": "unverified_claim", "text": "giảm 50%" }
  ]
}
```

### 2.5. Image

| Method | Path                           | Quyền          | Mô tả                                                                 |
| ------ | ------------------------------ | -------------- | --------------------------------------------------------------------- |
| POST   | `/images/prompt-suggestions`   | `generate.run` | 202, `{campaign_id?, source_asset_id?, preset}` → job trả `prompts[]` |
| POST   | `/images/generations`          | `generate.run` | 202                                                                   |
| POST   | `/images/{assetId}/edits`      | `generate.run` | 202, `{file_id, prompt, mask_file_id?}` → version mới                 |
| POST   | `/images/{assetId}/variations` | `generate.run` | 202, `{file_id, count}`                                               |

```json
POST /images/generations
{
  "campaign_id": "uuid",
  "prompt": "Poster quảng bá khóa học tiếng Anh, phong cách trẻ trung...",
  "preset": "poster",
  "reference_file_ids": ["uuid"],
  "aspect_ratio": "4:5",
  "quality": "standard",
  "count": 2,
  "model_code": null
}
```

Validate: quyền truy cập `reference_file_ids`, MIME ảnh, model hỗ trợ ref image / aspect / count → nếu không: `422 CAPABILITY_NOT_SUPPORTED` với `details: [{"field":"aspect_ratio","supported":["1:1","16:9","9:16"]}]`.

### 2.6. Video

| Method | Path                                               | Quyền          | Mô tả                                                                    |
| ------ | -------------------------------------------------- | -------------- | ------------------------------------------------------------------------ |
| POST   | `/videos`                                          | `generate.run` | 202, tạo asset video + job `video_storyboard` (script + storyboard)      |
| GET    | `/videos/{assetId}`                                | `brand.view`   | Project: settings, scenes, final file, job hiện tại                      |
| PUT    | `/videos/{assetId}/scenes`                         | `generate.run` | Lưu toàn bộ storyboard đã sửa (thêm/xóa/sắp xếp)                         |
| POST   | `/videos/{assetId}/estimate`                       | `generate.run` | 200, dự toán chi tiết (không tạo job)                                    |
| POST   | `/videos/{assetId}/productions`                    | `generate.run` | 202, job `video_production`; `{budget_override?: bool}` (chỉ Lead/Admin) |
| POST   | `/videos/{assetId}/scenes/{sceneId}/regenerations` | `generate.run` | 202, `{visual_prompt?}` → sinh lại cảnh + render lại                     |
| POST   | `/videos/{assetId}/renders`                        | `generate.run` | 202, render lại (VD đổi nhạc/phụ đề) không sinh cảnh                     |

```json
POST /videos
{
  "campaign_id": "uuid",
  "source": {"type": "image", "file_ids": ["uuid"]},
  "prompt": "Video quảng cáo hiện đại, thân thiện...",
  "script_from_asset_id": null,
  "preset": "social_vertical",
  "aspect_ratio": "9:16",
  "target_duration_seconds": 20,
  "resolution": "1080p",
  "voiceover": {"enabled": true, "language": "vi", "voice_id": "configured_voice"},
  "subtitles": {"enabled": true, "language": "vi", "style": "burn_in"},
  "music": {"enabled": true, "track_brand_asset_id": "uuid"}
}
```

`POST /videos/{id}/estimate` response:

```json
{
  "data": {
    "scene_count": 5,
    "total_clip_seconds": 20,
    "breakdown": [
      {
        "item": "scene_clips",
        "provider": "vendor_x",
        "model": "model_y",
        "units": 20,
        "unit": "video_second_1080p",
        "amount": 0.0
      },
      { "item": "voiceover", "units": 420, "unit": "char", "amount": 0.0 },
      { "item": "subtitles_script", "units": 1, "unit": "request", "amount": 0.0 }
    ],
    "total": { "amount": 0.0, "currency": "USD", "is_estimate": true },
    "limits": { "per_job_limit": 0.0, "campaign_remaining": 0.0, "global_remaining": 0.0 },
    "requires_override": false
  }
}
```

Scene object:

```json
{
  "id": "uuid",
  "sequence_no": 1,
  "duration_ms": 4000,
  "visual_prompt": "...",
  "narration": "...",
  "subtitle_text": "...",
  "source_file_id": null,
  "clip_file_id": "uuid",
  "status": "ready",
  "attempt_count": 1,
  "last_error": null,
  "preview_url": "signed-url (15m)"
}
```

### 2.7. Jobs

| Method | Path                                       | Quyền                      | Mô tả                                                                         |
| ------ | ------------------------------------------ | -------------------------- | ----------------------------------------------------------------------------- |
| GET    | `/jobs?status&type&campaign_id&created_by` | owner hoặc `provider.view` |                                                                               |
| GET    | `/jobs/{id}`                               | owner hoặc `provider.view` | Gồm `steps[]`, `progress`, `result`, `error`                                  |
| GET    | `/jobs/{id}/events`                        | như trên                   | **SSE**: `event: status\|progress\|step`, `data: {...}`                       |
| POST   | `/jobs/{id}/cancel`                        | owner hoặc admin           | 202; job đã kết thúc → 409                                                    |
| POST   | `/jobs/{id}/retry`                         | owner hoặc admin           | Chỉ job `failed` & lỗi retryable (admin: `{force:true}`); tạo reservation mới |

```json
GET /jobs/{id}
{
  "data": {
    "id": "uuid", "job_type": "video_production", "status": "running", "progress": 45,
    "campaign_id": "uuid", "asset_id": "uuid",
    "provider_code": "vendor_x", "model_code": "model_y",
    "cost": {"estimate": 0.0, "actual": null, "source": null, "currency": "USD"},
    "steps": [
      {"step_key": "scene_generation:1", "status": "succeeded", "attempt_count": 1},
      {"step_key": "scene_generation:2", "status": "waiting_provider", "attempt_count": 1},
      {"step_key": "video_render", "status": "pending"}
    ],
    "result": null,
    "error": null,
    "created_at": "...", "started_at": "...", "finished_at": null
  }
}
```

### 2.8. Assets, Review, Files

| Method | Path                                                              | Quyền            | Mô tả                                                                                |
| ------ | ----------------------------------------------------------------- | ---------------- | ------------------------------------------------------------------------------------ |
| GET    | `/assets?type&campaign_id&status&created_by&tag&q&from&to&cursor` | `brand.view`     | Library search                                                                       |
| GET    | `/assets/{id}`                                                    | `brand.view`     | Chi tiết + current version + provenance                                              |
| PATCH  | `/assets/{id}`                                                    | owner/lead       | `{title?, tags?}`                                                                    |
| GET    | `/assets/{id}/versions`                                           | `brand.view`     |                                                                                      |
| POST   | `/assets/{id}/versions`                                           | owner/lead       | Revision thủ công `{content_json, change_note, base_version_no}`                     |
| POST   | `/assets/{id}/versions/{versionNo}/restore`                       | owner/lead       | Tạo version mới copy từ version cũ                                                   |
| POST   | `/assets/{id}/submit-review`                                      | owner/lead       | `draft → pending_review`; poster bắt buộc `checklist.typography_checked=true`        |
| POST   | `/assets/{id}/approve`                                            | `asset.review`   | `{comment?}`; người tạo không tự duyệt → 403 `SELF_APPROVAL_NOT_ALLOWED`             |
| POST   | `/assets/{id}/request-changes`                                    | `asset.review`   | `{comment}` bắt buộc                                                                 |
| GET    | `/reviews/queue`                                                  | `asset.review`   | Asset `pending_review`                                                               |
| POST   | `/assets/{id}/download-url`                                       | `asset.download` | `{file_id?}` → `{url, expires_at}`; Creator/Reviewer chỉ khi `approved`; ghi audit   |
| DELETE | `/assets/{id}`                                                    | owner/lead       | Soft delete; ghi audit                                                               |
| POST   | `/files/upload-url`                                               | auth             | `{filename, mime_type, size_bytes, purpose}` → `{file_id, url, headers, expires_at}` |
| POST   | `/files/{id}/complete`                                            | owner            | Server verify magic bytes/size/dimension → `ready` hoặc 415/413                      |

### 2.9. Ops: providers, usage, budgets, audit

| Method         | Path                                                                      | Quyền             | Mô tả                                       |
| -------------- | ------------------------------------------------------------------------- | ----------------- | ------------------------------------------- |
| GET            | `/settings/providers`                                                     | `provider.view`   | Không trả secret                            |
| PATCH          | `/settings/providers/{id}`                                                | `provider.manage` | status, config_json, secret_ref             |
| GET/POST/PATCH | `/settings/models` (`/{id}`)                                              | view/manage       | capabilities, concurrency_limit, is_default |
| GET/POST       | `/settings/models/{id}/pricing`                                           | view/manage       | Thêm pricing version mới (không sửa bản cũ) |
| GET/POST       | `/settings/prompt-templates`                                              | `provider.manage` | Tạo version mới (P1)                        |
| GET            | `/usage/summary?from&to&group_by=day\|campaign\|provider\|user\|job_type` | `usage.view`      | Tổng theo `cost_source`                     |
| GET            | `/settings/budgets`                                                       | `usage.view`      | global hiện tại + per-job settings          |
| PATCH          | `/settings/budgets`                                                       | `budget.manage`   | Ghi audit                                   |
| PUT            | `/campaigns/{id}/budget`                                                  | `budget.manage`   |                                             |
| GET            | `/audit-logs?actor_id&resource_type&action&from&to`                       | `audit.view`      |                                             |
| GET            | `/notifications` / POST `/notifications/{id}/read`                        | auth              |                                             |
| GET            | `/dashboard/summary`                                                      | auth              | Widget dashboard theo quyền                 |
| GET            | `/healthz`, `/readyz`, `/metrics`                                         | internal          | Không expose ra internet                    |

## 3. SSE

`GET /api/v1/jobs/{id}/events` (header `Accept: text/event-stream`)

```
event: status
data: {"job_id":"...","status":"running","progress":30}

event: step
data: {"step_key":"scene_generation:2","status":"failed","error_kind":"ContentPolicy"}

event: done
data: {"job_id":"...","status":"succeeded","result":{"asset_ids":["..."]}}
```


ET | `/settings/budgets` | `usage.view` | global hiện tại + per-job settings |
| PATCH | `/settings/budgets` | `budget.manage` | Ghi audit |
| PUT | `/campaigns/{id}/budget` | `budget.manage` | |
| GET | `/audit-logs?actor_id&resource_type&action&from&to` | `audit.view` | |
| GET | `/notifications` / POST `/notifications/{id}/read` | auth | |
| GET | `/dashboard/summary` | auth | Widget dashboard theo quyền |
| GET | `/healthz`, `/readyz`, `/metrics` | internal | Không expose ra internet |

## 3. SSE

`GET /api/v1/jobs/{id}/events` (header `Accept: text/event-stream`)

```
event: status
data: {"job_id":"...","status":"running","progress":30}

event: step
data: {"step_key":"scene_generation:2","status":"failed","error_kind":"ContentPolicy"}

event: done
data: {"job_id":"...","status":"succeeded","result":{"asset_ids":["..."]}}
```

- Server gửi `: ping` mỗi 15s. Nguồn sự kiện: Redis pub/sub `job:{id}` (worker publish sau khi commit DB).
- FE fallback polling `GET /jobs/{id}` mỗi 3s nếu SSE lỗi.
