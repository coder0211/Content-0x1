# BACKLOG — AI Content Studio MVP

- Ước lượng tính bằng **dev-day lý tưởng (d)**, giả định dev dùng AI coding assistant.
- Owner: **BE** (Go), **FE** (React), **FS** (fullstack), **TL** (tech lead), **PO**.
- Ưu tiên: **P0** bắt buộc MVP, **P1** cắt được nếu trễ.
- Sprint: S1–S5 (xem [plan/SPRINT-PLAN.md](../plan/SPRINT-PLAN.md)).
- Mọi task phải đạt **DoD task** ở [SPEC-01 §7](../spec/SPEC-01-product.md#7-definition-of-done--nghiệm-thu-mvp).
- Code đặt theo [SPEC-06](../spec/SPEC-06-code-structure.md) (Clean Architecture); task UI tuân theo [SPEC-05](../spec/SPEC-05-design-guidelines.md) và PR đính kèm ảnh chụp các trạng thái ở SPEC-05 §5.

## Tổng quan epic

| Epic | Tên | Task | Tổng (d) |
|---|---|---|---|
| E01 | Foundation & Auth | ACS-001 → 010 | 12.5 |
| E02 | Job orchestration & Provider core | ACS-011 → 019 | 10.5 |
| E03 | Brand, Product, Campaign, Storage | ACS-020 → 026 | 8.5 |
| E04 | Content Studio | ACS-030 → 038 | 10.0 |
| E05 | Cost & Budget | ACS-040 → 044 | 4.0 |
| E06 | Image Studio | ACS-050 → 054 | 5.5 |
| E07 | Video Studio | ACS-060 → 072 | 15.5 |
| E08 | Review & Asset Library | ACS-080 → 085 | 6.0 |
| E09 | AI Operations & Dashboard | ACS-090 → 095 | 6.0 |
| E10 | Hardening, Deploy & Pilot | ACS-100 → 110 | 10.0 |
| | **Tổng** (80 task) | | **88.5** |

---

## E01 — Foundation & Auth

### ACS-001 · Monorepo + Docker Compose local
S1 · 1d · BE · P0 · Deps: —
- Tạo cấu trúc `api/`, `web/`, `deploy/`, `docs/` theo SPEC-02 §3.
- `docker-compose.yml`: postgres 16, redis 7, minio (+ bucket init), mailpit.
- `Makefile`: `make up`, `make migrate`, `make test`, `make gen`.

**AC:** `make up` trên máy sạch chạy được toàn bộ stack; README hướng dẫn setup ≤ 10 phút.

### ACS-002 · CI pipeline
S1 · 0.5d · BE · P0 · Deps: ACS-001
- GitHub Actions/GitLab CI: Go lint (golangci-lint), `go test` (testcontainers), web lint + typecheck + vitest, build Docker images.
- Kiểm tra kiến trúc: `depguard` + `go-arch-lint` (BE), `eslint-plugin-boundaries` (FE) theo SPEC-06 §2.7, §3.4.

**AC:** PR fail khi lint/test fail hoặc vi phạm quy tắc phụ thuộc layer; thời gian CI < 10 phút.

### ACS-003 · Go API skeleton (Clean Architecture)
S1 · 2d · BE · P0 · Deps: ACS-001
- Dựng khung theo SPEC-06 §2: `shared/kernel` (ID, Money, DomainError, Actor, Page), `ports` (TxManager, Clock, Storage, AI, Audit), `platform/*`, module mẫu `modules/_template` (domain/usecase/adapter/module.go), composition root `cmd/server`, `cmd/worker`.
- `cmd/server`, `cmd/worker`; config từ env; slog JSON; chi router; middleware: request_id, recover, access log (redact), CORS nội bộ.
- Error envelope + mapping error code (SPEC-04 §1).
- `openapi.yaml` khung + `healthz`, `readyz`, `metrics`.

**AC:** Lỗi panic trả 500 envelope chuẩn, có `request_id`; log không chứa header `Cookie/Authorization`; module mẫu có test ở cả 3 layer và pass arch lint.

### ACS-004 · React web skeleton + design system
S1 · 2.5d · FE · P0 · Deps: ACS-001
- Vite + TS strict + Tailwind + shadcn/ui; React Router; TanStack Query; API client sinh từ OpenAPI (`openapi-typescript` + fetch wrapper xử lý envelope/CSRF).
- Cấu trúc theo SPEC-06 §3: `app/`, `shared/{ui,api,lib,kernel}`, module mẫu với 4 layer domain/application/infrastructure/presentation, DI container cho repository.
- Design tokens theo SPEC-05 §2 (`theme.css`, `tailwind.config.ts`, font Be Vietnam Pro); chỉnh shadcn (Button, Input, Table, Dialog, Toast); pattern `StatusBadge`, `CostChip`, `JobProgress`, `AIProvenance`, `EmptyState`.
- Lint chặn class trong danh sách cấm SPEC-05 §1.1 (gradient, backdrop-blur, rounded-2xl+, shadow-lg+).
- App shell theo SPEC-05 §4.1: sidebar điều hướng (Dashboard, Campaigns, Content, Image, Video, Library, Review, Brand, Settings).

**AC:** Build prod thành công; route placeholder cho mọi mục; lỗi API hiển thị toast theo `error.code`; trang `/_design` hiển thị đủ token + component để PO duyệt style.

### ACS-005 · Migration + sqlc
S1 · 1d · BE · P0 · Deps: ACS-003
- goose; migrations 0001–0003 (SPEC-03); sqlc config; seed roles/permissions.

**AC:** `migrate up` + `down` chạy sạch; CI chạy migration trên DB trống.

### ACS-006 · Authentication (session)
S1 · 1.5d · BE · P0 · Deps: ACS-005 · FR-AUTH-01
- Login/logout, argon2id, bảng `sessions` (token hash), cookie HttpOnly/Secure/SameSite, CSRF double-submit, idle 12h/absolute 7d, lockout 5 lần/15 phút.
- CLI `acs admin create --email` để tạo admin đầu tiên.

**AC:** Test: sai pass 5 lần → 423; request POST thiếu CSRF → 403; session hết hạn → 401.

### ACS-007 · RBAC + /me
S1 · 1d · BE · P0 · Deps: ACS-006 · FR-AUTH-03
- `RequirePermission(code)` middleware; helper ownership check ở application layer; `GET /me`.

**AC:** Bảng test RBAC (table-driven) cho mọi permission ở SPEC-01 §2.

### ACS-008 · User management API + UI
S4 · 1.5d · FS · P0 · Deps: ACS-007 · FR-AUTH-02
- API `/users` CRUD, gán role; khóa user → revoke sessions; audit.
- UI Settings › Users & Roles.

**AC:** Khóa user đang đăng nhập → request tiếp theo của user đó trả 401.

### ACS-009 · Audit log service
S1 · 0.5d · BE · P0 · Deps: ACS-005 · FR-OPS-06
- Migration 0010 (`audit_logs`, `notifications`); `audit.Record(ctx, action, resource, before, after)`; DB role app không có UPDATE/DELETE.

**AC:** Thử UPDATE `audit_logs` bằng app role → lỗi permission.

### ACS-010 · Login page + auth guard
S1 · 1d · FE · P0 · Deps: ACS-004, ACS-007
- Trang login; guard route; ẩn menu theo `permissions`; xử lý 401 → về login.

**AC:** User không có `usage.view` không thấy menu Budgets & Usage, và gọi trực tiếp API vẫn nhận 403.

---

## E02 — Job orchestration & Provider core

### ACS-011 · Job data model
S1 · 1d · BE · P0 · Deps: ACS-005
- Migration 0007–0008 (provider config, jobs, steps, events, outbox); repository; hàm chuyển trạng thái job/step có kiểm tra hợp lệ + ghi `ai_job_events`.

**AC:** Unit test bảng chuyển trạng thái; chuyển sai → error `INVALID_STATE_TRANSITION`.

### ACS-012 · Transactional outbox + dispatcher
S1 · 1d · BE · P0 · Deps: ACS-011
- `jobs.Enqueue(tx, step)` ghi outbox trong cùng TX; dispatcher poll `FOR UPDATE SKIP LOCKED` → asynq; metric `outbox_lag_seconds`.

**AC:** Kill Redis giữa chừng → khởi động lại không mất step; không enqueue trùng (asynq unique task id = step_id+attempt).

### ACS-013 · Worker runtime
S1 · 2d · BE · P0 · Deps: ACS-012
- Đăng ký handler theo `step_type`; queue `text/image/video/render/maintenance` với weight (SPEC-02 §5.4); lease + heartbeat; timeout theo loại; retry chỉ lỗi Transient/RateLimited, backoff + jitter; dead-letter → job failed; khi step xong → enqueue step phụ thuộc (DAG theo `depends_on`).
- Publish Redis pub/sub `job:{id}` sau commit.

**AC:** Test với fake provider: lỗi transient 2 lần rồi OK → succeeded, `attempt_count=3`; lỗi InvalidRequest → failed ngay; DAG 3 nhánh song song + 1 join chạy đúng thứ tự.

### ACS-014 · Provider interfaces + registry + fakes
S1 · 1d · BE · P0 · Deps: ACS-003
- Interface Text/Image/Video/Voice, `ProviderError` (SPEC-02 §4); registry đọc `ai_providers/ai_models`; fake provider có thể cấu hình delay/lỗi/kết quả mẫu (ảnh/clip/audio mẫu).

**AC:** Toàn bộ flow chạy local không cần API key thật.

### ACS-015 · Spike: POC provider (OQ-01)
S1 · 1.5d (timebox) · TL · P0 · Deps: —
- Thử ≥ 2 lựa chọn cho mỗi loại Text/Image/Video/TTS: chất lượng tiếng Việt, ảnh tham chiếu, image-to-video, thời lượng/clip tối đa, độ trễ, giá, ToS thương mại, có API cancel/status không.
- Output: `docs/spike-providers.md` + đề xuất + capabilities thực tế để chốt OQ-07.

**AC:** PO + TL ký chốt provider trước ngày 1 của S2; nếu chưa có video provider đạt → kích hoạt fallback "ảnh + Ken Burns + TTS" (FFmpeg) cho MVP.

### ACS-016 · Job API + SSE + cancel/retry
S2 · 1.5d · BE · P0 · Deps: ACS-013 · FR-OPS-02
- `GET /jobs`, `GET /jobs/{id}`, `GET /jobs/{id}/events` (SSE), `POST cancel`, `POST retry` (SPEC-04 §2.7).

**AC:** SSE nhận event trong ≤ 2s sau khi worker commit; cancel job đã xong → 409.

### ACS-017 · Idempotency-Key middleware
S2 · 0.5d · BE · P0 · Deps: ACS-011 · NFR-03
- Bắt buộc với POST tạo job; lưu `(created_by, idempotency_key, request_hash)`.

**AC:** Gửi trùng key + cùng body → trả job cũ, không tạo job/reservation mới; khác body → 409 `IDEMPOTENCY_CONFLICT`.

### ACS-018 · Reconciler + maintenance
S3 · 1d · BE · P0 · Deps: ACS-013
- Cron 5 phút: step quá lease → requeue; `waiting_provider` quá hạn → `GetStatus`; job `needs_reconciliation`; dọn file tạm/upload `pending` > 24h.

**AC:** Giả lập worker chết giữa step video → trong ≤ 10 phút step được tiếp tục và hoàn tất.

### ACS-019 · FE job tracker
S2 · 1d · FE · P0 · Deps: ACS-016
- Hook `useJob(id)` (SSE + fallback polling 3s), component progress/status/error, toast khi xong/lỗi, nút cancel/retry.

**AC:** Mất kết nối SSE → tự fallback polling, UI vẫn cập nhật.

---

## E03 — Brand, Product, Campaign, Storage

### ACS-020 · Brand API + versioning
S2 · 1d · BE · P0 · Deps: ACS-005 · FR-BRD-01, 02, 03
- Migration 0004; CRUD brand; mỗi lần đổi profile → `brand_versions` mới; optimistic lock `expected_version_no`; validate hex color.

**AC:** 2 người sửa cùng lúc → người sau nhận 409 `VERSION_CONFLICT`.

### ACS-021 · Brand Studio UI
S2 · 1.5d · FE · P0 · Deps: ACS-020
- Form profile (màu, typography, tone, từ nên dùng/cấm, CTA, personas); xem lịch sử version.

**AC:** Creator/Reviewer xem read-only.

### ACS-022 · Product API + UI
S2 · 1d · FS · P0 · Deps: ACS-020 · FR-BRD-04
- CRUD product, facts (USP, giá, offers có hạn, policies, proof có cờ verified).

**AC:** Offer hết hạn bị đánh dấu và không được đưa vào prompt.

### ACS-023 · Brand assets (logo, ảnh tham chiếu, nhạc)
S3 · 1d · FS · P0 · Deps: ACS-024 · FR-BRD-05, FR-VID-05
- Upload + gắn loại; nhạc bắt buộc nhập `license_json` (nguồn, loại license).

**AC:** Không cho dùng track nhạc thiếu license trong video.

### ACS-024 · Storage service
S1 · 1.5d · BE · P0 · Deps: ACS-005 · FR-IMG-05, NFR-07
- S3 client; `POST /files/upload-url` (presigned PUT, giới hạn size/type); `POST /files/{id}/complete` (sniff magic bytes, size, dimension/duration bằng ffprobe cho video/audio); signed GET TTL 15 phút; redact signature trong log.

**AC:** Upload file `.exe` đổi đuôi `.png` → 415 và file bị xóa.

### ACS-025 · Campaign API
S2 · 1d · BE · P0 · Deps: ACS-020 · FR-CMP-01, 02
- Migration 0005; CRUD; validate brief; state machine; campaign members.

**AC:** Chuyển `draft → completed` → 409.

### ACS-026 · Campaign UI
S2 · 1.5d · FE · P0 · Deps: ACS-025 · FR-CMP-04
- Danh sách + filter; form brief (wizard 2 bước); Campaign Detail tabs Overview/Content/Images/Videos.

**AC:** Từ tab Content/Images/Videos bấm "Tạo mới" → studio mở sẵn campaign + brief.

---

## E04 — Content Studio

### ACS-030 · Text provider adapter
S2 · 1d · BE · P0 · Deps: ACS-014, ACS-015
- Adapter cho provider đã chọn: structured output (JSON schema), usage token, map lỗi; contract test (recorded) + test `live`.

**AC:** Rate limit từ provider → `RateLimited` với `RetryAfter`.

### ACS-031 · Prompt builder
S2 · 1d · BE · P0 · Deps: ACS-030 · FR-CNT-06
- Seed `prompt_templates` cho: facebook_post, tiktok_script, caption, email, video_script, content_plan.
- Ghép: system (quy tắc + brand tone + từ cấm) | `<user_data>` (brief, product facts còn hạn, tài liệu) | output schema. Chỉ thị "không tạo giá/ưu đãi/cam kết ngoài facts".

**AC:** Snapshot test prompt; tài liệu chứa "ignore previous instructions" không thay đổi system section.

### ACS-032 · Content generation workflow
S2 · 1.5d · BE · P0 · Deps: ACS-031, ACS-016, ACS-041 · FR-CNT-01
- `POST /content/generations` → validate → estimate → reserve → job (202). Worker: gọi text → tạo N `content_assets` + version 1, gắn `generated_by_job_id`, `brand_version_id`.

**AC:** variants=3 → 3 asset, mỗi asset version 1 đầy đủ schema; job lưu provider/model/prompt version.

### ACS-033 · Rewrite & transform
S2 · 1d · BE · P0 · Deps: ACS-032 · FR-CNT-02, 03, 05
- Rewrite → version mới cùng asset; transform → asset mới có `source_asset_id` (bao gồm `video_script`).

**AC:** Rewrite không ghi đè version cũ.

### ACS-034 · Asset versions API
S2 · 1d · BE · P0 · Deps: ACS-006 · FR-CNT-04, FR-AST-02
- Migration 0006; `POST /assets/{id}/versions` (base_version_no → 409 nếu lệch), restore, list versions; cập nhật `search_tsv`; asset `approved` có version mới → về `draft`.

**AC:** Restore v1 khi đang ở v3 → tạo v4 = nội dung v1.

### ACS-035 · Content Studio UI
S2 · 2d · FE · P0 · Deps: ACS-019, ACS-034
- Panel trái: brief/tùy chọn (platform, type, framework, độ dài, tone, biến thể). Phải: tab biến thể, TipTap editor, nút viết lại nhanh, lịch sử revision, cảnh báo inline, "Gửi sang Video Studio".

**AC:** Luồng Flow A từ tạo → sửa → lưu revision chạy được end-to-end với fake provider.

### ACS-036 · Post-generation checks
S2 · 0.5d · BE · P0 · Deps: ACS-032 · FR-CNT-06, 07
- Phát hiện từ cấm; số tiền/% trong output không có trong facts → `unverified_claim`.

**AC:** Unit test với 10 mẫu output.

### ACS-037 · Content plan cho campaign
S4 · 1d · FS · P0 · Deps: ACS-032, ACS-026 · FR-CMP-03
- Job `content_plan` → plan (chủ đề, góc tiếp cận, lịch đăng, danh sách đầu ra); UI sửa/duyệt plan; "Sinh tất cả bài theo plan".

**AC:** Plan đã duyệt lưu `campaigns.content_plan`.

### ACS-038 · Prompt template management UI
S5 · 1d · FS · **P1** · Deps: ACS-031 · FR-CNT-08
- Admin/Lead tạo version template mới, kích hoạt/retire.

**AC:** Job mới dùng version active; job cũ vẫn tra được version đã dùng.

---

## E05 — Cost & Budget

### ACS-040 · Pricing + estimator
S2 · 1d · BE · P0 · Deps: ACS-011 · FR-OPS-01, NFR-10
- Migration 0009 (phần pricing/usage/budget); `cost.Estimate` cho text (token ước lượng) và image; sau mở rộng cho video ở ACS-064.

**AC:** Không có pricing hiệu lực cho model → không cho tạo job (`503 SERVICE_UNAVAILABLE` code `PRICING_MISSING`).

### ACS-041 · Budget reservation / settle / release
S2 · 1d · BE · P0 · Deps: ACS-040 · FR-OPS-04
- Reserve trong TX tạo job (khóa global → campaign); per-job limit; settle khi job kết thúc; release khi cancel/failed trước khi gọi provider.

**AC:** Test 20 request song song khi ngân sách chỉ đủ 10 → đúng ≤ 10 job được tạo.

### ACS-042 · Usage records
S2 · 0.5d · BE · P0 · Deps: ACS-041 · FR-OPS-03
- Ghi `ai_usage_records` mỗi step có gọi provider; `cost_source`; timeout sau khi đã submit → `estimated` + `needs_reconciliation`.

**AC:** Tổng `cost_actual` job = tổng usage records của job.

### ACS-043 · Concurrency cap & rate limit
S3 · 0.5d · BE · P0 · Deps: ACS-013
- Semaphore Redis theo `provider:model` và theo user (video); rate limit API tạo job (VD 30/phút/user).

**AC:** Vượt cap → step chờ (không fail); vượt rate limit API → 429 `RATE_LIMITED`.

### ACS-044 · Budget alerts
S4 · 1d · FS · P0 · Deps: ACS-041, ACS-009 · FR-OPS-05
- Ngưỡng 80/100% → notification in-app + email Lead; 100% global + hard_stop → chặn job mới (Admin override, audit).

**AC:** Mỗi ngưỡng chỉ gửi 1 lần/kỳ.

---

## E06 — Image Studio

### ACS-050 · Image provider adapter
S3 · 1d · BE · P0 · Deps: ACS-014, ACS-015
- Generate + edit; capabilities; tải output về S3 (không dùng URL user nhập); contract test.

**AC:** Tham số không hỗ trợ → 422 kèm danh sách giá trị hợp lệ.

### ACS-051 · Image generation workflow
S3 · 1.5d · BE · P0 · Deps: ACS-050, ACS-041 · FR-IMG-01, 02, 04, 05
- `POST /images/generations`, `/edits`, `/variations`; validate ref files (quyền + MIME); job → asset `image` + version + files (`parent_file_id` cho biến thể); thumbnail.

**AC:** count=4 → 1 asset với 4 file variant; ref file của campaign khác mà user không có quyền → 404.

### ACS-052 · Prompt suggestion
S3 · 0.5d · BE · P0 · Deps: ACS-030 · FR-IMG-03
- Job `image_prompt_suggest` từ brief/bài viết → 3 prompt.

**AC:** Prompt gợi ý dùng màu/tone brand.

### ACS-053 · Image Studio UI
S3 · 2d · FE · P0 · Deps: ACS-051, ACS-019
- Form prompt, preset, aspect ratio, quality, số ảnh, chọn ảnh tham chiếu (từ brand assets / library / upload); hiển thị dự toán; gallery + lightbox; biến thể / chỉnh sửa bằng prompt; checkbox typography cho poster (FR-IMG-06).

**AC:** Flow B chạy end-to-end với fake + provider thật trên staging.

### ACS-054 · Image cost rules
S3 · 0.5d · BE · P0 · Deps: ACS-040
- Estimator theo số ảnh × size × quality × model.

**AC:** Ước tính khớp bảng giá test.

---

## E07 — Video Studio

### ACS-060 · Video project + storyboard job
S3 · 1.5d · BE · P0 · Deps: ACS-032 · FR-VID-01, 02
- `POST /videos` → asset `video` + job `video_storyboard` (script → storyboard → tạo `video_scenes`); `GET /videos/{id}`; `PUT /videos/{id}/scenes` (thêm/xóa/sắp xếp, tổng thời lượng ≤ giới hạn).
- Hỗ trợ `script_from_asset_id` (kịch bản từ Content Studio).

**AC:** Storyboard 20s 9:16 → 4–6 cảnh có duration, visual_prompt, narration, subtitle.

### ACS-061 · Video provider adapter
S3 · 1.5d · BE · P0 · Deps: ACS-014, ACS-015
- Submit / GetStatus / Cancel; text-to-video & image-to-video; map trạng thái/lỗi; tải output về S3; contract test.

**AC:** Provider trả `failed` vì content policy → step `failed` với `error_kind=ContentPolicy`, không retry.

### ACS-062 · TTS adapter
S3 · 1d · BE · P0 · Deps: ACS-014, ACS-015 · FR-VID-05
- Synthesize tiếng Việt; trả audio + timings (nếu có); usage theo ký tự.

**AC:** Audio lưu S3, duration đo bằng ffprobe.

### ACS-063 · Scene generation step
S3 · 1.5d · BE · P0 · Deps: ACS-061, ACS-069 · FR-VID-04
- Step `scene_generation:<n>`: submit → `waiting_provider` → poll 10s (re-enqueue có delay, không giữ worker) → download → normalize (fps 30, resolution preset, yuv420p, H.264, AAC/silent) → `clip_file_id`.

**AC:** 5 cảnh song song, 1 cảnh lỗi → 4 cảnh `ready`, job `failed` với thông tin cảnh lỗi; không sinh lại 4 cảnh OK khi retry.

### ACS-064 · Video estimate + budget gate
S3 · 1d · BE · P0 · Deps: ACS-041, ACS-060 · FR-VID-03
- `POST /videos/{id}/estimate` (breakdown); `POST /videos/{id}/productions` kiểm tra per-job/campaign/global; override chỉ Lead/Admin + audit.

**AC:** Creator vượt per-job limit → 429 `JOB_BUDGET_EXCEEDED`, UI gợi ý giảm cảnh/thời lượng.

### ACS-065 · Voice-over & subtitle steps
S4 · 1d · BE · P0 · Deps: ACS-062 · FR-VID-05
- Voiceover từ narration (theo cảnh hoặc toàn bài); SRT từ subtitle_text + timing (theo duration cảnh hoặc TTS timings).

**AC:** SRT hợp lệ, không có đoạn chồng thời gian.

### ACS-066 · FFmpeg render worker
S4 · 2d · BE · P0 · Deps: ACS-063, ACS-065 · FR-VID-06
- Dockerfile render (ffmpeg, font tiếng Việt); builder args (không shell); concat + transition (xfade), crop/scale theo preset, mix voice + nhạc (ducking đơn giản), subtitle burn-in (ASS) hoặc soft; xuất MP4 H.264/AAC, faststart; thumbnail.

**AC:** Video 9:16, 1:1, 16:9 render đúng; phụ đề tiếng Việt có dấu hiển thị đúng.

### ACS-067 · Quality check
S4 · 0.5d · BE · P0 · Deps: ACS-066 · FR-VID-06
- ffprobe: codec, resolution, duration ±0.5s, có audio stream; lỗi → retry render 1 lần rồi fail.

**AC:** File hỏng → step `quality_check` failed, asset không chuyển sang sẵn sàng review.

### ACS-068 · Regenerate scene & re-render
S4 · 1d · BE · P0 · Deps: ACS-066 · FR-VID-07
- Job `video_scene_regenerate` (scene + render + qc) và `video_render` (chỉ render); version mới cho asset.

**AC:** Regenerate cảnh 3 → chỉ 1 usage record video mới; các cảnh khác dùng lại clip cũ.

### ACS-069 · Video workflow DAG
S3 · 1d · BE · P0 · Deps: ACS-013 · SPEC-02 §5.5
- Định nghĩa steps + `depends_on` cho `video_production`, `video_scene_regenerate`, `video_render`; progress % theo step.

**AC:** Render chỉ chạy khi mọi scene + voice + subtitle bắt buộc đã succeeded.

### ACS-071 · Storyboard editor UI
S3 · 2d · FE · P0 · Deps: ACS-060, ACS-064 · FR-VID-02, 03, 08
- Wizard: nguồn (prompt/ảnh/kịch bản), preset, thời lượng, voice/subtitle/music; danh sách cảnh kéo-thả, sửa prompt/lời thoại/thời lượng; panel dự toán + nút "Sản xuất".

**AC:** Tổng thời lượng vượt giới hạn → chặn nút sản xuất với thông báo rõ.

### ACS-072 · Video production UI
S4 · 1.5d · FE · P0 · Deps: ACS-071, ACS-068 · FR-VID-07, 08
- Trạng thái từng cảnh (thumbnail/preview), nút sinh lại cảnh, timeline đơn giản, player MP4, nút render lại, gửi duyệt.

**AC:** Flow C chạy end-to-end trên staging với provider thật.

---

## E08 — Review & Asset Library

### ACS-080 · Approval API
S4 · 1d · BE · P0 · Deps: ACS-034 · FR-AST-03
- submit-review / approve / request-changes; chặn tự duyệt; checklist typography cho poster; notification cho reviewer/người tạo; audit.

**AC:** Bảng test chuyển trạng thái asset đầy đủ.

### ACS-081 · Review queue UI
S4 · 1.5d · FE · P0 · Deps: ACS-080 · FR-AST-04
- Danh sách `pending_review`; màn review hiển thị nội dung/ảnh/video + diff với version trước (text) + comment.

**AC:** Reviewer duyệt/yêu cầu sửa không cần rời trang queue.

### ACS-082 · Library search API
S4 · 1d · BE · P0 · Deps: ACS-034 · FR-AST-01
- Full-text (`search_tsv`, unaccent) + filters + cursor.

**AC:** p95 < 500ms trên dataset seed 10k asset.

### ACS-083 · Asset Library UI
S4 · 1.5d · FE · P0 · Deps: ACS-082 · FR-AST-01, 02
- Grid/list, bộ lọc, chi tiết asset (versions, provenance: job/provider/model/prompt version/brand version).

**AC:** Lọc theo campaign + type + status đồng thời.

### ACS-084 · Download
S4 · 0.5d · BE · P0 · Deps: ACS-080, ACS-024 · FR-AST-05
- `POST /assets/{id}/download-url`; quy tắc approved; audit mỗi lần tải.

**AC:** Creator tải asset `draft` → 403.

### ACS-085 · Tags & quyền sử dụng
S5 · 0.5d · FS · **P1** · Deps: ACS-083 · FR-AST-06

---

## E09 — AI Operations & Dashboard

### ACS-090 · Provider / model / pricing admin
S5 · 1.5d · FS · P0 · Deps: ACS-040 · FR-OPS-01
- API + UI: bật/tắt provider, `secret_ref`, capabilities, concurrency, model mặc định, thêm pricing version (không sửa bản cũ); audit.

**AC:** Response API không bao giờ chứa giá trị secret.

### ACS-091 · Job monitor UI
S5 · 1d · FE · P0 · Deps: ACS-016 · FR-OPS-02
- Bảng job lọc theo status/type/provider/user; chi tiết steps/events/lỗi; retry/cancel.

### ACS-092 · Usage summary
S4 · 1.5d · FS · P0 · Deps: ACS-042 · FR-OPS-03
- `GET /usage/summary` group_by; UI biểu đồ theo ngày/campaign/provider, tách `provider_reported/calculated/estimated`.

**AC:** Tổng trên UI khớp SUM DB.

### ACS-093 · Budget settings UI
S4 · 0.5d · FE · P0 · Deps: ACS-041 · FR-CMP-05, FR-OPS-04
- Sửa global/per-job/campaign budget, ngưỡng cảnh báo.

### ACS-094 · Audit log UI
S5 · 0.5d · FE · P0 · Deps: ACS-009

### ACS-095 · Dashboard
S4 · 1d · FS · P0 · Deps: ACS-016, ACS-080 · FR-DSH-01, 02
- `GET /dashboard/summary`; widgets theo quyền; lối tắt.

---

## E10 — Hardening, Deploy & Pilot

### ACS-100 · Security tests: RBAC & IDOR
S5 · 1d · BE · P0 · Deps: hầu hết API
- Test tự động ma trận role × endpoint; truy cập resource của user khác (asset, file, job).

**AC:** 0 lỗi IDOR; báo cáo kèm danh sách endpoint đã test.

### ACS-101 · Security review: file, SSRF, FFmpeg, prompt injection
S5 · 1d · TL · P0
- Checklist SPEC-02 §7; thử upload độc hại, URL nội bộ, ký tự đặc biệt trong phụ đề, prompt injection trong tài liệu.

### ACS-102 · Secrets & key rotation
S5 · 0.5d · BE · P0 · Deps: ACS-090
- Kết nối secret manager (hoặc env được quản lý); runbook rotate/thu hồi key; kiểm tra log không lộ secret/signed URL.

### ACS-103 · Backup & restore drill
S5 · 0.5d · BE · P0 · NFR-08
- pg_dump hằng ngày + retention; S3 versioning; **diễn tập restore** lên môi trường tạm và ghi biên bản.

### ACS-104 · Observability
S4 · 1d · BE · P0 · NFR-06
- Metrics SPEC-02 §8, dashboard Grafana (hoặc tương đương), alert job failed rate / outbox lag / ngân sách.

### ACS-105 · E2E tests
S5 · 1.5d · FE · P0
- Playwright cho Flow A, B, C (fake provider) + login/RBAC cơ bản; chạy trên CI.

### ACS-106 · Staging deploy + CD
S3 · 1d · BE · P0 · Deps: ACS-002
- Docker images api/worker/render/web, Nginx (HTTPS, SSE không buffer), docker-compose staging, auto deploy nhánh main.

**AC:** Staging có provider thật với ngân sách riêng nhỏ.

### ACS-107 · Production deploy + go-live checklist
S5 · 1d · BE · P0 · Deps: ACS-103, ACS-104
- Tách DB/bucket prod; seed roles, admin, providers, pricing, budget (OQ-03); checklist go-live.

### ACS-108 · Pilot & stabilization
S5 · 2d (dev) · FS + PO · P0
- 3–5 user marketing chạy 10 brief thật; đo thời gian brief → nháp, chi phí/asset, tỉ lệ cần sửa; fix bug ưu tiên.

**AC:** Báo cáo pilot so với baseline KPI (ACS-110).

### ACS-109 · Hướng dẫn sử dụng
S5 · 0.5d · PO · P0
- Tài liệu ngắn + video hướng dẫn 3 flow; quy định review & bản quyền nhạc/ảnh.

### ACS-110 · Chuẩn bị dữ liệu & baseline KPI
S2 · (PO, không tính capacity dev) · PO · P0
- Nhập 1–2 brand thật, 5 product với facts đã xác thực, 10 brief mẫu; đo thời gian hiện tại brief → bản nháp làm baseline.
