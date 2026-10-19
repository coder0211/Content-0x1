# AI Content Studio — Bộ tài liệu triển khai MVP

Nguồn: PRD "AI Content Studio" (ChatGPT share, v1.0). Bộ tài liệu này chuyển PRD thành **spec có thể code được**, **kế hoạch sprint** và **backlog task**.

## Quyết định đã chốt

| Hạng mục         | Quyết định                                                                                                                                                                     |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Mô hình sản phẩm | Công cụ **nội bộ**, 1 workspace công ty                                                                                                                                        |
| Đầu ra           | Text · Image · Video (cả 3 trong MVP)                                                                                                                                          |
| Mức độ video     | AI sinh video từ prompt/ảnh **và dựng hoàn chỉnh** (voice-over, phụ đề, nhạc, MP4)                                                                                             |
| Xuất bản         | **Tải file thủ công**, chưa auto-post mạng xã hội                                                                                                                              |
| Frontend         | React + Vite + TypeScript + Tailwind, style "công cụ làm việc" (SPEC-05)                                                                                                       |
| Backend          | Go modular monolith, REST `/api/v1`                                                                                                                                            |
| Code structure   | Clean Architecture theo module nghiệp vụ, cả BE và FE, kiểm tra bằng lint trên CI (SPEC-06)                                                                                    |
| Data             | PostgreSQL (nguồn sự thật), Redis (queue/lock), S3-compatible storage                                                                                                          |
| Media            | FFmpeg chạy trong render worker riêng                                                                                                                                          |
| AI               | Adapter cho Text / Image / Video / Voice — không khóa vào 1 provider                                                                                                           |
| Timeline         | PRD tham chiếu 8 tuần; kế hoạch khuyến nghị **10 tuần = 5 sprint × 2 tuần** (19/10 – 25/12/2026), team 2 dev + TL part-time + 1 PO (lý do & phương án 8 tuần: xem sprint plan) |

## Cấu trúc tài liệu

| File                                                                   | Nội dung                                                                                                                          |
| ---------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| [spec/SPEC-01-product.md](spec/SPEC-01-product.md)                     | Mục tiêu, phạm vi, roles/RBAC, module, functional requirements (FR) + acceptance criteria, user flows, NFR, DoD                   |
| [spec/SPEC-02-architecture.md](spec/SPEC-02-architecture.md)           | Kiến trúc, cấu trúc source Go/React, provider interface, queue & workflow, cost control, bảo mật                                  |
| [spec/SPEC-03-database.md](spec/SPEC-03-database.md)                   | ERD, state machine, migration SQL đầy đủ                                                                                          |
| [spec/SPEC-04-api.md](spec/SPEC-04-api.md)                             | API contract: envelope, lỗi, toàn bộ endpoint + request/response                                                                  |
| [spec/SPEC-05-design-guidelines.md](spec/SPEC-05-design-guidelines.md) | Style "công cụ làm việc", danh sách cấm anti-AI look, design tokens, quy tắc component, layout từng màn, microcopy, accessibility |
| [spec/SPEC-06-code-structure.md](spec/SPEC-06-code-structure.md)       | Clean Architecture cho Go và React: layer, quy tắc phụ thuộc, cây thư mục, code mẫu, kiểm tra kiến trúc tự động                   |
| [plan/SPRINT-PLAN.md](plan/SPRINT-PLAN.md)                             | 4 sprint: mục tiêu, phạm vi, demo, exit criteria, milestone, rủi ro                                                               |
| [tasks/BACKLOG.md](tasks/BACKLOG.md)                                   | Epic → task, ước lượng, phụ thuộc, acceptance criteria                                                                            |
| [tasks/backlog.csv](tasks/backlog.csv)                                 | Backlog dạng CSV để import Jira                                                                                                   |

## Quy ước ID

- `FR-<module>-<nn>`: functional requirement (VD `FR-VID-03`).
- `NFR-<nn>`: yêu cầu phi chức năng.
- `E<nn>`: epic. `ACS-<nnn>`: task trong backlog.
- `OQ-<nn>`: câu hỏi mở cần PO/Tech lead chốt.

## Câu hỏi mở cần chốt trước Sprint 1

| ID    | Câu hỏi                                                                                       | Owner          | Hạn                           |
| ----- | --------------------------------------------------------------------------------------------- | -------------- | ----------------------------- |
| OQ-01 | Chọn provider cụ thể cho Text / Image / Video / TTS (cần POC chất lượng tiếng Việt, giá, ToS) | Tech lead      | Cuối Sprint 1 (spike ACS-015) |
| OQ-02 | Phương thức đăng nhập: email+password nội bộ hay Google Workspace SSO ngay từ MVP?            | PO + IT        | Trước Sprint 1                |
| OQ-03 | Ngân sách AI tháng (global), ngưỡng cảnh báo (%), hạn mức mặc định per-job                    | Marketing Lead | Trước Sprint 2                |
| OQ-04 | Nguồn nhạc nền có bản quyền (thư viện nội bộ / mua license)                                   | Marketing Lead | Trước Sprint 3                |
| OQ-05 | Hạ tầng triển khai (VM + Docker Compose hay K8s nội bộ), S3 provider (AWS S3 / MinIO)         | DevOps         | Trước Sprint 1                |
| OQ-06 | Thời gian lưu trữ file tạm & asset bị reject (retention)                                      | PO             | Trước Sprint 4                |
| OQ-07 | Giới hạn video MVP: thời lượng tối đa, số cảnh tối đa, độ phân giải (phụ thuộc OQ-01)         | PO + Tech lead | Đầu Sprint 3                  |

## Chạy local (≤ 10 phút)

Yêu cầu: Docker, Go 1.26, Node ≥ 20.19 + pnpm, ffmpeg (cho render/ffprobe).

```bash
make up            # postgres 16, redis 7, minio (+bucket acs-assets), mailpit (http://localhost:8025)
make migrate       # áp dụng migrations 0001–0011
make seed          # admin demo: admin@acs.local / admin12345! (chỉ local)
make run-api       # API http://localhost:8080 (healthz, readyz, metrics, /api/v1)
make run-worker    # worker (QUEUES=text,image để giới hạn queue)
cd web && pnpm install && pnpm dev   # web http://localhost:5173
```

Khác: `make test` (không cần Docker), `make lint`, `make gen` (sqlc + OpenAPI + types FE), `make up-app` (chạy api/worker trong container). Quy ước cho developer/agent: [docs/CONVENTIONS.md](docs/CONVENTIONS.md). Quyết định đã chốt: [docs/DECISIONS.md](docs/DECISIONS.md).
