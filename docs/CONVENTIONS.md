# CONVENTIONS — rules for every agent working on `api/`

Binding for all module agents. Specs: SPEC-06 (structure), SPEC-04 (API), SPEC-03 (DB). Open questions: `docs/DECISIONS.md`.
If something here conflicts with a spec, this file wins for *mechanics* (paths, commands); the spec wins for *behaviour*.

## 0. Golden rules

1. **Stay in your module.** You own `api/internal/modules/<m>/`, `api/openapi/<m>.yaml`, `web/src/modules/<m>/`, and migrations `00NN_<m>_*.sql`. Shared files you may touch: exactly **one line** in `api/internal/app/modules.go`, **one row** in `api/internal/archtest/arch_test.go` (`allowedDeps`, only for a new module), **one entry** in `api/sqlc.yaml`. Anything else (kernel, ports, platform, httpx, other modules' contracts) → do not edit; record the need in `docs/DECISIONS.md` under your agent heading and work around it inside your module.
2. **Never break a published contract** (`modules/*/usecase/api.go`, `ports/*`, `kernel/*`). Additive changes only, and only in your own module's `api.go`.
3. **Do not commit.** The orchestrator commits.
4. `cd api && go build ./... && go vet ./... && go test ./...` must pass **without Docker**. Also run `make lint-go` if you can (golangci-lint, pinned).
5. User-facing messages (`DomainError.Message`) are **Vietnamese**; codes, logs, identifiers and comments are English.

## 1. Layout of a module

```
internal/modules/<m>/
  domain/            entities, value objects, state machines, pure rules (stdlib + kernel only)
  usecase/
    api.go           PUBLIC contract for other modules (interface + DTOs)
    ports.go         repositories/ports this module needs (declared here, implemented in adapter)
    <use_case>.go    one interactor per file; permission check via kernel.Actor; tx via ports.TxManager
  adapter/
    http/            handler.go (Routes), dto.go (snake_case JSON), tests with httptest
    postgres/        queries.sql → sqlc output (db.go, models.go, queries.sql.go) + repository.go
    worker/          StepHandlers (job engine calls them)
  <m>test/           in-memory fake of usecase.API for OTHER modules' tests (exported)
  module.go          Register(c *registry.Container) (registry.Module, error)
```

Copy `internal/modules/_template` to start (it has tests in all three layers). The leading `_` keeps the template out of `./...`; Makefile/CI test it explicitly.

### Dependency rules (enforced by `internal/archtest` in `go test ./...` and by depguard in `golangci-lint`)

| From | May import |
|---|---|
| `domain` | stdlib, `shared/kernel`, uuid, decimal, own `domain` |
| `usecase` | `domain`, `ports`, `kernel`, other modules' **`usecase`** (only those allowed by SPEC-06 §2.5 table in `archtest.allowedDeps`) — never `platform`, `providers`, `net/http`, pgx, asynq, chi, redis, minio |
| `adapter/*` | own `usecase`/`domain`, `platform/*` (httpx, postgres…), other modules' `usecase` |
| `module.go` | anything of its own module, `app/registry`, `platform/*`, other modules' `usecase` |
| `<b>test` fakes | only from `_test.go` files (and from module `b`'s own `module.go` placeholder) |

Never import another module's `adapter`, `domain`, or root package. Never query another module's tables — call its `usecase.API`.

## 2. Registering a module

1. `module.go`:
   ```go
   func Register(c *registry.Container) (registry.Module, error) {
       repo := postgres.NewRepository(c.DB)
       svc := &usecase.Service{Tx: c.Tx, Repo: repo, Clock: c.Clock, Audit: c.APIs.Audit, Jobs: c.APIs.Jobs /* … */}
       c.APIs.<Field> = svc            // publish your API (field already exists in registry.APIs)
       return &Module{svc: svc, handler: &http.Handler{Svc: svc}}, nil
   }
   ```
   Optional interfaces on `*Module`: `Routes(chi.Router)` (registry.HTTPModule), `StepHandlers() []jobuc.StepHandler` (registry.StepModule), `Runners(registry.Process) []registry.Runner` (registry.RunnerModule).
2. Add **one line** to `api/internal/app/modules.go` → `Modules` (order = dependency order; image/video/dashboard slots are pre-commented).
3. Modules that already have a placeholder `module.go` (job, budget, asset, brand, campaign, provider, content): **replace the body of `Register`**, keep the signature and the `c.APIs.<Field>` assignment. Keep the `<m>test` fake compiling against your `api.go` — other agents' tests use it.
4. `registry.Platform` fields may be nil in unit tests (`TestComposeAllModulesWithoutDrivers` composes every module with an empty platform): do not call drivers inside `Register`.

Published APIs (`registry.APIs`): `Audit` (ports.AuditRecorder+Notifier), `Identity`, `Authenticator`, `Files`, `Jobs`, `Steps`, `Budget`, `Assets`, `Brands`, `Campaigns`, `Providers`, `Content`.

## 3. HTTP

- Routes are registered on the `/api/v1` router, which already runs `Authn` (cookie → actor) and `CSRF` (non-GET needs `X-CSRF-Token`). Add per-route guards:
  ```go
  r.Group(func(r chi.Router) {
      r.Use(httpx.RequirePermission(kernel.PermGenerateRun))
      r.With(httpx.RequireIdempotencyKey).Post("/content/generations", h.generate) // every POST that creates an AI job
  })
  ```
  `RequirePermission` is coarse RBAC; **ownership/IDOR checks belong in the use case** (`actor.CanModifyOwned(ownerID)`; return `kernel.ErrNotFound` for resources the actor may not see).
- Handler shape: `httpx.Decode` (rejects unknown fields) → use case → `httpx.OK|Created|Accepted|NoContent|List` or `httpx.Error(w, r, err)`. Path IDs: `httpx.PathID(r, "id")`. Pagination: `httpx.PageRequest(r)` + `kernel.PageFromRows` + `httpx.List` (sets `meta.next_cursor`).
- AI job creation: read `httpx.IdempotencyFrom(r)` and pass `Key`/`RequestHash` into `jobuc.JobSpec`; respond `202` with `{job_id, status, estimated_cost:{amount,currency,is_estimate:true}}` (schema `JobAccepted`).
- JSON is snake_case; times RFC3339 UTC; nullable fields are explicit `null`, lists are `[]` never `null`.
- SSE: `platform/sse` (`sse.Start`, `Pump`), events from `ports.EventSubscriber`.

## 4. Errors

- Return `*kernel.DomainError` from domain/use cases. Use sentinels (`kernel.ErrNotFound`, `ErrForbidden`, `ErrInvalidTransition`, `ErrVersionConflict`, `ErrBudgetExceeded`, …) and derive with `.WithMessage("…tiếng Việt…")` / `.WithDetails(kernel.Detail{…})` — **never mutate sentinels**. Validation: `kernel.ValidationErrors` (`v.Add("brief.target_audience", "required")`) → 400 with all details.
- Status mapping lives in `httpx` (SPEC-04 §1.2). New module-specific codes: `httpx.RegisterErrorCode("MY_CODE", 409)` in your `adapter/http` `init()`. Unknown codes map to 400. Non-domain errors → logged 500 `INTERNAL` (cause never sent to the client).
- Compare with `errors.Is(err, kernel.ErrNotFound)` (matches by code).

## 5. Database

- Migrations: `api/migrations/00NN_<module>_<what>.sql`, goose format (`-- +goose Up` / `-- +goose Down`; wrap `$$` bodies in `StatementBegin/End`). 0001–0011 are the base schema (SPEC-03) — **never edit them**; add `0012+`. To avoid number clashes between parallel agents, use your module's reserved block: identity/audit/file 0012–0019, job 0020–0029, budget 0030–0039, brand 0040–0049, campaign 0050–0059, asset 0060–0069, provider 0070–0079, content 0080–0089, image 0090–0099, video 0100–0109, dashboard 0110–0119. Goose applies them in numeric order; gaps are fine.
- Queries: `internal/modules/<m>/adapter/postgres/queries.sql`; add an entry to `api/sqlc.yaml` (copy an existing `sql:` item, change the two paths). Generate with `make sqlc` (pinned `sqlc v1.30.0` via `go run`). **Commit generated code**; CI fails if it is stale.
- Type mapping: uuid → `uuid.UUID` (= `kernel.ID`), nullable → pointers; numeric → `decimal.Decimal`; jsonb → `json.RawMessage`; timestamptz → `time.Time`/`*time.Time`.
- Repositories use `New(db.Conn(ctx, r.pool))` so they join the use case's transaction (`ports.TxManager.WithinTx`; nested calls reuse the outer tx). Map `pgx.ErrNoRows` with `db.MapNotFound`, unique violations with `db.IsUniqueViolation`.
- `NULL`-able computed text columns must be `COALESCE`d before scanning into `string`.
- `audit_logs` is append-only (trigger). Write audit rows through `c.APIs.Audit.Record(ctx, action, resourceType, resourceID, before, after)` inside your transaction; actor/IP/request id come from ctx.

## 6. Jobs, steps, budget (AI work)

Flow for any AI request (SPEC-06 §2.4):
```
handler → usecase: Require(generate.run) → validate capabilities (Providers.ResolveModel) → Budget.Estimate
  → Tx.WithinTx { res := Jobs.Create(spec)   // Existing=true → return it, do NOT reserve again
                  Budget.Reserve(ReserveRequest{JobID: res.JobID, …}) }   → 202
worker → StepHandler.Handle(ctx, sc) → provider via c.AI (ports.AIProviders) → Files.Store / Assets.CreateAsset
       → Budget.RecordUsage (every provider call, also on failure after the provider may have charged: Estimated=true)
job finishes → app/hooks.go calls Budget.Settle (do not call Settle yourself)
```
- Step handler: `StepType()`, `Queue()` (`jobuc.QueueText|Image|Video|Render|Maintenance`), `Timeout()`, `Handle()`. Return `*ports.ProviderError` for vendor failures (retried only if Transient/RateLimited), `jobuc.Permanent(err)` or a `DomainError` for non-retryable failures, `StepResult{Wait: &WaitSpec{…}}` to poll asynchronously (never block a worker), `StepResult{JobResult: {"asset_ids": …}}` from the last step.
- Workers never fetch user-supplied URLs (SSRF): inputs come from `Files.Open`; outputs go to `Files.Store`.
- Budget categories: `app.CategoryFor(jobType)`.

## 7. Files & storage

- Browser uploads: `POST /files/upload-url` → PUT to the presigned URL → `POST /files/{id}/complete`. Purposes: `brand_logo, reference_image, image_mask, video_source, music, document`.
- Other modules reference files by ID and must call `Files.RequireReady(ctx, id, kinds...)` plus their own access check before using one. Signed URLs: `Files.SignedURL(ctx, id, downloadName)` (TTL ≤ 15 min). Downloads of assets must be audited by the asset module.

## 8. OpenAPI

- Each module owns `api/openapi/<m>.yaml` containing **only** `paths:` and `components:`. Reuse base components: `Meta`, `ListMeta`, `Error`, `Money`, `JobAccepted`, `UUID`, parameters `Limit`, `Cursor`, `IdempotencyKey`, `CSRFToken`, `PathID`, responses `BadRequest`, `Unauthorized`, `Forbidden`, `NotFound`, `Conflict`, `PayloadTooLarge`, `UnsupportedMediaType`, `Unprocessable`, `Locked`, `TooManyRequests`, `ServiceUnavailable`, `NoContent`.
- Paths are relative to `/api/v1`. Prefix your component names with your domain (`BrandProfile`, `VideoScene`) to avoid collisions — duplicates fail the bundle.
- Run `make openapi` (bundles into `api/openapi.yaml`, checks dangling `$ref`s); a Go test fails when the bundle is stale. Never edit `api/openapi.yaml` by hand. FE types: `make gen-web`.
- Inside flow mappings `{ … }` quote any string containing a comma.

## 9. Testing

| Layer | How | Docker? |
|---|---|---|
| domain | table-driven unit tests | no |
| usecase | fakes: `internal/testutil` (Clock, Tx, Audit, Notifier, Events, Storage, `Actor(roles…)`), other modules' `<m>test` fakes, in-memory repos in your `<m>test` or `_test.go` | no |
| adapter/http | `httptest` through `httpx.NewRouter` with a fake `httpx.Authenticator` (see `identity/adapter/http/handler_test.go`, `_template`) | no |
| adapter/postgres | `pgtest.Postgres(t)` (`internal/testutil/pgtest`): migrated Postgres 16 via testcontainers; **auto-skips** without Docker or with `-short` | yes (skips) |
| providers | recorded/fake; live tests behind `//go:build live` | no |

Every module ships tests in all three layers. Cover RBAC (table over roles) and IDOR (other user's resource → 404).

## 10. Commands

```
make up              # postgres, redis, minio (+bucket acs-assets), mailpit
make migrate seed    # schema + demo admin admin@acs.local / admin12345!
make run-api         # :8080   (make run-worker QUEUES=text,image for workers)
make test            # Go (+_template) and web unit tests, no Docker needed
make lint            # go vet + archtest + golangci-lint (pinned) + web lint/typecheck
make gen             # sqlc + openapi bundle + web API types
```

## 11. Logging & security

- Use `c.Logger` / `slog.*Context(ctx, …)`; request_id/user_id are added automatically, add `job_id`/`step_id` with `log.With(ctx, slog.String("job_id", …))`. Never log secrets, cookies, tokens, signed URLs (the logger redacts known keys, but do not rely on it).
- Provider keys: read via `config.Secret(secretRef)`; never store keys in the DB or return them to the FE.
- FFmpeg: only through `ports.VideoRenderer` (argument vectors, user text only via subtitle files).
