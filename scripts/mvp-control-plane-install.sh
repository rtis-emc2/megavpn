#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${MEGAVPN_MVP_APP_DIR:-/opt/megavpn}"
ENV_FILE="${MEGAVPN_MVP_ENV_FILE:-/etc/megavpn/megavpn.env}"
MASTER_KEY_PATH="${MEGAVPN_MVP_MASTER_KEY_PATH:-/etc/megavpn/master.key}"
MASTER_KEY_VERSION="${MEGAVPN_MVP_MASTER_KEY_VERSION:-v1}"
ARTIFACT_ROOT="${MEGAVPN_MVP_ARTIFACT_ROOT:-/var/lib/megavpn/artifacts}"
API_LISTEN_ADDR="${MEGAVPN_MVP_API_LISTEN_ADDR:-0.0.0.0:8080}"
WEB_ROOT="${MEGAVPN_MVP_WEB_ROOT:-$APP_DIR/web}"
LOG_LEVEL="${MEGAVPN_MVP_LOG_LEVEL:-info}"
ADMIN_USERNAME="${MEGAVPN_MVP_ADMIN_USERNAME:-${MEGAVPN_BOOTSTRAP_ADMIN_USERNAME:-superadmin}}"
ADMIN_EMAIL="${MEGAVPN_MVP_ADMIN_EMAIL:-${MEGAVPN_BOOTSTRAP_ADMIN_EMAIL:-superadmin@rtis.local}}"
ADMIN_DISPLAY_NAME="${MEGAVPN_MVP_ADMIN_DISPLAY_NAME:-${MEGAVPN_BOOTSTRAP_ADMIN_DISPLAY_NAME:-Superadmin}}"
DATABASE_DSN="${MEGAVPN_MVP_DATABASE_DSN:-${MEGAVPN_DATABASE_DSN:-}}"
PUBLIC_BASE_URL="${MEGAVPN_MVP_PUBLIC_BASE_URL:-${MEGAVPN_PUBLIC_BASE_URL:-}}"
TRUST_PROXY_HEADERS="${MEGAVPN_MVP_TRUST_PROXY_HEADERS:-${MEGAVPN_TRUST_PROXY_HEADERS:-false}}"
HEALTH_URL="${MEGAVPN_MVP_HEALTH_URL:-http://127.0.0.1:8080/healthz}"
ALLOW_LOOPBACK="${MEGAVPN_MVP_ALLOW_LOOPBACK:-0}"
RUN_TESTS="${MEGAVPN_MVP_RUN_TESTS:-0}"
ADMIN_PASSWORD_FILE="${MEGAVPN_MVP_ADMIN_PASSWORD_FILE:-/root/megavpn-mvp-admin.txt}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

log() {
  printf '[mvp-install] %s\n' "$*"
}

die() {
  printf '[mvp-install] ERROR: %s\n' "$*" >&2
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

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

detect_public_base_url() {
  local addresses address hostname_value
  addresses="$(hostname -I 2>/dev/null || true)"
  for address in $addresses; do
    case "$address" in
      127.*|::1)
        continue
        ;;
      *)
        printf 'http://%s:8080\n' "$address"
        return 0
        ;;
    esac
  done
  hostname_value="$(hostname -f 2>/dev/null || hostname)"
  [[ -n "$hostname_value" ]] || return 1
  printf 'http://%s:8080\n' "$hostname_value"
}

is_loopback_url() {
  case "$1" in
    http://localhost*|https://localhost*|http://127.*|https://127.*|http://[[]::1[]]*|https://[[]::1[]]*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

env_quote() {
  local value="${1//\'/\'\\\'\'}"
  printf "'%s'" "$value"
}

write_env_line() {
  local file="$1"
  local key="$2"
  local value="$3"
  printf '%s=%s\n' "$key" "$(env_quote "$value")" >>"$file"
}

generate_secret() {
  openssl rand -base64 32 | tr -d '\n'
}

sync_source_tree() {
  if [[ "$SRC_DIR" == "$APP_DIR" ]]; then
    log "source tree already in $APP_DIR"
    return 0
  fi
  log "sync source tree to $APP_DIR"
  install -d -m 0755 "$APP_DIR"
  rsync -a --exclude '.git' --exclude 'bin' "$SRC_DIR"/ "$APP_DIR"/
}

write_runtime_env() {
  local tmp backup admin_password old_umask
  admin_password="${MEGAVPN_MVP_ADMIN_PASSWORD:-${MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD:-}}"
  if [[ -z "$admin_password" ]]; then
    admin_password="$(generate_secret)"
    install -d -m 0700 "$(dirname "$ADMIN_PASSWORD_FILE")"
    old_umask="$(umask)"
    umask 0077
    {
      printf 'RTIS MegaVPN MVP bootstrap admin\n'
      printf 'username=%s\n' "$ADMIN_USERNAME"
      printf 'password=%s\n' "$admin_password"
      printf 'created_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    } >"$ADMIN_PASSWORD_FILE"
    umask "$old_umask"
    log "generated bootstrap admin password: $ADMIN_PASSWORD_FILE"
  fi

  install -d -m 0750 "$(dirname "$ENV_FILE")"
  if [[ -f "$ENV_FILE" ]]; then
    backup="${ENV_FILE}.bak.$(date -u +%Y%m%d%H%M%S)"
    cp -p "$ENV_FILE" "$backup"
    log "existing env file backed up to $backup"
  fi

  tmp="${ENV_FILE}.tmp.$$"
  trap 'rm -f "$tmp"' RETURN
  {
    printf '# Managed by scripts/mvp-control-plane-install.sh\n'
    printf '# Update through MEGAVPN_MVP_* variables and rerun the installer.\n'
  } >"$tmp"
  write_env_line "$tmp" "MEGAVPN_DATABASE_DSN" "$DATABASE_DSN"
  write_env_line "$tmp" "MEGAVPN_API_LISTEN_ADDR" "$API_LISTEN_ADDR"
  write_env_line "$tmp" "MEGAVPN_PUBLIC_BASE_URL" "$PUBLIC_BASE_URL"
  write_env_line "$tmp" "MEGAVPN_WEB_ROOT" "$WEB_ROOT"
  write_env_line "$tmp" "MEGAVPN_ARTIFACT_ROOT" "$ARTIFACT_ROOT"
  write_env_line "$tmp" "MEGAVPN_MASTER_KEY_PATH" "$MASTER_KEY_PATH"
  write_env_line "$tmp" "MEGAVPN_MASTER_KEY_VERSION" "$MASTER_KEY_VERSION"
  write_env_line "$tmp" "MEGAVPN_TRUST_PROXY_HEADERS" "$TRUST_PROXY_HEADERS"
  write_env_line "$tmp" "MEGAVPN_AGENT_ALLOW_AUTO_REGISTER" "false"
  write_env_line "$tmp" "MEGAVPN_AGENT_SIGNATURE_ENFORCE" "false"
  write_env_line "$tmp" "MEGAVPN_AGENT_SIGNATURE_WINDOW" "5m"
  write_env_line "$tmp" "MEGAVPN_BOOTSTRAP_ADMIN_USERNAME" "$ADMIN_USERNAME"
  write_env_line "$tmp" "MEGAVPN_BOOTSTRAP_ADMIN_EMAIL" "$ADMIN_EMAIL"
  write_env_line "$tmp" "MEGAVPN_BOOTSTRAP_ADMIN_DISPLAY_NAME" "$ADMIN_DISPLAY_NAME"
  write_env_line "$tmp" "MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD" "$admin_password"
  write_env_line "$tmp" "MEGAVPN_LOG_LEVEL" "$LOG_LEVEL"
  install -m 0600 "$tmp" "$ENV_FILE"
  rm -f "$tmp"
  trap - RETURN
}

ensure_master_key() {
  local old_umask
  if [[ -f "$MASTER_KEY_PATH" ]]; then
    log "master key exists: $MASTER_KEY_PATH"
    return 0
  fi
  log "generate master key: $MASTER_KEY_PATH"
  install -d -m 0750 "$(dirname "$MASTER_KEY_PATH")"
  old_umask="$(umask)"
  umask 0077
  openssl rand -hex 32 >"$MASTER_KEY_PATH"
  umask "$old_umask"
  chmod 0600 "$MASTER_KEY_PATH"
}

install_systemd_units() {
  install -m 0644 deploy/systemd/megavpn-migrate.service /etc/systemd/system/megavpn-migrate.service
  install -m 0644 deploy/systemd/megavpn-api.service /etc/systemd/system/megavpn-api.service
  install -m 0644 deploy/systemd/megavpn-worker.service /etc/systemd/system/megavpn-worker.service
  install -m 0644 deploy/systemd/megavpn-agent.service /etc/systemd/system/megavpn-agent.service
  install -d -m 0755 /etc/systemd/system/megavpn-migrate.service.d
  install -d -m 0755 /etc/systemd/system/megavpn-api.service.d
  install -d -m 0755 /etc/systemd/system/megavpn-worker.service.d
  install -d -m 0755 /etc/systemd/system/megavpn-agent.service.d
  cat >/etc/systemd/system/megavpn-migrate.service.d/10-mvp-paths.conf <<EOF
[Service]
WorkingDirectory=$APP_DIR
Environment=MEGAVPN_MIGRATIONS_DIR=$APP_DIR/migrations
ExecStart=
ExecStart=$APP_DIR/bin/megavpn-migrate
EOF
  cat >/etc/systemd/system/megavpn-api.service.d/10-mvp-paths.conf <<EOF
[Service]
WorkingDirectory=$APP_DIR
ExecStart=
ExecStart=$APP_DIR/bin/megavpn-api
EOF
  cat >/etc/systemd/system/megavpn-worker.service.d/10-mvp-paths.conf <<EOF
[Service]
WorkingDirectory=$APP_DIR
ExecStart=
ExecStart=$APP_DIR/bin/megavpn-worker
EOF
  cat >/etc/systemd/system/megavpn-agent.service.d/10-mvp-paths.conf <<EOF
[Service]
WorkingDirectory=$APP_DIR
ExecStart=
ExecStart=$APP_DIR/bin/megavpn-agent
EOF
  systemctl daemon-reload
}

run_migrations() {
  local result
  systemctl reset-failed megavpn-migrate.service >/dev/null 2>&1 || true
  systemctl start megavpn-migrate.service
  result="$(systemctl show -p Result --value megavpn-migrate.service 2>/dev/null || true)"
  [[ "$result" == "success" ]] || die "migrations failed; inspect: journalctl -u megavpn-migrate.service -n 120 --no-pager"
}

start_services() {
  systemctl enable --now megavpn-api.service megavpn-worker.service
  systemctl restart megavpn-api.service megavpn-worker.service
}

health_check() {
  local attempt
  for attempt in {1..20}; do
    if curl -fsS "$HEALTH_URL" >/dev/null; then
      curl -fsS "$HEALTH_URL"
      printf '\n'
      return 0
    fi
    sleep 1
  done
  systemctl --no-pager --full status megavpn-api.service megavpn-worker.service || true
  die "health check failed: $HEALTH_URL"
}

require_command go
require_command openssl
require_command rsync
require_command systemctl
require_command curl

[[ "${EUID:-$(id -u)}" -eq 0 ]] || die "run as root"
[[ -n "$DATABASE_DSN" ]] || die "set MEGAVPN_MVP_DATABASE_DSN=postgres://user:password@host:5432/megavpn?sslmode=disable"
[[ "$APP_DIR" != *[[:space:]]* ]] || die "MEGAVPN_MVP_APP_DIR must not contain whitespace because it is written into systemd ExecStart"

if [[ -z "$PUBLIC_BASE_URL" ]]; then
  PUBLIC_BASE_URL="$(detect_public_base_url)" || die "set MEGAVPN_MVP_PUBLIC_BASE_URL=https://control.example.com"
  log "MEGAVPN_MVP_PUBLIC_BASE_URL not set; detected $PUBLIC_BASE_URL"
fi
if is_loopback_url "$PUBLIC_BASE_URL" && ! is_true "$ALLOW_LOOPBACK"; then
  die "public base URL is loopback: $PUBLIC_BASE_URL; remote nodes cannot enroll. Set MEGAVPN_MVP_PUBLIC_BASE_URL or MEGAVPN_MVP_ALLOW_LOOPBACK=1 for lab only"
fi

sync_source_tree
cd "$APP_DIR"

install -d -m 0750 "$ARTIFACT_ROOT"
install -d -m 0750 /etc/megavpn/control-plane-tls
ensure_master_key
write_runtime_env

log "download Go modules"
go mod download

if is_true "$RUN_TESTS"; then
  log "run tests"
  go test ./...
else
  log "skip tests; set MEGAVPN_MVP_RUN_TESTS=1 to enable"
fi

log "build binaries"
./scripts/build.sh

log "install Web UI"
./scripts/install-web.sh "$WEB_ROOT"

log "install systemd units"
install_systemd_units

log "run migrations"
run_migrations

log "start control plane services"
start_services

log "health check"
health_check

cat <<EOF

[mvp-install] MVP control plane is running.
[mvp-install] URL: $PUBLIC_BASE_URL
[mvp-install] Env: $ENV_FILE
[mvp-install] Artifacts: $ARTIFACT_ROOT
[mvp-install] Bootstrap admin: $ADMIN_USERNAME
EOF
if [[ -f "$ADMIN_PASSWORD_FILE" ]]; then
  printf '[mvp-install] Generated admin credentials: %s\n' "$ADMIN_PASSWORD_FILE"
fi
printf '[mvp-install] Next: login, add the first real node, save SSH access, create enrollment token, queue bootstrap.\n'
