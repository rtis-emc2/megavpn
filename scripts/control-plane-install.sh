#!/usr/bin/env bash
set -euo pipefail

export PATH="/snap/bin:/usr/local/go/bin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:${PATH:-}"

APP_DIR="${MEGAVPN_CP_APP_DIR:-/opt/megavpn}"
ENV_FILE="${MEGAVPN_CP_ENV_FILE:-/etc/megavpn/megavpn.env}"
MASTER_KEY_PATH="${MEGAVPN_CP_MASTER_KEY_PATH:-/etc/megavpn/master.key}"
MASTER_KEY_VERSION="${MEGAVPN_CP_MASTER_KEY_VERSION:-v1}"
ARTIFACT_ROOT="${MEGAVPN_CP_ARTIFACT_ROOT:-/var/lib/megavpn/artifacts}"
WEB_ROOT="${MEGAVPN_CP_WEB_ROOT:-}"
API_LISTEN_ADDR="${MEGAVPN_CP_API_LISTEN_ADDR:-}"
PUBLIC_BASE_URL="${MEGAVPN_CP_PUBLIC_BASE_URL:-${MEGAVPN_PUBLIC_BASE_URL:-}}"
CONTROL_DOMAIN="${MEGAVPN_CP_DOMAIN:-}"
TLS_MODE="${MEGAVPN_CP_TLS_MODE:-}"
TRUST_PROXY_HEADERS="${MEGAVPN_CP_TRUST_PROXY_HEADERS:-}"
DATABASE_DSN="${MEGAVPN_CP_DATABASE_DSN:-${MEGAVPN_DATABASE_DSN:-}}"
DB_HOST="${MEGAVPN_CP_DB_HOST:-127.0.0.1}"
DB_PORT="${MEGAVPN_CP_DB_PORT:-5432}"
DB_NAME="${MEGAVPN_CP_DB_NAME:-megavpn}"
DB_USER="${MEGAVPN_CP_DB_USER:-megavpn}"
DB_PASSWORD="${MEGAVPN_CP_DB_PASSWORD:-}"
DB_SSLMODE="${MEGAVPN_CP_DB_SSLMODE:-disable}"
LOG_LEVEL="${MEGAVPN_CP_LOG_LEVEL:-info}"
RUN_TESTS="${MEGAVPN_CP_RUN_TESTS:-0}"
ASSUME_YES="${MEGAVPN_CP_ASSUME_YES:-0}"
INSTALL_PACKAGES="${MEGAVPN_CP_INSTALL_PACKAGES:-}"
VALIDATE_ONLY="${MEGAVPN_CP_VALIDATE_ONLY:-0}"
GO_TARBALL_URL="${MEGAVPN_CP_GO_TARBALL_URL:-}"
GO_TARBALL_SHA256="${MEGAVPN_CP_GO_TARBALL_SHA256:-}"
ADMIN_USERNAME="${MEGAVPN_CP_ADMIN_USERNAME:-${MEGAVPN_BOOTSTRAP_ADMIN_USERNAME:-superadmin}}"
ADMIN_EMAIL="${MEGAVPN_CP_ADMIN_EMAIL:-${MEGAVPN_BOOTSTRAP_ADMIN_EMAIL:-}}"
ADMIN_DISPLAY_NAME="${MEGAVPN_CP_ADMIN_DISPLAY_NAME:-${MEGAVPN_BOOTSTRAP_ADMIN_DISPLAY_NAME:-Superadmin}}"
ADMIN_PASSWORD="${MEGAVPN_CP_ADMIN_PASSWORD:-${MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD:-}}"
ADMIN_PASSWORD_FILE="${MEGAVPN_CP_ADMIN_PASSWORD_FILE:-/root/megavpn-control-plane-admin.txt}"
MASTER_KEY_VALUE="${MEGAVPN_CP_MASTER_KEY_VALUE:-}"
HEALTH_URL="${MEGAVPN_CP_HEALTH_URL:-}"
NGINX_CONF_PATH="${MEGAVPN_CP_NGINX_CONF_PATH:-/etc/nginx/conf.d/megavpn-control-plane.conf}"
TLS_CERT_PATH="${MEGAVPN_CP_TLS_CERT_PATH:-/etc/megavpn/control-plane-tls/install-selfsigned.crt}"
TLS_KEY_PATH="${MEGAVPN_CP_TLS_KEY_PATH:-/etc/megavpn/control-plane-tls/install-selfsigned.key}"
MANAGED_TLS_CERT_PATH="${MEGAVPN_CP_MANAGED_TLS_CERT_PATH:-/etc/megavpn/control-plane-tls/fullchain.pem}"
MANAGED_TLS_KEY_PATH="${MEGAVPN_CP_MANAGED_TLS_KEY_PATH:-/etc/megavpn/control-plane-tls/privkey.pem}"
FORCE_SELF_SIGNED_NGINX="${MEGAVPN_CP_FORCE_SELF_SIGNED_NGINX:-0}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

log() {
  printf '[control-plane-install] %s\n' "$*"
}

warn() {
  printf '[control-plane-install] WARNING: %s\n' "$*" >&2
}

die() {
  printf '[control-plane-install] ERROR: %s\n' "$*" >&2
  exit 1
}

lowercase() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

is_true() {
  case "$(lowercase "$1")" in
    1|true|yes|y|on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

has_tty() {
  [[ -t 0 && -t 1 ]]
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

required_go_version() {
  sed -nE 's/^go ([0-9]+(\.[0-9]+){1,2}).*/\1/p' "$SRC_DIR/go.mod" | head -n 1
}

installed_go_version() {
  command -v go >/dev/null 2>&1 || return 1
  local version
  version="$(go env GOVERSION 2>/dev/null || true)"
  if [[ -z "$version" ]]; then
    version="$(go version | awk '{print $3}')"
  fi
  printf '%s' "$version" | sed -E 's/^go//; s/^([0-9]+(\.[0-9]+){1,2}).*/\1/'
}

version_ge() {
  local have="$1"
  local want="$2"
  [[ -n "$have" && -n "$want" ]] || return 1
  [[ "$(printf '%s\n%s\n' "$want" "$have" | sort -V | head -n 1)" == "$want" ]]
}

file_sha256() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
    return 0
  fi
  die "sha256sum or shasum is required to verify downloaded Go toolchain"
}

default_hostname() {
  local value
  value="$(hostname -f 2>/dev/null || hostname 2>/dev/null || true)"
  if [[ -n "$value" && "$value" != "(none)" ]]; then
    printf '%s' "$value"
    return 0
  fi
  value="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
  [[ -n "$value" ]] && printf '%s' "$value"
}

trim_trailing_slashes() {
  local value="$1"
  while [[ "$value" == */ ]]; do
    value="${value%/}"
  done
  printf '%s' "$value"
}

url_scheme() {
  case "$1" in
    https://*) printf 'https' ;;
    http://*) printf 'http' ;;
    *) printf '' ;;
  esac
}

url_hostport() {
  local value="$1"
  value="${value#http://}"
  value="${value#https://}"
  value="${value%%/*}"
  printf '%s' "$value"
}

url_host() {
  local hp
  hp="$(url_hostport "$1")"
  if [[ "$hp" == \[*\]* ]]; then
    hp="${hp#\[}"
    hp="${hp%%\]*}"
    printf '%s' "$hp"
    return 0
  fi
  printf '%s' "${hp%%:*}"
}

url_port() {
  local scheme hp rest
  scheme="$(url_scheme "$1")"
  hp="$(url_hostport "$1")"
  if [[ "$hp" == \[*\]*:* ]]; then
    rest="${hp##*\]:}"
    [[ "$rest" != "$hp" ]] && printf '%s' "$rest" && return 0
  elif [[ "$hp" == *:* ]]; then
    printf '%s' "${hp##*:}"
    return 0
  fi
  if [[ "$scheme" == "https" ]]; then
    printf '443'
  else
    printf '80'
  fi
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

is_ipv4() {
  [[ "$1" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]
}

is_safe_server_name() {
  local value="$1"
  is_ipv4 "$value" || [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9.-]{0,252}[A-Za-z0-9]$ ]]
}

is_safe_abs_path() {
  [[ "$1" =~ ^/[A-Za-z0-9._/@:+-]+$ ]]
}

require_safe_abs_path() {
  local label="$1"
  local value="$2"
  is_safe_abs_path "$value" || die "$label must be an absolute path without whitespace or shell metacharacters: $value"
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

generate_hex32() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32 | tr -d '\n'
    return 0
  fi
  od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
}

generate_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32 | tr -d '\n'
    return 0
  fi
  generate_hex32
}

generate_master_key() {
  generate_hex32
}

urlencode() {
  local raw="$1"
  local out=""
  local char encoded
  local i
  LC_ALL=C
  for ((i = 0; i < ${#raw}; i++)); do
    char="${raw:i:1}"
    case "$char" in
      [a-zA-Z0-9.~_-])
        out+="$char"
        ;;
      *)
        printf -v encoded '%%%02X' "'$char"
        out+="$encoded"
        ;;
    esac
  done
  printf '%s' "$out"
}

redact_dsn() {
  sed -E 's#(postgres(ql)?://[^:/@]+:)[^@]+@#\1****@#' <<<"$1"
}

prompt_text() {
  local var_name="$1"
  local label="$2"
  local default_value="$3"
  local required="${4:-false}"
  local current="${!var_name:-}"
  local answer
  if [[ -n "$current" ]]; then
    return 0
  fi
  if is_true "$ASSUME_YES" || ! has_tty; then
    if [[ -n "$default_value" ]]; then
      printf -v "$var_name" '%s' "$default_value"
      return 0
    fi
    [[ "$required" == "true" ]] && die "$label is required; set $var_name or the corresponding MEGAVPN_CP_* variable"
    return 0
  fi
  if [[ -n "$default_value" ]]; then
    read -r -p "$label [$default_value]: " answer
    answer="${answer:-$default_value}"
  else
    read -r -p "$label: " answer
  fi
  if [[ -z "$answer" && "$required" == "true" ]]; then
    die "$label is required"
  fi
  printf -v "$var_name" '%s' "$answer"
}

prompt_secret() {
  local var_name="$1"
  local label="$2"
  local required="${3:-false}"
  local current="${!var_name:-}"
  local answer=""
  if [[ -n "$current" ]]; then
    return 0
  fi
  if is_true "$ASSUME_YES" || ! has_tty; then
    [[ "$required" == "true" ]] && die "$label is required; set $var_name or the corresponding MEGAVPN_CP_* variable"
    return 0
  fi
  read -r -s -p "$label: " answer
  printf '\n'
  if [[ -z "$answer" && "$required" == "true" ]]; then
    die "$label is required"
  fi
  printf -v "$var_name" '%s' "$answer"
}

prompt_yes_no() {
  local label="$1"
  local default_value="${2:-yes}"
  local answer suffix
  if is_true "$ASSUME_YES" || ! has_tty; then
    [[ "$default_value" == "yes" ]]
    return $?
  fi
  if [[ "$default_value" == "yes" ]]; then
    suffix='[Y/n]'
  else
    suffix='[y/N]'
  fi
  read -r -p "$label $suffix: " answer
  answer="$(lowercase "$answer")"
  if [[ -z "$answer" ]]; then
    [[ "$default_value" == "yes" ]]
    return $?
  fi
  case "$answer" in
    y|yes)
      return 0
      ;;
    n|no)
      return 1
      ;;
    *)
      die "unknown answer: $answer"
      ;;
  esac
}

normalize_tls_mode() {
  case "$(lowercase "$1")" in
    self|self-signed|self_signed|self-signed-nginx|self_signed_nginx|nginx)
      printf 'self-signed-nginx'
      ;;
    external|proxy|external-https|external_https)
      printf 'external-https'
      ;;
    http|direct-http|http-direct|lab)
      printf 'http-direct'
      ;;
    *)
      printf ''
      ;;
  esac
}

prompt_tls_mode() {
  local answer normalized
  if [[ -n "$TLS_MODE" ]]; then
    normalized="$(normalize_tls_mode "$TLS_MODE")"
    [[ -n "$normalized" ]] || die "unsupported MEGAVPN_CP_TLS_MODE=$TLS_MODE"
    TLS_MODE="$normalized"
    return 0
  fi
  if is_true "$ASSUME_YES" || ! has_tty; then
    TLS_MODE="self-signed-nginx"
    return 0
  fi
  cat <<'EOF'

TLS/public access mode:
  1) self-signed-nginx  - installer writes local HTTPS nginx edge with a self-signed cert
  2) external-https     - HTTPS is terminated by an existing external proxy/LB
  3) http-direct        - lab only, API listens directly over HTTP
EOF
  read -r -p "Select mode [1]: " answer
  answer="${answer:-1}"
  case "$answer" in
    1) TLS_MODE="self-signed-nginx" ;;
    2) TLS_MODE="external-https" ;;
    3) TLS_MODE="http-direct" ;;
    *) die "unsupported TLS mode selection: $answer" ;;
  esac
}

default_api_listen_addr() {
  if [[ "$TLS_MODE" == "http-direct" ]]; then
    printf '0.0.0.0:8080'
  else
    printf '127.0.0.1:8080'
  fi
}

default_public_url() {
  local domain="$1"
  if [[ "$TLS_MODE" == "http-direct" ]]; then
    printf 'http://%s:8080' "$domain"
  else
    printf 'https://%s' "$domain"
  fi
}

health_url_from_listen() {
  local listen="$1"
  local port="${listen##*:}"
  [[ "$port" =~ ^[0-9]+$ ]] || port="8080"
  printf 'http://127.0.0.1:%s/healthz' "$port"
}

validate_public_url() {
  local scheme
  PUBLIC_BASE_URL="$(trim_trailing_slashes "$PUBLIC_BASE_URL")"
  scheme="$(url_scheme "$PUBLIC_BASE_URL")"
  [[ "$scheme" == "http" || "$scheme" == "https" ]] || die "public URL must start with http:// or https://"
  if [[ "$TLS_MODE" != "http-direct" && "$scheme" != "https" ]]; then
    die "production TLS modes require https:// public URL"
  fi
  if is_loopback_url "$PUBLIC_BASE_URL"; then
    die "public URL is loopback; remote agents cannot enroll: $PUBLIC_BASE_URL"
  fi
}

prompt_database() {
  local use_full_dsn password_encoded user_encoded
  if [[ -n "$DATABASE_DSN" ]]; then
    return 0
  fi
  if has_tty && ! is_true "$ASSUME_YES"; then
    if prompt_yes_no "Enter full PostgreSQL DSN instead of field-by-field wizard?" "no"; then
      use_full_dsn="yes"
    else
      use_full_dsn="no"
    fi
  else
    use_full_dsn="no"
  fi
  if [[ "$use_full_dsn" == "yes" ]]; then
    prompt_text DATABASE_DSN "PostgreSQL DSN" "" true
    return 0
  fi
  prompt_text DB_HOST "PostgreSQL host" "$DB_HOST" true
  prompt_text DB_PORT "PostgreSQL port" "$DB_PORT" true
  prompt_text DB_NAME "PostgreSQL database" "$DB_NAME" true
  prompt_text DB_USER "PostgreSQL user" "$DB_USER" true
  prompt_secret DB_PASSWORD "PostgreSQL password" true
  prompt_text DB_SSLMODE "PostgreSQL sslmode" "$DB_SSLMODE" true
  user_encoded="$(urlencode "$DB_USER")"
  password_encoded="$(urlencode "$DB_PASSWORD")"
  DATABASE_DSN="postgres://$user_encoded:$password_encoded@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=$DB_SSLMODE"
}

prompt_admin() {
  local generated_password old_umask
  if [[ -z "$ADMIN_EMAIL" ]]; then
    ADMIN_EMAIL="superadmin@$(url_host "$PUBLIC_BASE_URL")"
  fi
  prompt_text ADMIN_USERNAME "Bootstrap admin username" "$ADMIN_USERNAME" true
  prompt_text ADMIN_EMAIL "Bootstrap admin email" "$ADMIN_EMAIL" true
  prompt_text ADMIN_DISPLAY_NAME "Bootstrap admin display name" "$ADMIN_DISPLAY_NAME" true
  if [[ -z "$ADMIN_PASSWORD" ]]; then
    if has_tty && ! is_true "$ASSUME_YES"; then
      if prompt_yes_no "Generate a random bootstrap admin password?" "yes"; then
        ADMIN_PASSWORD="$(generate_secret)"
        generated_password="yes"
      else
        prompt_secret ADMIN_PASSWORD "Bootstrap admin password" true
        generated_password="no"
      fi
    else
      ADMIN_PASSWORD="$(generate_secret)"
      generated_password="yes"
    fi
  else
    generated_password="no"
  fi
  if [[ "$generated_password" == "yes" ]]; then
    if is_true "$VALIDATE_ONLY"; then
      return 0
    fi
    install -d -m 0700 "$(dirname "$ADMIN_PASSWORD_FILE")"
    old_umask="$(umask)"
    umask 0077
    {
      printf 'RTIS MegaVPN Control Plane bootstrap admin\n'
      printf 'username=%s\n' "$ADMIN_USERNAME"
      printf 'password=%s\n' "$ADMIN_PASSWORD"
      printf 'created_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    } >"$ADMIN_PASSWORD_FILE"
    umask "$old_umask"
  fi
}

prompt_configuration() {
  local default_domain default_public default_listen default_trust
  prompt_tls_mode
  if [[ -n "$PUBLIC_BASE_URL" && -z "$CONTROL_DOMAIN" ]]; then
    default_domain="$(url_host "$PUBLIC_BASE_URL")"
  else
    default_domain="$(default_hostname)"
  fi
  prompt_text CONTROL_DOMAIN "Control Plane public domain/IP" "$default_domain" true
  if [[ -z "$PUBLIC_BASE_URL" ]]; then
    default_public="$(default_public_url "$CONTROL_DOMAIN")"
  else
    default_public="$PUBLIC_BASE_URL"
  fi
  prompt_text PUBLIC_BASE_URL "Control Plane public URL" "$default_public" true
  validate_public_url
  if [[ -z "$CONTROL_DOMAIN" ]]; then
    CONTROL_DOMAIN="$(url_host "$PUBLIC_BASE_URL")"
  fi
  default_listen="$(default_api_listen_addr)"
  prompt_text API_LISTEN_ADDR "API listen address" "$default_listen" true
  if [[ -z "$TRUST_PROXY_HEADERS" ]]; then
    if [[ "$TLS_MODE" == "http-direct" ]]; then
      default_trust="false"
    else
      default_trust="true"
    fi
    prompt_text TRUST_PROXY_HEADERS "Trust reverse-proxy headers" "$default_trust" true
  fi
  prompt_text APP_DIR "Install directory" "$APP_DIR" true
  prompt_text ENV_FILE "Runtime env file" "$ENV_FILE" true
  prompt_text MASTER_KEY_PATH "Secret master key path" "$MASTER_KEY_PATH" true
  prompt_text MASTER_KEY_VERSION "Secret master key version" "$MASTER_KEY_VERSION" true
  prompt_text ARTIFACT_ROOT "Artifact storage directory" "$ARTIFACT_ROOT" true
  if [[ -z "$WEB_ROOT" ]]; then
    WEB_ROOT="$APP_DIR/web"
  fi
  prompt_text WEB_ROOT "Web UI directory" "$WEB_ROOT" true
  prompt_text LOG_LEVEL "Log level" "$LOG_LEVEL" true
  prompt_text RUN_TESTS "Run Go tests during install (0/1)" "$RUN_TESTS" true
  if [[ -z "$HEALTH_URL" ]]; then
    HEALTH_URL="$(health_url_from_listen "$API_LISTEN_ADDR")"
  fi
  prompt_text HEALTH_URL "Local health check URL" "$HEALTH_URL" true
  prompt_database
  prompt_admin
}

validate_configuration() {
  if ! is_true "$VALIDATE_ONLY"; then
    [[ "${EUID:-$(id -u)}" -eq 0 ]] || die "run as root"
  fi
  [[ -n "$DATABASE_DSN" ]] || die "database DSN is required"
  require_safe_abs_path "install directory" "$APP_DIR"
  require_safe_abs_path "web root" "$WEB_ROOT"
  require_safe_abs_path "runtime env file" "$ENV_FILE"
  require_safe_abs_path "master key path" "$MASTER_KEY_PATH"
  require_safe_abs_path "artifact root" "$ARTIFACT_ROOT"
  require_safe_abs_path "nginx config path" "$NGINX_CONF_PATH"
  require_safe_abs_path "TLS certificate path" "$TLS_CERT_PATH"
  require_safe_abs_path "TLS key path" "$TLS_KEY_PATH"
  require_safe_abs_path "managed TLS certificate path" "$MANAGED_TLS_CERT_PATH"
  require_safe_abs_path "managed TLS key path" "$MANAGED_TLS_KEY_PATH"
  [[ "$API_LISTEN_ADDR" == *:* ]] || die "API listen address must include host:port"
  if [[ "$TLS_MODE" == "self-signed-nginx" ]] && ! is_safe_server_name "$CONTROL_DOMAIN"; then
    die "Control Plane domain/IP contains unsupported characters for nginx server_name: $CONTROL_DOMAIN"
  fi
  if [[ "$TLS_MODE" == "http-direct" ]]; then
    warn "http-direct mode is for lab only; do not expose it as production Control Plane"
  fi
}

review_configuration() {
  cat <<EOF

Control Plane install plan:
  install dir:       $APP_DIR
  env file:          $ENV_FILE
  public URL:        $PUBLIC_BASE_URL
  TLS mode:          $TLS_MODE
  API listen:        $API_LISTEN_ADDR
  trust proxy:       $TRUST_PROXY_HEADERS
  database DSN:      $(redact_dsn "$DATABASE_DSN")
  master key path:   $MASTER_KEY_PATH
  artifact root:     $ARTIFACT_ROOT
  web root:          $WEB_ROOT
  bootstrap admin:   $ADMIN_USERNAME <$ADMIN_EMAIL>
  run tests:         $RUN_TESTS
EOF
  if ! is_true "$ASSUME_YES" && has_tty; then
    prompt_yes_no "Continue with installation?" "no" || exit 0
  fi
}

maybe_install_packages() {
  local package_commands=(curl rsync openssl tar)
  local packages=(curl rsync openssl tar ca-certificates)
  local missing=()
  local command_name package
  if [[ "$TLS_MODE" == "self-signed-nginx" ]]; then
    package_commands+=(nginx)
    packages+=(nginx)
  fi
  for command_name in "${package_commands[@]}"; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
      missing+=("$command_name")
    fi
  done
  if [[ ${#missing[@]} -eq 0 ]]; then
    return 0
  fi
  if [[ -z "$INSTALL_PACKAGES" ]]; then
    if command -v apt-get >/dev/null 2>&1 && prompt_yes_no "Install missing apt packages: ${missing[*]}?" "yes"; then
      INSTALL_PACKAGES="1"
    else
      INSTALL_PACKAGES="0"
    fi
  fi
  if is_true "$INSTALL_PACKAGES"; then
    command -v apt-get >/dev/null 2>&1 || die "automatic package install supports apt-get only"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y "${packages[@]}"
  fi
}

install_go_from_tarball() {
  local tmp actual backup
  [[ -n "$GO_TARBALL_URL" ]] || return 1
  [[ -n "$GO_TARBALL_SHA256" ]] || die "MEGAVPN_CP_GO_TARBALL_SHA256 is required with MEGAVPN_CP_GO_TARBALL_URL"
  require_command curl
  require_command tar
  tmp="$(mktemp)"
  log "download pinned Go toolchain: $GO_TARBALL_URL"
  curl -fsSL "$GO_TARBALL_URL" -o "$tmp"
  actual="$(file_sha256 "$tmp")"
  if [[ "$(lowercase "$actual")" != "$(lowercase "$GO_TARBALL_SHA256")" ]]; then
    rm -f "$tmp"
    die "Go toolchain SHA-256 mismatch: got $actual, want $GO_TARBALL_SHA256"
  fi
  if [[ -d /usr/local/go ]]; then
    backup="/usr/local/go.backup.$(date -u +%Y%m%d%H%M%S)"
    log "backup existing /usr/local/go to $backup"
    mv /usr/local/go "$backup"
  fi
  tar -C /usr/local -xzf "$tmp"
  rm -f "$tmp"
  export PATH="/usr/local/go/bin:$PATH"
}

ensure_go_toolchain() {
  local required installed
  required="$(required_go_version)"
  [[ -n "$required" ]] || die "cannot determine required Go version from go.mod"
  installed="$(installed_go_version || true)"
  if version_ge "$installed" "$required"; then
    log "Go toolchain OK: $installed >= $required"
    return 0
  fi
  if [[ -n "$installed" ]]; then
    warn "installed Go $installed is older than required $required"
  else
    warn "Go toolchain is not installed"
  fi

  if [[ -n "$GO_TARBALL_URL" ]]; then
    install_go_from_tarball
    installed="$(installed_go_version || true)"
    version_ge "$installed" "$required" || die "installed Go $installed does not satisfy required $required"
    log "Go toolchain installed: $installed"
    return 0
  fi

  if command -v apt-get >/dev/null 2>&1; then
    if [[ -z "$INSTALL_PACKAGES" ]]; then
      if prompt_yes_no "Install Go from apt and verify version >= $required?" "yes"; then
        INSTALL_PACKAGES="1"
      else
        INSTALL_PACKAGES="0"
      fi
    fi
    if is_true "$INSTALL_PACKAGES"; then
      export DEBIAN_FRONTEND=noninteractive
      apt-get update
      apt-get install -y golang-go
      installed="$(installed_go_version || true)"
      if version_ge "$installed" "$required"; then
        log "Go toolchain installed from apt: $installed"
        return 0
      fi
      warn "apt installed Go $installed, but the project requires $required"
    fi
  fi

  die "Go >= $required is required. Install Go manually or set MEGAVPN_CP_GO_TARBALL_URL and MEGAVPN_CP_GO_TARBALL_SHA256 for a pinned toolchain install"
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

ensure_master_key() {
  local old_umask
  if [[ -f "$MASTER_KEY_PATH" ]]; then
    log "master key exists: $MASTER_KEY_PATH"
    chmod 0600 "$MASTER_KEY_PATH"
    return 0
  fi
  if [[ -z "$MASTER_KEY_VALUE" ]]; then
    if has_tty && ! is_true "$ASSUME_YES"; then
      if prompt_yes_no "Generate a new 32-byte secret master key?" "yes"; then
        MASTER_KEY_VALUE="$(generate_master_key)"
      else
        prompt_secret MASTER_KEY_VALUE "Secret master key value" true
      fi
    else
      MASTER_KEY_VALUE="$(generate_master_key)"
    fi
  fi
  log "write master key: $MASTER_KEY_PATH"
  install -d -m 0750 "$(dirname "$MASTER_KEY_PATH")"
  old_umask="$(umask)"
  umask 0077
  printf '%s\n' "$MASTER_KEY_VALUE" >"$MASTER_KEY_PATH"
  umask "$old_umask"
  chmod 0600 "$MASTER_KEY_PATH"
}

write_runtime_env() {
  local tmp backup
  install -d -m 0750 "$(dirname "$ENV_FILE")"
  if [[ -f "$ENV_FILE" ]]; then
    backup="${ENV_FILE}.bak.$(date -u +%Y%m%d%H%M%S)"
    cp -p "$ENV_FILE" "$backup"
    log "existing env file backed up to $backup"
  fi
  tmp="${ENV_FILE}.tmp.$$"
  trap 'rm -f "$tmp"' RETURN
  {
    printf '# Managed by scripts/control-plane-install.sh\n'
    printf '# Re-run the installer or edit carefully with service restart.\n'
  } >"$tmp"
  write_env_line "$tmp" "MEGAVPN_DATABASE_DSN" "$DATABASE_DSN"
  write_env_line "$tmp" "MEGAVPN_API_LISTEN_ADDR" "$API_LISTEN_ADDR"
  write_env_line "$tmp" "MEGAVPN_PUBLIC_BASE_URL" "$PUBLIC_BASE_URL"
  write_env_line "$tmp" "MEGAVPN_PRODUCTION_MODE" "true"
  write_env_line "$tmp" "MEGAVPN_WEB_ROOT" "$WEB_ROOT"
  write_env_line "$tmp" "MEGAVPN_ARTIFACT_ROOT" "$ARTIFACT_ROOT"
  write_env_line "$tmp" "MEGAVPN_MASTER_KEY_PATH" "$MASTER_KEY_PATH"
  write_env_line "$tmp" "MEGAVPN_MASTER_KEY_VERSION" "$MASTER_KEY_VERSION"
  write_env_line "$tmp" "MEGAVPN_TRUST_PROXY_HEADERS" "$TRUST_PROXY_HEADERS"
  write_env_line "$tmp" "MEGAVPN_AGENT_ALLOW_AUTO_REGISTER" "false"
  write_env_line "$tmp" "MEGAVPN_AGENT_SIGNATURE_ENFORCE" "true"
  write_env_line "$tmp" "MEGAVPN_AGENT_SIGNATURE_WINDOW" "5m"
  write_env_line "$tmp" "MEGAVPN_BOOTSTRAP_ADMIN_USERNAME" "$ADMIN_USERNAME"
  write_env_line "$tmp" "MEGAVPN_BOOTSTRAP_ADMIN_EMAIL" "$ADMIN_EMAIL"
  write_env_line "$tmp" "MEGAVPN_BOOTSTRAP_ADMIN_DISPLAY_NAME" "$ADMIN_DISPLAY_NAME"
  write_env_line "$tmp" "MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD" "$ADMIN_PASSWORD"
  write_env_line "$tmp" "MEGAVPN_LOG_LEVEL" "$LOG_LEVEL"
  install -m 0600 "$tmp" "$ENV_FILE"
  rm -f "$tmp"
  trap - RETURN
}

install_systemd_units() {
  log "install systemd units"
  install -m 0644 deploy/systemd/megavpn-migrate.service /etc/systemd/system/megavpn-migrate.service
  install -m 0644 deploy/systemd/megavpn-api.service /etc/systemd/system/megavpn-api.service
  install -m 0644 deploy/systemd/megavpn-worker.service /etc/systemd/system/megavpn-worker.service
  install -m 0644 deploy/systemd/megavpn-agent.service /etc/systemd/system/megavpn-agent.service
  install -d -m 0755 /etc/systemd/system/megavpn-migrate.service.d
  install -d -m 0755 /etc/systemd/system/megavpn-api.service.d
  install -d -m 0755 /etc/systemd/system/megavpn-worker.service.d
  install -d -m 0755 /etc/systemd/system/megavpn-agent.service.d
  cat >/etc/systemd/system/megavpn-migrate.service.d/10-control-plane-paths.conf <<EOF
[Service]
WorkingDirectory=$APP_DIR
Environment=MEGAVPN_MIGRATIONS_DIR=$APP_DIR/migrations
ExecStart=
ExecStart=$APP_DIR/bin/megavpn-migrate
EOF
  cat >/etc/systemd/system/megavpn-api.service.d/10-control-plane-paths.conf <<EOF
[Service]
WorkingDirectory=$APP_DIR
ExecStart=
ExecStart=$APP_DIR/bin/megavpn-api
EOF
  cat >/etc/systemd/system/megavpn-worker.service.d/10-control-plane-paths.conf <<EOF
[Service]
WorkingDirectory=$APP_DIR
ExecStart=
ExecStart=$APP_DIR/bin/megavpn-worker
EOF
  cat >/etc/systemd/system/megavpn-agent.service.d/10-control-plane-paths.conf <<EOF
[Service]
WorkingDirectory=$APP_DIR
ExecStart=
ExecStart=$APP_DIR/bin/megavpn-agent
EOF
  systemctl daemon-reload
}

san_for_domain() {
  if is_ipv4 "$CONTROL_DOMAIN"; then
    printf 'IP:%s' "$CONTROL_DOMAIN"
  else
    printf 'DNS:%s' "$CONTROL_DOMAIN"
  fi
}

install_self_signed_nginx() {
  local public_port upstream_hostport redirect_url san cert_path key_path
  [[ "$TLS_MODE" == "self-signed-nginx" ]] || return 0
  require_command nginx
  if managed_tls_edge_present && ! is_true "$FORCE_SELF_SIGNED_NGINX"; then
    log "preserve managed nginx TLS edge: $NGINX_CONF_PATH"
    nginx -t
    systemctl enable --now nginx.service
    systemctl reload nginx.service
    return 0
  fi
  public_port="$(url_port "$PUBLIC_BASE_URL")"
  upstream_hostport="$API_LISTEN_ADDR"
  if [[ "$upstream_hostport" == 0.0.0.0:* ]]; then
    upstream_hostport="127.0.0.1:${upstream_hostport##*:}"
  fi
  cert_path="$TLS_CERT_PATH"
  key_path="$TLS_KEY_PATH"
  if managed_tls_material_present && ! is_true "$FORCE_SELF_SIGNED_NGINX"; then
    cert_path="$MANAGED_TLS_CERT_PATH"
    key_path="$MANAGED_TLS_KEY_PATH"
    log "use existing managed nginx TLS material: $cert_path"
  else
    san="$(san_for_domain)"
    install -d -m 0750 "$(dirname "$TLS_CERT_PATH")"
    if [[ ! -f "$TLS_CERT_PATH" || ! -f "$TLS_KEY_PATH" ]]; then
      log "generate self-signed TLS certificate for $CONTROL_DOMAIN"
      openssl req -x509 -newkey rsa:4096 -sha256 -days 825 -nodes \
        -keyout "$TLS_KEY_PATH" \
        -out "$TLS_CERT_PATH" \
        -subj "/CN=$CONTROL_DOMAIN" \
        -addext "subjectAltName=$san"
      chmod 0640 "$TLS_CERT_PATH"
      chmod 0600 "$TLS_KEY_PATH"
    fi
  fi
  redirect_url="https://\$host"
  if [[ "$public_port" != "443" ]]; then
    redirect_url="https://\$host:$public_port"
  fi
  log "write nginx TLS edge: $NGINX_CONF_PATH"
  cat >"$NGINX_CONF_PATH" <<EOF
map \$http_upgrade \$megavpn_connection_upgrade {
    default upgrade;
    '' close;
}

server {
    listen 80;
    server_name $CONTROL_DOMAIN;
    return 301 $redirect_url\$request_uri;
}

server {
    listen $public_port ssl http2;
    server_name $CONTROL_DOMAIN;

    ssl_certificate $cert_path;
    ssl_certificate_key $key_path;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:MEGAVPN_INSTALL_TLS:10m;
    ssl_session_timeout 10m;

    client_max_body_size 16m;

    location / {
        proxy_http_version 1.1;
        proxy_set_header Host \$http_host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$remote_addr;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Host \$http_host;
        proxy_set_header X-Forwarded-Port $public_port;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \$megavpn_connection_upgrade;
        proxy_read_timeout 120s;
        proxy_send_timeout 120s;
        proxy_pass http://$upstream_hostport;
    }
}
EOF
  nginx -t
  systemctl enable --now nginx.service
  systemctl reload nginx.service
}

managed_tls_material_present() {
  [[ -s "$MANAGED_TLS_CERT_PATH" && -s "$MANAGED_TLS_KEY_PATH" ]]
}

managed_tls_edge_present() {
  [[ -f "$NGINX_CONF_PATH" ]] || return 1
  managed_tls_material_present || return 1
  grep -Fq "$MANAGED_TLS_CERT_PATH" "$NGINX_CONF_PATH" || return 1
  grep -Fq "$MANAGED_TLS_KEY_PATH" "$NGINX_CONF_PATH" || return 1
}

run_migrations() {
  local result
  log "run migrations"
  systemctl reset-failed megavpn-migrate.service >/dev/null 2>&1 || true
  systemctl start megavpn-migrate.service
  result="$(systemctl show -p Result --value megavpn-migrate.service 2>/dev/null || true)"
  [[ "$result" == "success" ]] || die "migrations failed; inspect: journalctl -u megavpn-migrate.service -n 120 --no-pager"
}

start_services() {
  log "start API and worker"
  systemctl enable --now megavpn-api.service megavpn-worker.service
  systemctl restart megavpn-api.service megavpn-worker.service
}

health_check() {
  local attempt
  log "health check: $HEALTH_URL"
  for attempt in {1..30}; do
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

main() {
  prompt_configuration
  validate_configuration
  review_configuration
  if is_true "$VALIDATE_ONLY"; then
    log "validation completed; MEGAVPN_CP_VALIDATE_ONLY=1, installation skipped"
    return 0
  fi
  maybe_install_packages
  ensure_go_toolchain
  if [[ "$TLS_MODE" == "self-signed-nginx" ]]; then
    require_command openssl
  fi
  require_command rsync
  require_command systemctl
  require_command curl

  sync_source_tree
  cd "$APP_DIR"

  install -d -m 0750 "$ARTIFACT_ROOT"
  install -d -m 0750 /etc/megavpn/control-plane-tls
  ensure_master_key
  write_runtime_env

  log "download Go modules"
  go mod download

  if is_true "$RUN_TESTS"; then
    log "run Go tests"
    go test ./...
  else
    log "skip Go tests; set MEGAVPN_CP_RUN_TESTS=1 to enable"
  fi

  log "build binaries"
  ./scripts/build.sh

  log "install Web UI"
  ./scripts/install-web.sh "$WEB_ROOT"

  install_systemd_units
  run_migrations
  start_services
  install_self_signed_nginx
  health_check

  cat <<EOF

[control-plane-install] Control Plane is running.
[control-plane-install] URL: $PUBLIC_BASE_URL
[control-plane-install] Env: $ENV_FILE
[control-plane-install] Artifacts: $ARTIFACT_ROOT
[control-plane-install] Master key: $MASTER_KEY_PATH
[control-plane-install] Bootstrap admin: $ADMIN_USERNAME
EOF
  if [[ -f "$ADMIN_PASSWORD_FILE" ]]; then
    printf '[control-plane-install] Generated admin credentials: %s\n' "$ADMIN_PASSWORD_FILE"
  fi
  if [[ "$TLS_MODE" == "self-signed-nginx" ]]; then
    if managed_tls_material_present && ! is_true "$FORCE_SELF_SIGNED_NGINX"; then
      printf '[control-plane-install] Managed TLS certificate: %s\n' "$MANAGED_TLS_CERT_PATH"
      printf '[control-plane-install] Managed TLS key: %s\n' "$MANAGED_TLS_KEY_PATH"
    else
      printf '[control-plane-install] Self-signed TLS certificate: %s\n' "$TLS_CERT_PATH"
      printf '[control-plane-install] Replace it later through Certificates + Settings -> Control Plane TLS.\n'
    fi
  fi
  printf '[control-plane-install] Next: login, create ingress/egress nodes, issue enrollment tokens and start agents.\n'
}

main "$@"
