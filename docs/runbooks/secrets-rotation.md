# Runbook — Secrets & key rotation (ACS-102)

Principles (SPEC-02 §7): secrets live only in the managed env file on the VM (`/opt/acs/deploy/<env>/.env`, mode 600, owner `deploy`) sourced from the team secret manager (vault/1Password/cloud secret manager — the secret manager is the source of truth, the `.env` is a rendered copy). The DB stores only **names** (`ai_providers.secret_ref`); the API reads values via `config.Secret(secret_ref)`, never returns them to the FE and never logs them.

## 1. Inventory

| Secret | Where used | Rotation cadence | Blast radius if leaked |
|---|---|---|---|
| `PROVIDER_<CODE>_API_KEY` (e.g. `PROVIDER_ANTHROPIC_API_KEY`, `PROVIDER_OPENAI_API_KEY`, `PROVIDER_GOOGLE_API_KEY`) | api + workers, by `secret_ref` | 90 days, or immediately on suspicion | Spend on our account |
| `POSTGRES_PASSWORD` / `DATABASE_URL` | api, workers, migrate, backup | 180 days | Full data access |
| `REDIS_PASSWORD` / `REDIS_URL` | api, workers | 180 days | Queue tampering |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY`, `MINIO_ROOT_*` | api, workers, minio-init | 180 days | Asset read/write |
| `BACKUP_S3_*`, age identity (offline, not on VM) | backup | 365 days | Backup read |
| SMTP credentials | api (budget alerts) | 365 days | Mail spoofing |
| `GRAFANA_ADMIN_PASSWORD` | grafana | 180 days | Metrics view |
| CI: `STAGING_SSH_KEY`, `STAGING_GHCR_TOKEN` | GitHub Actions | 180 days | Deploy access |
| TLS private key (`certs/privkey.pem`) | nginx | with certificate renewal | MITM |

Sessions are random tokens stored hashed (no signing secret to rotate); to force everyone to log in again: `DELETE FROM sessions;` (audited by running it through an admin change ticket).

## 2. Rotate a provider API key (zero downtime)

1. Create the new key in the vendor console (keep the old one active).
2. Update the secret manager entry; render the new value into `.env` (`PROVIDER_<CODE>_API_KEY=…`).
   *Alternative without overwriting:* add `PROVIDER_<CODE>_API_KEY_V2`, then Admin › Settings › Providers › set `secret_ref` to the new name (audited, `PATCH /settings/providers/{id}`).
3. `docker compose --env-file .env up -d api worker render` (recreates containers with the new env; running jobs retry transient failures).
4. Verify: run one cheap generation per affected capability; `ai_provider_errors_total{provider="<code>",kind="Auth"}` stays flat.
5. Revoke the old key in the vendor console. Record date + operator in the ops log.

**Compromised key:** revoke at the vendor **first** (accept failing jobs), then steps 2–4; check vendor usage dashboard and `/usage/summary?group_by=provider` for unexpected spend; set the provider to `disabled` in Settings if no new key is available yet.

## 3. Rotate database / Redis / S3 credentials

- **Postgres:** `ALTER ROLE acs WITH PASSWORD '<new>';` → update `POSTGRES_PASSWORD` + `DATABASE_URL` in `.env` → `docker compose up -d api worker render` (the backup job reads `.env` on its next run). Note `POSTGRES_PASSWORD` only initialises a fresh volume; changing it in `.env` alone does nothing.
- **Redis:** set the new password in `.env` (`REDIS_PASSWORD`, `REDIS_URL`) → `docker compose up -d redis api worker render` (short queue pause; asynq retries).
- **S3/MinIO:** `mc admin user add acs <new-access> <new-secret>` + `mc admin policy attach acs readwrite --user <new-access>` → update `.env` → recreate api/workers → verify upload + download → `mc admin user remove acs <old-access>`. AWS: create a second access key on the IAM user, switch, then deactivate/delete the old key.
- **CI/SSH:** generate a new deploy key, append the public key to `~deploy/.ssh/authorized_keys`, update `STAGING_SSH_KEY`, run the workflow manually, remove the old public key.

## 4. Verify nothing leaks (run after every rotation and before go-live)

```bash
cd /opt/acs/deploy/staging
# 1) No provider key / password value appears in container logs (use the real values from .env).
for v in $(grep -E '^(PROVIDER_.*_API_KEY|POSTGRES_PASSWORD|REDIS_PASSWORD|S3_SECRET_KEY)=' .env | cut -d= -f2-); do
  [ -n "$v" ] && docker compose logs --no-color --since 168h api worker render nginx 2>&1 | grep -F -q -- "$v" && echo "LEAK: value of a secret found in logs"
done; echo "secret value scan done"
# 2) No signed URLs, cookies or auth headers in logs.
docker compose logs --no-color --since 168h api worker render nginx 2>&1 \
  | grep -E -i 'X-Amz-Signature|X-Amz-Credential|acs_session=|authorization: bearer|api_key"?[:=] *"?[A-Za-z0-9_-]{12,}' \
  && echo "LEAK: signed URL / credential pattern found" || echo "pattern scan clean"
# 3) nginx access logs use the redacted format (path only, no query string).
docker compose exec nginx sh -c 'grep -c "X-Amz-" /var/log/nginx/*.log || true'
# 4) API never returns secrets.
curl -s -b cookies.txt https://$APP_HOST/api/v1/settings/providers | grep -E -i 'api_key|sk-|secret"' && echo "LEAK in API" || echo "providers API clean"
# 5) Nothing secret committed.
git -C /path/to/repo log -p | grep -E 'PROVIDER_[A-Z_]+_API_KEY=[^C]' && echo "LEAK in git history" || echo "git clean"
```

Any hit → treat the secret as compromised (section 2 "Compromised key") and file an incident.
