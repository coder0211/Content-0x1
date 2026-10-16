# SPEC-06 — Code Structure (Clean Architecture)

Áp dụng Clean Architecture cho cả backend (Go) và frontend (React), tổ chức theo **module nghiệp vụ** (vertical slice). Bên trong mỗi module chia layer theo quy tắc phụ thuộc hướng vào trong. Thay thế cấu trúc sơ bộ ở SPEC-02 §3.

## 1. Quy tắc chung

```
        ┌──────────────────────────────────────────┐
        │ Frameworks & Drivers (platform, main)    │  pgx, redis, S3, chi, ffmpeg, React DOM
        │  ┌────────────────────────────────────┐  │
        │  │ Interface Adapters (adapter)       │  │  HTTP handler, worker handler,
        │  │  ┌──────────────────────────────┐  │  │  postgres repo, provider adapter
        │  │  │ Use cases (usecase)          │  │  │  interactor + port (interface)
        │  │  │  ┌────────────────────────┐  │  │  │
        │  │  │  │ Domain (domain)        │  │  │  │  entity, value object, rule
        │  │  │  └────────────────────────┘  │  │  │
        │  │  └──────────────────────────────┘  │  │
        │  └────────────────────────────────────┘  │
        └──────────────────────────────────────────┘
              Phụ thuộc chỉ được hướng VÀO TRONG
```

| Quy tắc                                | Chi tiết                                                                                                                                                                        |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| R1. Dependency rule                    | `domain` không import gì ngoài stdlib và `shared/kernel`. `usecase` chỉ import `domain` và port. `adapter` import `usecase`/`domain`. Chỉ `cmd/` (composition root) biết tất cả |
| R2. Port do người dùng định nghĩa      | Interface (repository, AI provider, storage, queue, clock) khai báo ở **usecase** (nơi dùng), adapter implement                                                                 |
| R3. Không lộ framework vào trong       | Không có `pgx`, `http.Request`, `asynq`, struct JSON của vendor trong `domain`/`usecase`                                                                                        |
| R4. Module giao tiếp qua API công khai | Module A chỉ gọi module B qua interface trong `B/usecase/api.go` (hoặc domain event), **không** import `B/adapter` hay bảng DB của B                                            |
| R5. Transaction thuộc use case         | Use case mở transaction qua port `TxManager`; repository lấy tx từ `context`                                                                                                    |
| R6. DTO tách biệt                      | HTTP DTO ≠ use case input/output ≠ domain entity ≠ DB row. Mapping nằm ở adapter                                                                                                |
| R7. Kiểm tra bằng công cụ              | `depguard`/`go-arch-lint` (BE) và `eslint-plugin-boundaries` (FE) chạy trên CI; vi phạm thì build fail                                                                          |

## 2. Backend (Go)

### 2.1. Cây thư mục

```
api/
├── cmd/
│   ├── server/main.go            # composition root: đọc config, tạo platform, wire module, chạy HTTP
│   ├── worker/main.go            # composition root cho worker (--queues=text,image,...)
│   └── acs/main.go               # CLI: tạo admin, seed, migrate
├── internal/
│   ├── shared/
│   │   └── kernel/               # dùng chung mọi layer, thuần Go
│   │       ├── id.go             # type ID (UUID), NewID
│   │       ├── money.go          # Money{Amount decimal, Currency}
│   │       ├── errors.go         # DomainError{Code, Message, Details}, ErrNotFound, ErrConflict...
│   │       ├── page.go           # Cursor, Page[T]
│   │       └── actor.go          # Actor{UserID, Permissions} (người thực hiện use case)
│   │
│   ├── ports/                    # port dùng chung nhiều module (usecase-level interfaces)
│   │   ├── tx.go                 # TxManager
│   │   ├── clock.go              # Clock
│   │   ├── ai.go                 # TextGenerator, ImageGenerator, VideoGenerator, VoiceSynthesizer + DTO + ProviderError
│   │   ├── storage.go            # ObjectStorage (Presign, Put, Get, Delete, Stat)
│   │   ├── media.go              # MediaProber, VideoRenderer
│   │   ├── events.go             # EventPublisher (job progress → SSE)
│   │   └── audit.go              # AuditRecorder
│   │
│   ├── modules/
│   │   ├── identity/             # auth, users, roles, sessions, RBAC
│   │   ├── brand/                # brand, brand versions, brand assets, products
│   │   ├── campaign/             # campaign, brief, content plan, members
│   │   ├── asset/                # content_assets, versions, files, approvals, library search, download
│   │   ├── job/                  # ai_jobs/steps/events, outbox, workflow engine, retry, reconcile
│   │   ├── budget/               # pricing, estimator, budgets, reservations, usage, alerts
│   │   ├── content/              # use case sinh/viết lại/chuyển đổi content, prompt builder, post-checks
│   │   ├── image/                # use case sinh/sửa/biến thể ảnh, prompt suggest
│   │   ├── video/                # video project, scenes, storyboard, production workflow, render
│   │   ├── provider/             # quản lý ai_providers/ai_models/prompt_templates (cấu hình)
│   │   ├── audit/                # audit log + notifications
│   │   └── dashboard/            # read model tổng hợp
│   │
│   ├── providers/                # adapter AI: implement ports/ai.go
│   │   ├── registry.go           # chọn adapter theo provider_code/model
│   │   ├── openai/               # ví dụ: text + image
│   │   ├── <video_vendor>/
│   │   ├── <tts_vendor>/
│   │   └── fake/                 # dùng cho local/test
│   │
│   └── platform/                 # frameworks & drivers, không chứa nghiệp vụ
│       ├── config/
│       ├── log/                  # slog + redact
│       ├── postgres/             # pgx pool, TxManager impl (tx trong ctx), sqlc querier factory
│       ├── redis/
│       ├── queue/                # asynq client/server wrapper
│       ├── s3/                   # implement ports.ObjectStorage
│       ├── ffmpeg/               # implement ports.MediaProber, ports.VideoRenderer (exec args, không shell)
│       ├── httpx/                # chi router, middleware (request_id, auth, csrf, rbac, idempotency), envelope, error mapping
│       ├── sse/
│       └── metrics/
├── migrations/
├── sqlc.yaml
└── openapi.yaml
```

### 2.2. Cấu trúc bên trong một module

Ví dụ `modules/content/` (các module khác theo cùng khuôn):

```
modules/content/
├── domain/
│   ├── content.go            # ContentDraft, Variant, Platform, Framework (value object + validate)
│   ├── checks.go             # rule: forbidden words, unverified claims (thuần, test dễ)
│   └── errors.go             # ErrInvalidVariantCount...
├── usecase/
│   ├── api.go                # interface công khai cho module khác (R4)
│   ├── ports.go              # port riêng của module (VD PromptTemplateReader)
│   ├── generate_content.go   # GenerateContent interactor: validate → estimate → reserve → create job
│   ├── run_generation_step.go# xử lý step khi worker gọi: build prompt → TextGenerator → tạo asset/version
│   ├── rewrite_content.go
│   ├── transform_content.go
│   └── prompt/
│       └── builder.go        # ghép system / <user_data> / schema (thuần, snapshot test)
├── adapter/
│   ├── http/
│   │   ├── handler.go        # parse DTO → gọi usecase → presenter
│   │   ├── dto.go            # request/response JSON (snake_case)
│   │   └── routes.go
│   ├── worker/
│   │   └── steps.go          # đăng ký step_type "content_generation" với job engine
│   └── postgres/             # (nếu module có bảng riêng) repository impl dùng sqlc
│       ├── queries.sql
│       └── repository.go
└── module.go                 # constructor: New(deps) → *Module{Handler, StepHandlers, API}
```

Trách nhiệm từng layer:

| Layer              | Chứa                                                                                                  | Không được chứa                        |
| ------------------ | ----------------------------------------------------------------------------------------------------- | -------------------------------------- |
| `domain`           | Entity, value object, state machine (VD `asset.Status.CanTransition`), rule nghiệp vụ thuần           | I/O, context, SQL, JSON tag của API    |
| `usecase`          | Interactor (1 file = 1 use case), port, input/output struct, kiểm tra quyền theo `Actor`, transaction | HTTP status, SQL, gọi SDK vendor       |
| `adapter/http`     | Parse/validate request, map lỗi domain → HTTP (qua `httpx`), presenter                                | Logic nghiệp vụ, truy cập DB trực tiếp |
| `adapter/worker`   | Nhận step từ job engine, gọi use case                                                                 | Logic retry (do module `job` lo)       |
| `adapter/postgres` | Implement repository port bằng sqlc, map row ↔ entity                                                 | Rule nghiệp vụ                         |

### 2.3. Code mẫu

**Domain** (`modules/asset/domain/asset.go`)

```go
package domain

type Status string

const (
    StatusDraft            Status = "draft"
    StatusPendingReview    Status = "pending_review"
    StatusChangesRequested Status = "changes_requested"
    StatusApproved         Status = "approved"
    StatusArchived         Status = "archived"
)

var transitions = map[Status][]Status{
    StatusDraft:            {StatusPendingReview, StatusArchived},
    StatusPendingReview:    {StatusApproved, StatusChangesRequested},
    StatusChangesRequested: {StatusDraft},
    StatusApproved:         {StatusDraft, StatusArchived},
}

type Asset struct {
    ID        kernel.ID
    CreatedBy kernel.ID
    Status    Status
    // ...
}

func (a *Asset) Approve(reviewer kernel.ID, isAdmin bool) error {
    if a.CreatedBy == reviewer && !isAdmin {
        return kernel.NewError("SELF_APPROVAL_NOT_ALLOWED", "Không thể tự duyệt asset của mình")
    }
    return a.transition(StatusApproved)
}

func (a *Asset) transition(to Status) error {
    for _, s := range transitions[a.Status] {
        if s == to {
            a.Status = to
            return nil
        }
    }
    return kernel.NewError("INVALID_STATE_TRANSITION", string(a.Status)+" → "+string(to))
}
```

**Port + use case** (`modules/asset/usecase/approve_asset.go`)

```go
package usecase

type AssetRepository interface {
    GetForUpdate(ctx context.Context, id kernel.ID) (*domain.Asset, error)
    Save(ctx context.Context, a *domain.Asset) error
    AddApproval(ctx context.Context, ap domain.Approval) error
}

type ApproveAsset struct {
    Tx     ports.TxManager
    Assets AssetRepository
    Audit  ports.AuditRecorder
    Notify NotificationSender
}

type ApproveInput struct {
    AssetID kernel.ID
    Comment string
}

func (uc ApproveAsset) Execute(ctx context.Context, actor kernel.Actor, in ApproveInput) error {
    if !actor.Can("asset.review") {
        return kernel.ErrForbidden
    }
    return uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
        a, err := uc.Assets.GetForUpdate(ctx, in.AssetID)
        if err != nil {
            return err
        }
        if err := a.Approve(actor.UserID, actor.HasRole("admin")); err != nil {
            return err
        }
        if err := uc.Assets.Save(ctx, a); err != nil {
            return err
        }
        if err := uc.Assets.AddApproval(ctx, domain.NewApproval(a, actor.UserID, "approved", in.Comment)); err != nil {
            return err
        }
        uc.Audit.Record(ctx, "asset.approve", "asset", a.ID.String(), nil, a)
        return uc.Notify.AssetApproved(ctx, a)
    })
}
```

**HTTP adapter** (`modules/asset/adapter/http/handler.go`)

```go
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
    var req approveRequest
    if err := httpx.Decode(r, &req); err != nil {
        httpx.Error(w, r, err)
        return
    }
    err := h.approve.Execute(r.Context(), httpx.ActorFrom(r), usecase.ApproveInput{
        AssetID: httpx.PathID(r, "id"),
        Comment: req.Comment,
    })
    if err != nil {
        httpx.Error(w, r, err) // map kernel.DomainError.Code → HTTP status (SPEC-04 §1.2)
        return
    }
    httpx.NoContent(w)
}
```

**Composition root** (`cmd/server/main.go`, trích)

```go
db := postgres.MustConnect(cfg.DatabaseURL)
tx := postgres.NewTxManager(db)
store := s3.New(cfg.S3)
ai := providers.NewRegistry(cfg, providerRepo) // implement ports.TextGenerator...

jobMod := job.New(job.Deps{Tx: tx, DB: db, Queue: queue.NewClient(cfg.Redis)})
budgetMod := budget.New(budget.Deps{Tx: tx, DB: db})
assetMod := asset.New(asset.Deps{Tx: tx, DB: db, Storage: store, Audit: auditMod.API})
contentMod := content.New(content.Deps{
    Tx: tx, Jobs: jobMod.API, Budget: budgetMod.API, Assets: assetMod.API,
    Brands: brandMod.API, Text: ai,
})

r := httpx.NewRouter(cfg, identityMod.Middleware)
contentMod.Routes(r)
assetMod.Routes(r)
// ...
```

### 2.4. Module job & các module tạo nội dung

Module `job` là engine chung, không biết nội dung từng loại job:

```go
// modules/job/usecase/api.go
type API interface {
    // gọi trong transaction của use case nghiệp vụ → ghi job + steps + outbox (atomic)
    Create(ctx context.Context, spec JobSpec) (JobID, error)
    RequestCancel(ctx context.Context, actor kernel.Actor, id JobID) error
}

// Module nghiệp vụ đăng ký handler cho step_type của mình
type StepHandler interface {
    StepType() string                       // "content_generation", "scene_generation", ...
    Queue() string                          // text | image | video | render
    Handle(ctx context.Context, step StepContext) (StepResult, error)
}
```

Luồng: `content/usecase.GenerateContent` → `budget.API.Reserve` → `job.API.Create` (cùng tx) → worker → `job` engine gọi `content/adapter/worker` → `content/usecase.RunGenerationStep` → `ports.TextGenerator` → `asset.API.CreateWithVersion` → `budget.API.Settle`.

### 2.5. Ranh giới module (ai được gọi ai)

| Module    | Được gọi API của                                                    |
| --------- | ------------------------------------------------------------------- |
| identity  | —                                                                   |
| audit     | —                                                                   |
| brand     | audit                                                               |
| campaign  | brand, audit                                                        |
| asset     | audit (storage qua port)                                            |
| budget    | audit                                                               |
| job       | — (chỉ phát event)                                                  |
| provider  | audit                                                               |
| content   | brand, campaign, asset, job, budget, provider                       |
| image     | brand, campaign, asset, job, budget, provider                       |
| video     | brand, campaign, asset, job, budget, provider, content (lấy script) |
| dashboard | đọc read model (query riêng, chỉ đọc)                               |

Không được có vòng phụ thuộc. `job` và `budget` không biết đến `content/image/video`.

### 2.6. Kiểm thử theo layer

| Layer            | Loại test                                                 | Công cụ                                   |
| ---------------- | --------------------------------------------------------- | ----------------------------------------- |
| domain           | Unit thuần, table-driven (state machine, rule)            | `testing`                                 |
| usecase          | Unit với fake port (in-memory repo, fake AI, fake clock)  | `testing`, fake trong `internal/testutil` |
| adapter/postgres | Integration với Postgres thật                             | testcontainers-go                         |
| adapter/http     | Handler test với use case giả + test contract với OpenAPI | `httptest`, kin-openapi validator         |
| providers/\*     | Contract test (recorded) + `live` tag                     | go-vcr                                    |
| End-to-end       | API + worker + fake provider                              | docker compose trong CI                   |

### 2.7. Kiểm tra kiến trúc tự động

```yaml
# .golangci.yml (trích) — depguard
linters-settings:
  depguard:
    rules:
      domain:
        files: ["**/modules/*/domain/**"]
        deny:
          - pkg: "github.com/jackc/pgx"
          - pkg: "net/http"
          - pkg: "github.com/hibiken/asynq"
          - pkg: "acs/internal/platform"
          - pkg: "acs/internal/providers"
      usecase:
        files: ["**/modules/*/usecase/**"]
        deny:
          - pkg: "github.com/jackc/pgx"
          - pkg: "net/http"
          - pkg: "acs/internal/platform"
          - pkg: "acs/internal/providers"
```

Bổ sung `go-arch-lint` để chặn import chéo `modules/<a>/adapter` từ `modules/<b>` (R4).

## 3. Frontend (React + TypeScript)

### 3.1. Cây thư mục

```
web/src/
├── app/                          # composition root
│   ├── main.tsx
│   ├── router.tsx                # route → page của module (lazy)
│   ├── providers.tsx             # QueryClient, Auth, Theme, Toast, DI container
│   └── layout/                   # AppShell, Sidebar, Topbar (SPEC-05 §4.1)
├── shared/
│   ├── ui/                       # design system: shadcn đã chỉnh token (SPEC-05), không chứa nghiệp vụ
│   │   ├── theme.css
│   │   ├── button.tsx  input.tsx  table.tsx  dialog.tsx  toast.tsx ...
│   │   └── patterns/             # StatusBadge, CostChip, JobProgress, AIProvenance, EmptyState
│   ├── api/
│   │   ├── schema.d.ts           # sinh từ openapi.yaml (openapi-typescript)
│   │   ├── http.ts               # fetch wrapper: envelope, CSRF, request_id, ApiError(code)
│   │   └── sse.ts
│   ├── lib/                      # format (tiền, ngày, thời lượng), hooks chung
│   └── kernel/                   # Result, ID, Money, Permission types
└── modules/
    ├── auth/  dashboard/  brand/  campaign/  content/  image/  video/
    ├── library/  review/  job/  budget/  settings/
    └── <module>/
        ├── domain/               # type + rule thuần TS (VD canSubmitReview(asset), sceneTotalDuration)
        ├── application/          # use case dạng hook: useGenerateContent, useRewrite... + port (repository interface)
        ├── infrastructure/       # repository impl gọi shared/api, mapper DTO ↔ domain
        ├── presentation/
        │   ├── pages/            # ContentStudioPage
        │   └── components/       # BriefPanel, VariantTabs, RevisionList...
        └── index.ts              # public API của module (chỉ export page + hook cần chia sẻ)
```

### 3.2. Quy tắc layer FE

| Layer            | Chứa                                                              | Được import                                        |
| ---------------- | ----------------------------------------------------------------- | -------------------------------------------------- |
| `domain`         | Type, rule thuần, không React                                     | `shared/kernel`                                    |
| `application`    | Hook use case (TanStack Query mutation/query), port `XRepository` | `domain`, `shared/kernel`                          |
| `infrastructure` | `HttpContentRepository implements ContentRepository`, mapper      | `domain`, `application` (port), `shared/api`       |
| `presentation`   | Component, page; chỉ gọi hook của `application`                   | `application`, `domain`, `shared/ui`, `shared/lib` |
| `app`            | Router, provider, đăng ký repository vào DI context               | mọi thứ                                            |

- Component **không** gọi `fetch`/`shared/api` trực tiếp.
- Module khác chỉ import qua `modules/<x>/index.ts`.
- Trạng thái server dùng TanStack Query; trạng thái UI cục bộ dùng `useState`/Zustand trong `presentation`.

### 3.3. Code mẫu

```ts
// modules/content/application/ports.ts
export interface ContentRepository {
  generate(input: GenerateContentInput, idempotencyKey: string): Promise<JobAccepted>;
  rewrite(assetId: string, input: RewriteInput, idempotencyKey: string): Promise<JobAccepted>;
}

// modules/content/application/useGenerateContent.ts
export function useGenerateContent() {
  const repo = useRepository("content");
  return useMutation({
    mutationFn: (input: GenerateContentInput) => repo.generate(input, crypto.randomUUID()),
  });
}

// modules/content/infrastructure/HttpContentRepository.ts
export class HttpContentRepository implements ContentRepository {
  async generate(input: GenerateContentInput, key: string) {
    const res = await http.post("/content/generations", toGenerateDto(input), { idempotencyKey: key });
    return toJobAccepted(res);
  }
  // ...
}

// app/providers.tsx
const repositories = {
  content: new HttpContentRepository(),
  // test/storybook: new FakeContentRepository()
};
```

### 3.4. Kiểm tra kiến trúc FE

```js
// eslint.config.js (trích) — eslint-plugin-boundaries
settings: {
  'boundaries/elements': [
    { type: 'app', pattern: 'src/app/*' },
    { type: 'shared', pattern: 'src/shared/*' },
    { type: 'domain', pattern: 'src/modules/*/domain/*', capture: ['module'] },
    { type: 'application', pattern: 'src/modules/*/application/*', capture: ['module'] },
    { type: 'infrastructure', pattern: 'src/modules/*/infrastructure/*', capture: ['module'] },
    { type: 'presentation', pattern: 'src/modules/*/presentation/*', capture: ['module'] },
  ],
},
rules: {
  'boundaries/element-types': ['error', {
    default: 'disallow',
    rules: [
      { from: 'domain', allow: ['shared'] },
      { from: 'application', allow: ['domain', 'shared'] },
      { from: 'infrastructure', allow: ['domain', 'application', 'shared'] },
      { from: 'presentation', allow: ['application', 'domain', 'shared'] },
      { from: 'app', allow: ['*'] },
    ],
  }],
},
```

### 3.5. Kiểm thử FE

| Layer          | Test                                                                       |
| -------------- | -------------------------------------------------------------------------- |
| domain         | Vitest unit                                                                |
| application    | Vitest + fake repository                                                   |
| infrastructure | Vitest + MSW (mock HTTP theo OpenAPI)                                      |
| presentation   | Testing Library cho component có logic; ảnh chụp các trạng thái SPEC-05 §5 |
| E2E            | Playwright (ACS-105)                                                       |

## 4. Quy ước đặt tên

| Thứ          | BE (Go)                                                   | FE (TS)                                            |
| ------------ | --------------------------------------------------------- | -------------------------------------------------- |
| Use case     | `GenerateContent`, file `generate_content.go`             | `useGenerateContent`, file `useGenerateContent.ts` |
| Port         | `AssetRepository`, `ports.TextGenerator`                  | `ContentRepository`                                |
| Adapter impl | `postgres.AssetRepository`, `openai.TextAdapter`          | `HttpContentRepository`                            |
| DTO          | `generateContentRequest` (unexported, trong adapter/http) | `GenerateContentDto` (trong infrastructure)        |
| Lỗi          | `kernel.DomainError{Code: "BUDGET_EXCEEDED"}`             | `ApiError.code === 'BUDGET_EXCEEDED'`              |
| Use case     | `GenerateContent`, file `generate_content.go`             | `useGenerateContent`, file `useGenerateContent.ts` |
| Port         | `AssetRepository`, `ports.TextGenerator`                  | `ContentRepository`                                |
| Adapter impl | `postgres.AssetRepository`, `openai.TextAdapter`          | `HttpContentRepository`                            |
| DTO          | `generateContentRequest` (unexported, trong adapter/http) | `GenerateContentDto` (trong infrastructure)        |
| Lỗi          | `kernel.DomainError{Code: "BUDGET_EXCEEDED"}`             | `ApiError.code === 'BUDGET_EXCEEDED'`              |

## 5. Checklist khi thêm một tính năng

1. Thêm/điều chỉnh entity và rule trong `domain` + unit test.
2. Viết use case trong `usecase` và khai báo port nó cần + test với fake.
3. Implement port ở adapter (postgres/provider) + integration test.
4. Thêm HTTP handler + DTO + cập nhật `openapi.yaml`.
5. Nếu là AI job: thêm `StepHandler` ở `adapter/worker` và đăng ký trong `module.go`.
6. FE: chạy `make gen` để sinh lại types → domain/application/infrastructure/presentation của module.
7. CI pass: arch lint (BE + FE), test, typecheck.
