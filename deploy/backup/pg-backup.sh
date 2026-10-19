#!/usr/bin/env bash
# Daily PostgreSQL logical backup (ACS-103, SPEC-02 §7).
#
# Writes $BACKUP_DIR/acs-<db>-<UTC timestamp>.dump (pg_dump custom format,
# compressed), optionally encrypts it with age, optionally uploads it to an
# S3 bucket (versioned) and prunes local dumps older than
# $BACKUP_RETENTION_DAYS. Remote retention is enforced by the bucket's
# lifecycle rule (see docs/runbooks/backup-restore.md).
#
# Env (libpq): PGHOST PGPORT PGUSER PGPASSWORD PGDATABASE
# Env (script):
#   BACKUP_DIR              default /var/backups/acs
#   BACKUP_RETENTION_DAYS   default 14
#   BACKUP_AGE_RECIPIENT    optional age public key → .dump.age
#   BACKUP_S3_BUCKET        optional; enables upload with mc/mcli
#   BACKUP_S3_ALIAS_URL     e.g. http://minio:9000 or https://s3.amazonaws.com
#   BACKUP_S3_ACCESS_KEY / BACKUP_S3_SECRET_KEY
#
# Exit code != 0 on any failure, so cron/monitoring can alert.
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/var/backups/acs}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
DB="${PGDATABASE:?PGDATABASE is required}"
TS="$(date -u +%Y%m%dT%H%M%SZ)"

log() { printf '%s pg-backup %s\n' "$(date -u +%FT%TZ)" "$*"; }

mkdir -p "$BACKUP_DIR"
umask 077

out="$BACKUP_DIR/acs-${DB}-${TS}.dump"
tmp="${out}.partial"
trap 'rm -f "$tmp"' EXIT

log "dumping database ${DB} → ${out}"
pg_dump --format=custom --compress=6 --no-owner --no-privileges --file="$tmp" "$DB"
# Sanity check: the archive must list its table of contents.
pg_restore --list "$tmp" >/dev/null
mv "$tmp" "$out"

if [[ -n "${BACKUP_AGE_RECIPIENT:-}" ]]; then
  command -v age >/dev/null || { log "age not installed but BACKUP_AGE_RECIPIENT set"; exit 1; }
  age --recipient "$BACKUP_AGE_RECIPIENT" --output "${out}.age" "$out"
  rm -f "$out"
  out="${out}.age"
fi

sha256sum "$out" | tee "${out}.sha256" >/dev/null
log "dump ok ($(du -h "$out" | cut -f1))"

if [[ -n "${BACKUP_S3_BUCKET:-}" ]]; then
  MC="$(command -v mc || command -v mcli || true)"
  [[ -n "$MC" ]] || { log "mc/mcli not installed but BACKUP_S3_BUCKET set"; exit 1; }
  : "${BACKUP_S3_ALIAS_URL:?BACKUP_S3_ALIAS_URL is required for upload}"
  "$MC" alias set acsbackup "$BACKUP_S3_ALIAS_URL" "${BACKUP_S3_ACCESS_KEY:?}" "${BACKUP_S3_SECRET_KEY:?}" >/dev/null
  "$MC" cp --quiet "$out" "${out}.sha256" "acsbackup/${BACKUP_S3_BUCKET}/postgres/"
  log "uploaded to ${BACKUP_S3_BUCKET}/postgres/$(basename "$out")"
fi

deleted="$(find "$BACKUP_DIR" -maxdepth 1 -type f -name "acs-${DB}-*" -mtime +"$RETENTION_DAYS" -print -delete | wc -l)"
log "pruned ${deleted// /} local file(s) older than ${RETENTION_DAYS}d"
log "done"
