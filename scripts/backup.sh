#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="${MEGAVPN_BACKUP_DIR:-/var/backups/megavpn}"
DATABASE_DSN="${MEGAVPN_DATABASE_DSN:-${MEGAVPN_DATABASE_URL:-}}"
ARTIFACT_ROOT="${MEGAVPN_ARTIFACT_ROOT:-/var/lib/megavpn/artifacts}"
MASTER_KEY_PATH="${MEGAVPN_MASTER_KEY_PATH:-}"
CONFIG_DIR="${MEGAVPN_CONFIG_DIR:-/etc/megavpn}"
INCLUDE_MASTER_KEY="${MEGAVPN_BACKUP_INCLUDE_MASTER_KEY:-0}"
INCLUDE_CONFIG="${MEGAVPN_BACKUP_INCLUDE_CONFIG:-0}"

log() {
  printf '[backup] %s\n' "$*"
}

die() {
  printf '[backup] ERROR: %s\n' "$*" >&2
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

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
    return 0
  fi
  printf 'unavailable'
}

[[ -n "$DATABASE_DSN" ]] || die "MEGAVPN_DATABASE_DSN or MEGAVPN_DATABASE_URL is required"

require_command pg_dump
require_command tar

timestamp="$(date -u +%Y%m%d-%H%M%S)"
hostname_value="$(hostname 2>/dev/null || printf 'unknown')"
backup_name="megavpn-backup-${timestamp}.tar.gz"
mkdir -p "$BACKUP_DIR"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

umask 0077

log "dump PostgreSQL database"
pg_dump --format=custom --file="$workdir/db.dump" "$DATABASE_DSN"

artifacts_archive=""
if [[ -d "$ARTIFACT_ROOT" ]]; then
  log "archive artifacts from $ARTIFACT_ROOT"
  artifacts_archive="artifacts.tar.gz"
  tar -C "$ARTIFACT_ROOT" -czf "$workdir/$artifacts_archive" .
else
  log "artifact root does not exist, skip artifacts: $ARTIFACT_ROOT"
fi

master_key_sha256=""
master_key_included="0"
if [[ -n "$MASTER_KEY_PATH" && -f "$MASTER_KEY_PATH" ]]; then
  master_key_sha256="$(sha256_file "$MASTER_KEY_PATH")"
  if is_enabled "$INCLUDE_MASTER_KEY"; then
    log "include master key material because MEGAVPN_BACKUP_INCLUDE_MASTER_KEY is enabled"
    install -m 0600 "$MASTER_KEY_PATH" "$workdir/master.key"
    master_key_included="1"
  else
    log "master key material is not included; store it separately and offline"
  fi
fi

config_included="0"
if is_enabled "$INCLUDE_CONFIG"; then
  if [[ -d "$CONFIG_DIR" ]]; then
    log "include config directory because MEGAVPN_BACKUP_INCLUDE_CONFIG is enabled"
    tar -C "$CONFIG_DIR" -czf "$workdir/config.tar.gz" .
    config_included="1"
  else
    log "config directory does not exist, skip config: $CONFIG_DIR"
  fi
fi

cat >"$workdir/manifest.env" <<EOF
created_at_utc=$timestamp
hostname=$hostname_value
database_dump=db.dump
artifact_root=$ARTIFACT_ROOT
artifacts_archive=$artifacts_archive
master_key_path=$MASTER_KEY_PATH
master_key_sha256=$master_key_sha256
master_key_included=$master_key_included
config_dir=$CONFIG_DIR
config_included=$config_included
EOF

archive_path="$BACKUP_DIR/$backup_name"
tar -C "$workdir" -czf "$archive_path" .
chmod 0600 "$archive_path"

log "backup created: $archive_path"
log "restore with: MEGAVPN_RESTORE_CONFIRM=1 scripts/restore.sh $archive_path"
