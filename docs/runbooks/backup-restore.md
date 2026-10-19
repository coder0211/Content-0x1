# Runbook — Backup & restore (ACS-103, NFR-08)

Targets: **RPO ≤ 24h** (daily dump; tighter with WAL archiving, P1), **RTO ≤ 2h** for the database, object storage recoverable via bucket versioning.

## 1. What is backed up

| Data | Mechanism | Retention |
|---|---|---|
| PostgreSQL (all app data, audit log) | `deploy/backup/pg-backup.sh`: `pg_dump -Fc` daily at 02:15 UTC, archive verified with `pg_restore --list`, SHA-256 sidecar, optional `age` encryption, upload to `BACKUP_S3_BUCKET/postgres/` | Local: `BACKUP_RETENTION_DAYS` (default 14). Remote: lifecycle rule below (35 days) |
| Asset files (`S3_BUCKET`) | Bucket **versioning enabled** (`minio-init`, or AWS console) — overwrites/deletes keep noncurrent versions | Noncurrent versions expire after 30 days; OQ-06 purges (archived assets 90d, pending uploads 24h) are app-driven |
| Redis | Not backed up: queue state is rebuilt from the DB outbox/jobs; AOF only protects restarts | — |
| `.env`, certs | Secret store (see secrets-rotation.md), not in backups | — |

### Install

```bash
sudo cp deploy/backup/crontab.example /etc/cron.d/acs-backup   # adjust user/paths
sudo mkdir -p /var/backups/acs && sudo chown deploy /var/backups/acs
```

Manual run: `cd /opt/acs/deploy/staging && docker compose --env-file .env --profile backup run --rm backup`.

### Bucket versioning & lifecycle

MinIO (`mc` alias `acs`):

```bash
mc version enable acs/acs-assets
mc version info acs/acs-assets                       # → "versioning is enabled"
mc ilm rule add acs/acs-assets --noncurrent-expire-days 30
mc ilm rule add acs/acs-backups --prefix postgres/ --expire-days 35
mc retention set --default GOVERNANCE 14d acs/acs-backups   # optional object lock (bucket created with --with-lock)
```

AWS S3: enable *Bucket Versioning*, add lifecycle rules "NoncurrentVersionExpiration 30d" (assets) and "Expiration 35d on prefix postgres/" (backups); block public access on both; the backup bucket lives in a different account/region in production if possible.

## 2. Restore the database

1. Pick the dump: `ls -lt /var/backups/acs` or `mc ls acs/acs-backups/postgres/` (`mc cp` it locally with its `.sha256`).
2. **Drill / inspection:** restore into a scratch instance, never into production:
   ```bash
   docker run -d --name acs-restore -e POSTGRES_PASSWORD=drill -p 55432:5432 postgres:16-alpine
   PGHOST=localhost PGPORT=55432 PGUSER=postgres PGPASSWORD=drill \
     deploy/backup/pg-restore.sh /var/backups/acs/acs-acs-<ts>.dump acs_restore
   ```
   The script checks the SHA-256, creates the DB, runs `pg_restore --exit-on-error`, and prints goose version + row counts.
3. **Disaster recovery (production):**
   1. Announce maintenance; `docker compose stop api worker render` (nginx keeps serving the SPA, API returns 502).
   2. Snapshot the broken DB volume if possible (`docker compose exec postgres pg_dumpall` or volume copy) for forensics.
   3. Restore into a fresh database `acs_new` with `pg-restore.sh`, verify counts, then swap: `ALTER DATABASE acs RENAME TO acs_broken; ALTER DATABASE acs_new RENAME TO acs;` (no connections must be open).
   4. `docker compose run --rm migrate` (no-op if schema is current) → `docker compose up -d`.
   5. Verify: `/healthz`, login, Library lists assets, a download URL works, `/audit-logs` shows recent rows.
   6. Jobs that were running after the dump time are lost: inform users; budget reservations are reconciled by the job reconciler.

## 3. Restore asset files

- Deleted/overwritten object: `mc ls --versions acs/acs-assets/<key>` → `mc cp --version-id <id> acs/acs-assets/<key> acs/acs-assets/<key>`.
- Point-in-time for a prefix: `mc cp --recursive --rewind 2026-10-18T00:00Z acs/acs-assets/ acs/acs-assets-restore/`.
- After a DB restore, files created after the dump are orphans (harmless; purge job ignores them) — note them in the drill record.

## 4. Restore drill (required before go-live, then quarterly)

Copy this template into the ops log (one per drill):

```markdown
### Restore drill — <YYYY-MM-DD>
- Operator: <name>            Witness: <name>
- Environment restored to: <scratch host / container>
- Dump used: <file name>  size: <MB>  created at: <UTC>  sha256 ok: yes/no
- Encrypted: yes/no (identity file location verified: yes/no)

| Step | Start (UTC) | End (UTC) | Result / notes |
|---|---|---|---|
| Fetch dump from S3 | | | |
| Verify checksum | | | |
| pg_restore | | | |
| Post-restore checks (goose version, users/assets/audit counts) | | | |
| App smoke against restored DB (login, library, download URL) | | | |
| Restore 1 deleted object from bucket versioning | | | |

- Goose version restored: <n>   (expected: <n>)
- Row counts: users <n> · content_assets <n> · audit_logs <n>
- **RPO measured** (now − dump time): <h>      target ≤ 24h
- **RTO measured** (start fetch → smoke ok): <min>   target ≤ 120 min
- Result: PASS / FAIL
- Issues found & follow-ups (owner, due date):
```
