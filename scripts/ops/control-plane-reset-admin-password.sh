#!/usr/bin/env bash
set -euo pipefail

export PATH="/snap/bin:/usr/local/go/bin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:${PATH:-}"

APP_DIR="${MEGAVPN_CP_APP_DIR:-/opt/megavpn}"
ENV_FILE="${MEGAVPN_CP_ENV_FILE:-/etc/megavpn/megavpn.env}"
ADMIN_LOGIN="${MEGAVPN_ADMIN_LOGIN:-${MEGAVPN_CP_ADMIN_USERNAME:-superadmin}}"
ADMIN_PASSWORD_FILE="${MEGAVPN_ADMIN_PASSWORD_FILE:-/root/megavpn-control-plane-admin.txt}"
ADMIN_ACTIVATE="${MEGAVPN_ADMIN_ACTIVATE:-1}"
WRITE_PASSWORD_FILE="${MEGAVPN_ADMIN_WRITE_PASSWORD_FILE:-auto}"
ADMIN_BIN="${MEGAVPN_ADMIN_BIN:-$APP_DIR/bin/megavpn-admin}"

log() {
  printf '[control-plane-reset-admin-password] %s\n' "$*"
}

die() {
  printf '[control-plane-reset-admin-password] ERROR: %s\n' "$*" >&2
  exit 1
}

is_true() {
  case "${1,,}" in
    1|true|yes|y|on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

generate_password() {
  command -v openssl >/dev/null 2>&1 || die "openssl is required to generate a password"
  openssl rand -base64 36 | tr -d '\n'
}

load_env_file() {
  [[ -f "$ENV_FILE" ]] || die "env file not found: $ENV_FILE"
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
}

ensure_admin_binary() {
  if [[ -x "$ADMIN_BIN" ]]; then
    return 0
  fi
  [[ -d "$APP_DIR" ]] || die "app dir not found: $APP_DIR"
  command -v go >/dev/null 2>&1 || die "megavpn-admin is missing and go is not available; rerun scripts/control-plane-install.sh first"
  log "build megavpn-admin"
  (cd "$APP_DIR" && mkdir -p bin && go build -o "$ADMIN_BIN" ./cmd/admin)
}

write_password_file() {
  local password="$1"
  install -d -m 0700 "$(dirname "$ADMIN_PASSWORD_FILE")"
  local old_umask
  old_umask="$(umask)"
  umask 0077
  {
    printf 'RTIS MegaVPN Control Plane bootstrap admin\n'
    printf 'username=%s\n' "$ADMIN_LOGIN"
    printf 'password=%s\n' "$password"
    printf 'reset_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } >"$ADMIN_PASSWORD_FILE"
  umask "$old_umask"
  chmod 0600 "$ADMIN_PASSWORD_FILE"
}

main() {
  load_env_file
  ensure_admin_binary

  local generated_password="0"
  local password="${MEGAVPN_ADMIN_PASSWORD:-}"
  if [[ -z "$password" ]]; then
    password="$(generate_password)"
    generated_password="1"
  fi

  export MEGAVPN_ADMIN_PASSWORD="$password"
  local flags=(reset-password --login "$ADMIN_LOGIN")
  if is_true "$ADMIN_ACTIVATE"; then
    flags+=(--activate)
  fi

  "$ADMIN_BIN" "${flags[@]}"

  if [[ "$WRITE_PASSWORD_FILE" == "auto" && "$generated_password" == "1" ]] || is_true "$WRITE_PASSWORD_FILE"; then
    write_password_file "$password"
    log "admin credentials written: $ADMIN_PASSWORD_FILE"
  fi
}

main "$@"
