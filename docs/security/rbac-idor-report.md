# ACS-100 — RBAC & IDOR test report

Date: 2026-10-19 · Branch `feat/mvp-impl` · Scope: every `/api/v1` route of every backend module.
Result: **0 RBAC mismatches, 0 open IDOR issues.** Two IDOR defects were found while writing the tests and fixed (see the [findings](#findings)).

## How it is tested

All tests run in `go test ./...` without Docker (package `api/internal/e2e`).

| Test | What it proves |
|---|---|
| `TestRBACMatrixCoversEveryRoute` | The route list is taken from the **real** chi router via `chi.Walk`. `app.Modules` is composed exactly like `cmd/server`, against a lazily connecting, unreachable Postgres. Every route must have an entry in `expectedAccess`, and every entry must exist. **A new route without an RBAC expectation fails CI.** |
| `TestRBACMatrix` | 90 routes × 5 actors (anonymous, admin, marketing_lead, content_creator, reviewer). Anonymous gets `401 UNAUTHENTICATED`. A role without the required permission gets `403 FORBIDDEN`. A role that holds it reaches the use case (anything but 401/403). Expectations are written from SPEC-04 §2 + SPEC-01 §2 (`kernel.RolePermissions`), not copied from the handlers. A mutation check (changing one expectation) fails the test. |
| `TestCSRFRequiredOnEveryUnsafeRoute` | Every authenticated non-GET route except `POST /auth/login` rejects requests without `X-CSRF-Token` (`403 CSRF_INVALID`). |
| `TestIDOR` | Runs over real use cases and HTTP handlers on in-memory repositories (the `world` harness). Creator A owns a campaign, content asset, image asset, video project, upload and jobs. Creator B (not a member) and a reviewer try 35 cross-user requests; every one must be refused. The owner's asset is unchanged afterwards. Positive controls prove the same endpoints work for the owner, and that Lead/Admin get the access SPEC-04 grants. |
| `TestFlowA` | End-to-end Flow A over the same harness (see `docs/DECISIONS.md` → Integration — backend). |

Roles allowed through the gate are derived from `kernel.RolePermissions`. "Use-case rule" is the finer ownership check that applies after the gate.

## Endpoints tested (RBAC matrix)

| # | Endpoint | Gate | Roles allowed through the gate | Use-case rule |
|---|---|---|---|---|
| 1 | `POST /auth/login` | public | anyone |  |
| 2 | `POST /auth/logout` | authenticated | all roles | own session |
| 3 | `GET /me` | authenticated | all roles | own profile |
| 4 | `GET /users` | `user.manage` | admin |  |
| 5 | `POST /users` | `user.manage` | admin |  |
| 6 | `GET /users/{id}` | `user.manage` | admin |  |
| 7 | `PATCH /users/{id}` | `user.manage` | admin | 2.2 Brand & product |
| 8 | `GET /brands` | `brand.view` | admin, lead, creator, reviewer |  |
| 9 | `POST /brands` | `brand.manage` | admin, lead |  |
| 10 | `GET /brands/{id}` | `brand.view` | admin, lead, creator, reviewer |  |
| 11 | `PATCH /brands/{id}` | `brand.manage` | admin, lead |  |
| 12 | `DELETE /brands/{id}` | `brand.manage` | admin, lead |  |
| 13 | `GET /brands/{id}/versions` | `brand.view` | admin, lead, creator, reviewer |  |
| 14 | `GET /brands/{id}/assets` | `brand.view` | admin, lead, creator, reviewer |  |
| 15 | `POST /brands/{id}/assets` | `brand.manage` | admin, lead |  |
| 16 | `DELETE /brands/{id}/assets/{brandAssetId}` | `brand.manage` | admin, lead |  |
| 17 | `GET /brands/{id}/products` | `brand.view` | admin, lead, creator, reviewer |  |
| 18 | `POST /brands/{id}/products` | `brand.manage` | admin, lead |  |
| 19 | `GET /products/{id}` | `brand.view` | admin, lead, creator, reviewer |  |
| 20 | `PATCH /products/{id}` | `brand.manage` | admin, lead | 2.3 Campaign |
| 21 | `GET /campaigns` | `brand.view` | admin, lead, creator, reviewer |  |
| 22 | `POST /campaigns` | `campaign.manage` | admin, lead, creator |  |
| 23 | `GET /campaigns/member-candidates` | `campaign.manage` | admin, lead, creator |  |
| 24 | `GET /campaigns/{id}` | `brand.view` | admin, lead, creator, reviewer |  |
| 25 | `PATCH /campaigns/{id}` | `campaign.manage` | admin, lead, creator |  |
| 26 | `POST /campaigns/{id}/plan-generations` | `generate.run` | admin, lead, creator |  |
| 27 | `PUT /campaigns/{id}/plan` | `campaign.manage` | admin, lead, creator |  |
| 28 | `DELETE /campaigns/{id}/plan-proposal` | `campaign.manage` | admin, lead, creator |  |
| 29 | `GET /campaigns/{id}/budget` | `usage.view` | admin, lead |  |
| 30 | `PUT /campaigns/{id}/budget` | `budget.manage` | admin, lead | 2.4 Content |
| 31 | `POST /content/generations` | `generate.run` | admin, lead, creator |  |
| 32 | `POST /content/{assetId}/rewrites` | `generate.run` | admin, lead, creator |  |
| 33 | `POST /content/{assetId}/transforms` | `generate.run` | admin, lead, creator | 2.5 Image |
| 34 | `POST /images/prompt-suggestions` | `generate.run` | admin, lead, creator |  |
| 35 | `GET /images/generations` | `generate.run` | admin, lead, creator |  |
| 36 | `POST /images/generations` | `generate.run` | admin, lead, creator |  |
| 37 | `GET /images/references` | `generate.run` | admin, lead, creator |  |
| 38 | `POST /images/estimate` | `generate.run` | admin, lead, creator |  |
| 39 | `POST /images/{assetId}/edits` | `generate.run` | admin, lead, creator |  |
| 40 | `POST /images/{assetId}/variations` | `generate.run` | admin, lead, creator | 2.6 Video |
| 41 | `GET /videos` | `brand.view` | admin, lead, creator, reviewer |  |
| 42 | `POST /videos` | `generate.run` | admin, lead, creator |  |
| 43 | `GET /videos/{id}` | `brand.view` | admin, lead, creator, reviewer |  |
| 44 | `PUT /videos/{id}/scenes` | `generate.run` | admin, lead, creator |  |
| 45 | `POST /videos/{id}/estimate` | `generate.run` | admin, lead, creator |  |
| 46 | `POST /videos/{id}/productions` | `generate.run` | admin, lead, creator |  |
| 47 | `POST /videos/{id}/scenes/{sceneId}/regenerations` | `generate.run` | admin, lead, creator |  |
| 48 | `POST /videos/{id}/renders` | `generate.run` | admin, lead, creator | 2.7 Jobs: owner or provider.view (reads) / owner or admin (writes), in the use case |
| 49 | `GET /jobs` | authenticated | all roles | own jobs unless provider.view |
| 50 | `GET /jobs/{id}` | authenticated | all roles | owner or provider.view, else 404 |
| 51 | `GET /jobs/{id}/events` | authenticated | all roles | owner or provider.view, else 404 |
| 52 | `GET /jobs/{id}/history` | authenticated | all roles | owner or provider.view, else 404 |
| 53 | `POST /jobs/{id}/cancel` | authenticated | all roles | owner or admin |
| 54 | `POST /jobs/{id}/retry` | authenticated | all roles | owner or admin |
| 55 | `GET /assets` | `brand.view` | admin, lead, creator, reviewer |  |
| 56 | `GET /assets/{id}` | `brand.view` | admin, lead, creator, reviewer |  |
| 57 | `PATCH /assets/{id}` | `brand.view` | admin, lead, creator, reviewer | + owner/lead in use case |
| 58 | `DELETE /assets/{id}` | `brand.view` | admin, lead, creator, reviewer | + owner/lead in use case |
| 59 | `GET /assets/{id}/versions` | `brand.view` | admin, lead, creator, reviewer |  |
| 60 | `POST /assets/{id}/versions` | `brand.view` | admin, lead, creator, reviewer | + owner/lead |
| 61 | `POST /assets/{id}/versions/{versionNo}/restore` | `brand.view` | admin, lead, creator, reviewer | + owner/lead |
| 62 | `POST /assets/{id}/submit-review` | `brand.view` | admin, lead, creator, reviewer | + owner/lead |
| 63 | `POST /assets/{id}/approve` | `asset.review` | admin, lead, reviewer |  |
| 64 | `POST /assets/{id}/request-changes` | `asset.review` | admin, lead, reviewer |  |
| 65 | `GET /reviews/queue` | `asset.review` | admin, lead, reviewer |  |
| 66 | `POST /assets/{id}/download-url` | `asset.download` | admin, lead, creator, reviewer |  |
| 67 | `POST /files/upload-url` | authenticated | all roles | uploader becomes owner |
| 68 | `POST /files/{id}/complete` | authenticated | all roles | owner only, else 404 |
| 69 | `GET /files/{id}` | authenticated | all roles | owner only, else 404 |
| 70 | `GET /settings/providers` | `provider.view` | admin, lead |  |
| 71 | `PATCH /settings/providers/{id}` | `provider.manage` | admin |  |
| 72 | `GET /settings/models` | `provider.view` | admin, lead |  |
| 73 | `POST /settings/models` | `provider.manage` | admin |  |
| 74 | `GET /settings/models/{id}` | `provider.view` | admin, lead |  |
| 75 | `PATCH /settings/models/{id}` | `provider.manage` | admin |  |
| 76 | `GET /settings/models/{id}/pricing` | `provider.view` | admin, lead |  |
| 77 | `POST /settings/models/{id}/pricing` | `provider.manage` | admin |  |
| 78 | `GET /settings/prompt-templates` | `provider.view` | admin, lead |  |
| 79 | `GET /settings/prompt-templates/{id}` | `provider.view` | admin, lead |  |
| 80 | `POST /settings/prompt-templates` | `provider.view` | admin, lead | DECISIONS Agent 4: Lead/Admin (= provider.view holders) |
| 81 | `POST /settings/prompt-templates/{id}/activate` | `provider.view` | admin, lead |  |
| 82 | `POST /settings/prompt-templates/{id}/retire` | `provider.view` | admin, lead |  |
| 83 | `POST /cost-estimates` | `generate.run` | admin, lead, creator |  |
| 84 | `GET /usage/summary` | `usage.view` | admin, lead |  |
| 85 | `GET /settings/budgets` | `usage.view` | admin, lead |  |
| 86 | `PATCH /settings/budgets` | `budget.manage` | admin, lead |  |
| 87 | `GET /audit-logs` | `audit.view` | admin |  |
| 88 | `GET /notifications` | authenticated | all roles | own notifications |
| 89 | `POST /notifications/{id}/read` | authenticated | all roles | own notification |
| 90 | `GET /dashboard/summary` | authenticated | all roles | widgets filtered by permission |

Not under `/api/v1` (operational, unauthenticated by design, not exposed through nginx): `GET /healthz`, `GET /readyz`, `GET /metrics`.

## IDOR cases tested (`TestIDOR`)

A = content_creator (owner), B = second content_creator (no membership), R = reviewer. "404" means existence is not leaked. "403" is used where the resource is legitimately visible to `brand.view` (library, campaigns) but may not be changed.

| Resource | Attempt | Result |
|---|---|---|
| Content asset | B: PATCH, DELETE, new version, restore, submit-review | 403 |
| Content asset | B, R: download a draft | 403 (only approved; Lead/Admin may, verified) |
| Content asset | R: PATCH | 403 |
| Content asset | B: `POST /content/{id}/rewrites` | 403 |
| Campaign | B: PATCH, PUT plan, DELETE plan-proposal | 403 |
| Campaign | B: `POST /campaigns/{id}/plan-generations` | 403 (**was 202, fixed**) |
| Campaign | B: `POST /content/generations` in A's campaign | 403 (**was 202, fixed**) |
| Campaign | B: `POST /images/generations`, `POST /videos` in A's campaign | 403 |
| File (upload) | B, R: `GET /files/{id}`, `POST /files/{id}/complete` | 404 (admin: 200) |
| File (generated) | B: `GET /files/{id}` | 404 |
| File | B: A's image as `reference_file_ids` | 404 |
| File | B: A's image as video `source.file_ids` | 400 `source.file_ids[0]=not_found` |
| Job | B, R: GET, events (SSE), history, cancel, retry | 404 (lead via `provider.view`: 200) |
| Job | B: `GET /jobs` list | A's job is not listed |
| Image asset | B: edits, variations | 404 |
| Video project | B: PUT scenes, estimate, productions, renders, scene regeneration | 404 |

## Findings

| # | Severity | Finding | Status |
|---|---|---|---|
| 1 | High | `campaign.GetCampaignForGeneration` checked only `generate.run`. Any creator could start content, image or video generation in another creator's campaign and spend that campaign's budget. `plan-generations` also overwrote the owner's plan proposal. SPEC-01 §2: a creator works only in campaigns they created or are assigned to. | **Fixed.** Creator or member, or Lead/Admin, otherwise `403` with a Vietnamese message. The `campaigntest` fake mirrors the rule. Covered by a unit test and by `TestIDOR`. |
| 2 | Info | SPEC-04 lists `POST /settings/prompt-templates` under `provider.manage`. The code gates it with `provider.view` plus a Lead/Admin check, per the Agent 4 decision. The effective role set is the same: admin and lead. | Accepted (documented decision). |
| 3 | Info | `GET /images/generations` and `GET /images/references` require `generate.run`, so reviewers cannot open the image gallery; reviewers see images through `/assets` and `/reviews/queue`. SPEC-04 is silent. | Accepted. |

## Not covered

- Postgres-backed repositories are exercised by the module `adapter/postgres` tests, which skip without Docker. The IDOR harness uses the `<m>test` in-memory repositories, which the contract review checked against the SQL repositories.
- `GET /audit-logs`, `/notifications` and `/dashboard/summary` are covered by the RBAC matrix only. They are per-user or permission-filtered by construction and have no cross-user identifier to tamper with, except `POST /notifications/{id}/read`, which is scoped to the user in SQL (audit module).
