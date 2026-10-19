#!/usr/bin/env bash
# Restore a pg-backup.sh dump into a TARGET database (ACS-103 drill / DR).
#
# Usage: pg-restore.sh <dump file (.dump or .dump.age)> <target database>
#
# Env (libpq): PGHOST PGPORT PGUSER PGPASSWORD — point them at the restore
# server (a scratch instance for drills, never production by accident).
# Env: AGE_IDENTITY_FILE  private key file when the dump is encrypted.
#      ALLOW_EXISTING=1   restore into an existing non-empty database (--clean).
set -euo pipefail

[[ $# -eq 2 ]] || { echo "usage: $0 <dump> <target-db>" >&2; exit 2; }
dump="$1"
target="$2"

log() { printf '%s pg-restore %s\n' "$(date -u +%FT%TZ)" "$*"; }

[[ -f "$dump" ]] || { log "no such file: $dump"; exit 1; }
if [[ -f "${dump}.sha256" ]]; then
  (cd "$(dirname "$dump")" && sha256sum -c "$(basename "$dump").sha256")
fi

work="$dump"
cleanup() { [[ "$work" != "$dump" ]] && rm -f "$work"; return 0; }
trap cleanup EXIT

if [[ "$dump" == *.age ]]; then
  : "${AGE_IDENTITY_FILE:?AGE_IDENTITY_FILE is required for encrypted dumps}"
  work="$(mktemp)"
  age --decrypt --identity "$AGE_IDENTITY_FILE" --output "$work" "$dump"
fi

start=$(date +%s)
if psql -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = '${target//\'/}'" | grep -q 1; then
  [[ "${ALLOW_EXISTING:-0}" == "1" ]] || { log "database $target exists; set ALLOW_EXISTING=1 to overwrite"; exit 1; }
  clean=(--clean --if-exists)
else
  log "creating database $target"
  createdb "$target"
  clean=()
fi

log "restoring $(basename "$dump") → $target"
pg_restore --no-owner --no-privileges --exit-on-error "${clean[@]}" --dbname="$target" "$work"

# Post-restore checks: migrations recorded, key tables readable.
psql -d "$target" -v ON_ERROR_STOP=1 -c "SELECT max(version_id) AS goose_version FROM goose_db_version;" \
  -c "SELECT (SELECT count(*) FROM users) AS users, (SELECT count(*) FROM content_assets) AS assets, (SELECT count(*) FROM audit_logs) AS audit_rows;"

log "restore finished in $(( $(date +%s) - start ))s"
