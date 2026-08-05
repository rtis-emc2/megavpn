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
  case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
    1|true|yes|y|on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

require_safe_restore_target() {
  local label="$1"
  local value="$2"
  if [[ "$value" != /* ||
        "$value" == "/" ||
        "$value" == *"/./"* ||
        "$value" == */. ||
        "$value" == *"/../"* ||
        "$value" == */.. ]]; then
    die "$label must be a safe absolute path: $value"
  fi
  case "$value" in
    /bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/srv|/sys|/tmp|/usr|/var|/Applications|/Library|/System|/Users|/private)
      die "$label cannot be a protected system path: $value"
      ;;
  esac
}

validate_tar_archive() {
  local archive="$1"
  local label="$2"
  local listing="$3"
  local verbose_listing="$4"
  local entry normalized line type

  tar -tzf "$archive" >"$listing" || die "$label cannot be listed"
  while IFS= read -r entry || [[ -n "$entry" ]]; do
    normalized="$entry"
    while [[ "$normalized" == ./* ]]; do
      normalized="${normalized#./}"
    done
    if [[ "$normalized" == /* || "/$normalized/" == *"/../"* ]]; then
      die "$label contains an unsafe path: $entry"
    fi
  done <"$listing"

  tar -tvzf "$archive" >"$verbose_listing" || die "$label metadata cannot be listed"
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -n "$line" ]] || continue
    type="${line:0:1}"
    case "$type" in
      -|d)
        ;;
      *)
        die "$label contains a link or special file"
        ;;
    esac
  done <"$verbose_listing"
}

[[ -n "$BACKUP_ARCHIVE" ]] || die "backup archive path is required"
[[ -f "$BACKUP_ARCHIVE" ]] || die "backup archive does not exist: $BACKUP_ARCHIVE"
[[ -n "$DATABASE_DSN" ]] || die "MEGAVPN_DATABASE_DSN or MEGAVPN_DATABASE_URL is required"
is_enabled "$CONFIRM" || die "restore is destructive; set MEGAVPN_RESTORE_CONFIRM=1"
require_safe_restore_target "artifact root" "$ARTIFACT_ROOT"
if is_enabled "$RESTORE_MASTER_KEY"; then
  [[ -n "$MASTER_KEY_PATH" ]] || die "MEGAVPN_MASTER_KEY_PATH is required to restore master key"
  require_safe_restore_target "master key path" "$MASTER_KEY_PATH"
fi

require_command pg_restore
require_command tar

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
archive_copy="$workdir/input.tar.gz"
restore_root="$workdir/restore"
install -d -m 0700 "$restore_root"
install -m 0600 "$BACKUP_ARCHIVE" "$archive_copy"
validate_tar_archive "$archive_copy" "backup archive" "$workdir/backup.list" "$workdir/backup.verbose"
tar --no-same-owner --no-same-permissions -C "$restore_root" -xzf "$archive_copy"

[[ -f "$restore_root/db.dump" ]] || die "backup archive does not contain db.dump"
if is_enabled "$RESTORE_ARTIFACTS" && [[ -f "$restore_root/artifacts.tar.gz" ]]; then
  validate_tar_archive "$restore_root/artifacts.tar.gz" "artifact archive" "$workdir/artifacts.list" "$workdir/artifacts.verbose"
fi

log "restore PostgreSQL database"
pg_restore --clean --if-exists --no-owner --dbname="$DATABASE_DSN" "$restore_root/db.dump"

if is_enabled "$RESTORE_ARTIFACTS"; then
  if [[ -f "$restore_root/artifacts.tar.gz" ]]; then
    timestamp="$(date -u +%Y%m%d-%H%M%S)"
    if [[ -e "$ARTIFACT_ROOT" ]]; then
      preserved="${ARTIFACT_ROOT}.pre-restore-${timestamp}"
      log "preserve existing artifact root at $preserved"
      mv "$ARTIFACT_ROOT" "$preserved"
    fi
    log "restore artifacts into $ARTIFACT_ROOT"
    install -d -m 0750 "$ARTIFACT_ROOT"
    tar --no-same-owner --no-same-permissions -C "$ARTIFACT_ROOT" -xzf "$restore_root/artifacts.tar.gz"
  else
    log "backup does not contain artifacts archive"
  fi
else
  log "skip artifacts by MEGAVPN_RESTORE_ARTIFACTS=$RESTORE_ARTIFACTS"
fi

if is_enabled "$RESTORE_MASTER_KEY"; then
  [[ -f "$restore_root/master.key" ]] || die "backup does not contain master.key"
  if [[ -e "$MASTER_KEY_PATH" ]] && ! is_enabled "$OVERWRITE_MASTER_KEY"; then
    die "master key exists: $MASTER_KEY_PATH (set MEGAVPN_RESTORE_OVERWRITE_MASTER_KEY=1 to overwrite)"
  fi
  log "restore master key to $MASTER_KEY_PATH"
  install -d -m 0750 "$(dirname "$MASTER_KEY_PATH")"
  install -m 0600 "$restore_root/master.key" "$MASTER_KEY_PATH"
else
  log "skip master key restore; restore or verify MEGAVPN_MASTER_KEY_PATH separately"
fi

log "restore completed"
