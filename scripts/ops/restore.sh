#!/usr/bin/env bash
set -euo pipefail

BACKUP_ARCHIVE="${1:-}"
DATABASE_DSN="${MEGAVPN_DATABASE_DSN:-${MEGAVPN_DATABASE_URL:-}}"
ARTIFACT_ROOT="${MEGAVPN_ARTIFACT_ROOT:-/var/lib/megavpn/artifacts}"
MASTER_KEY_PATH="${MEGAVPN_MASTER_KEY_PATH:-}"
CONFIRM="${MEGAVPN_RESTORE_CONFIRM:-0}"
RESTORE_ARTIFACTS="${MEGAVPN_RESTORE_ARTIFACTS:-1}"
RESTORE_MASTER_KEY="${MEGAVPN_RESTORE_MASTER_KEY:-0}"
OVERWRITE_MASTER_KEY="${MEGAVPN_RESTORE_OVERWRITE_MASTER_KEY:-0}"

log() {
  printf '[restore] %s\n' "$*"
}

die() {
  printf '[restore] ERROR: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

is_enabled() {
  case "${1,,}" in
    1|true|yes|y|on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

[[ -n "$BACKUP_ARCHIVE" ]] || die "backup archive path is required"
[[ -f "$BACKUP_ARCHIVE" ]] || die "backup archive does not exist: $BACKUP_ARCHIVE"
[[ -n "$DATABASE_DSN" ]] || die "MEGAVPN_DATABASE_DSN or MEGAVPN_DATABASE_URL is required"
is_enabled "$CONFIRM" || die "restore is destructive; set MEGAVPN_RESTORE_CONFIRM=1"

require_command pg_restore
require_command tar

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
tar -C "$workdir" -xzf "$BACKUP_ARCHIVE"

[[ -f "$workdir/db.dump" ]] || die "backup archive does not contain db.dump"

log "restore PostgreSQL database"
pg_restore --clean --if-exists --no-owner --dbname="$DATABASE_DSN" "$workdir/db.dump"

if is_enabled "$RESTORE_ARTIFACTS"; then
  if [[ -f "$workdir/artifacts.tar.gz" ]]; then
    timestamp="$(date -u +%Y%m%d-%H%M%S)"
    if [[ -e "$ARTIFACT_ROOT" ]]; then
      preserved="${ARTIFACT_ROOT}.pre-restore-${timestamp}"
      log "preserve existing artifact root at $preserved"
      mv "$ARTIFACT_ROOT" "$preserved"
    fi
    log "restore artifacts into $ARTIFACT_ROOT"
    install -d -m 0750 "$ARTIFACT_ROOT"
    tar -C "$ARTIFACT_ROOT" -xzf "$workdir/artifacts.tar.gz"
  else
    log "backup does not contain artifacts archive"
  fi
else
  log "skip artifacts by MEGAVPN_RESTORE_ARTIFACTS=$RESTORE_ARTIFACTS"
fi

if is_enabled "$RESTORE_MASTER_KEY"; then
  [[ -n "$MASTER_KEY_PATH" ]] || die "MEGAVPN_MASTER_KEY_PATH is required to restore master key"
  [[ -f "$workdir/master.key" ]] || die "backup does not contain master.key"
  if [[ -e "$MASTER_KEY_PATH" ]] && ! is_enabled "$OVERWRITE_MASTER_KEY"; then
    die "master key exists: $MASTER_KEY_PATH (set MEGAVPN_RESTORE_OVERWRITE_MASTER_KEY=1 to overwrite)"
  fi
  log "restore master key to $MASTER_KEY_PATH"
  install -d -m 0750 "$(dirname "$MASTER_KEY_PATH")"
  install -m 0600 "$workdir/master.key" "$MASTER_KEY_PATH"
else
  log "skip master key restore; restore or verify MEGAVPN_MASTER_KEY_PATH separately"
fi

log "restore completed"
