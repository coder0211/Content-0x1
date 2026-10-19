# Go-live checklist — production (ACS-107)

Owner: <ops lead> · Date: <YYYY-MM-DD> · Release tag/commit: <sha>
Tick every box; anything unticked needs a written waiver from the PO.

## 1. Infrastructure (separate from staging)

- [ ] Production VM provisioned (OQ-05), OS patched, firewall: only 80/443 public, SSH restricted to admin IPs/VPN.
- [ ] **Separate Postgres** instance/volume and credentials (no shared DB with staging).
- [ ] **Separate bucket** (`acs-assets-prod`) and backup bucket, private (`anonymous none` / Block Public Access), **versioning enabled**, lifecycle rules applied (backup-restore.md §1).
- [ ] `/opt/acs/deploy/production/.env` rendered from the secret manager, mode 600: `APP_ENV=production`, `COOKIE_SECURE=true`, `ENABLE_FAKE_PROVIDERS=false`, non-default S3 keys (the API refuses to start otherwise), `SIGNED_URL_TTL ≤ 15m`, correct `APP_BASE_URL`/`CORS_ORIGINS`.
- [ ] TLS certificate covers `APP_HOST` and `S3_HOST`; auto-renewal tested (`certbot renew --dry-run`).
- [ ] Compose file copied from `deploy/staging/` with production hosts; image tags pinned to the release sha (no `latest`); MinIO/mc images pinned by digest.
- [ ] Container resource limits set (render worker CPU/RAM), log rotation on.

## 2. Data & seed

- [ ] `acs migrate up` ran successfully (`acs migrate status` shows all applied).
- [ ] Roles + RBAC matrix present (migration 0011): `SELECT role_code, count(*) FROM role_permissions GROUP BY 1;`.
- [ ] First Admin created: `ACS_ADMIN_PASSWORD=… acs admin create --email <admin> --name <name>`; demo seed (`acs seed demo`) **not** run.
- [ ] Pilot users created via Settings › Users with correct roles; temp passwords sent out-of-band.
- [ ] Providers configured (OQ-01): Anthropic text, OpenAI image, Google Veo video + `kenburns` fallback, Google TTS; `secret_ref` names set, keys present in `.env`; `fake` provider **disabled**.
- [ ] Model defaults and **pricing versions** entered for every enabled model (Settings › Models › Pricing) and cross-checked against vendor price pages.
- [ ] Budget (OQ-03): global **$1,000/month**, alerts **80% and 100%**, **hard_stop = true**; per-job limits text **$0.50**, image **$2**, video **$8**; budget alert e-mail recipients (Leads) verified.
- [ ] Prometheus `acs:budget_monthly_limit_usd` equals the DB limit (1000).
- [ ] Brand profiles / products of the pilot brands entered (or scheduled with the PO).

## 3. Backups & recovery

- [ ] Backup cron installed; first backup present locally **and** in the backup bucket; freshness check active.
- [ ] Restore drill executed and recorded (backup-restore.md §4) with RPO/RTO within targets.
- [ ] Bucket versioning restore of a single object tested.

## 4. Observability & alerting

- [ ] Prometheus scrapes `api:8080/metrics` (`up{job="acs-api"} == 1`); `/metrics` returns 404 publicly.
- [ ] Grafana "ACS — Overview" dashboard loads with data; reachable only via SSH tunnel/VPN.
- [ ] Alert rules loaded (`AIJobFailedRateHigh`, `OutboxLagHigh`, `VideoQueueBacklog`, `BudgetSpend80Percent/100Percent`, `APIDown`, `APIHighLatencyP95`) and routed to the on-call channel; one test alert fired and received.
- [ ] Log retention configured; secrets/signed-URL leak scan clean (secrets-rotation.md §4).

## 5. Security

- [ ] Security test pass (ACS-100) green on the release commit: IDOR, RBAC matrix, CSRF, upload sniffing, SSRF.
- [ ] Self-approval blocked (Creator cannot approve own asset); Creator cannot download non-approved assets (403).
- [ ] Audit log append-only verified (UPDATE on `audit_logs` fails); downloads appear in audit log.
- [ ] Provider keys rotated from any value used during testing.

## 6. Smoke test on production (after deploy, before announcing)

- [ ] `GET /healthz` 200, `/readyz` ok from inside the network.
- [ ] Admin login/logout; lockout after 5 wrong passwords (use a test account, then unlock).
- [ ] Create brand → campaign → generate 1 content (real provider) → edit → submit review → approve by a reviewer → download (signed URL expires ≤ 15 min).
- [ ] Generate 1 image; generate a short video (or kenburns) end-to-end; SSE progress updates live (no buffering).
- [ ] Usage page shows the costs; dashboard widgets match the user's permissions.

## 7. Rollback plan

- Application: set `IMAGE_TAG` in `.env` to the value in `.previous_image_tag` → `docker compose pull && docker compose up -d`. Goose migrations are forward-only in production: if a release migration must be reverted, restore the pre-deploy backup instead of `migrate down` unless the migration's Down was reviewed.
- Take an on-demand backup **immediately before** the go-live deploy (`--profile backup run --rm backup`).
- Provider incident: disable the provider in Settings (video falls back to `kenburns`).
- Spend runaway: Admin sets the global budget hard stop / lowers the limit; workers stop reserving new jobs.
- Decision makers & contacts: <PO>, <tech lead>, <ops on-call> — phone/Slack listed in the ops log.

## 8. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Product Owner | | | |
| Tech Lead | | | |
| Ops | | | |
