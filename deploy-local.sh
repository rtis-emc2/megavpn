#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${MEGAVPN_DEPLOY_APP_DIR:-/opt/megavpn}"
REMOTE="${MEGAVPN_DEPLOY_REMOTE:-origin}"
BRANCH="${MEGAVPN_DEPLOY_BRANCH:-}"
HEALTH_URL="${MEGAVPN_DEPLOY_HEALTH_URL:-http://127.0.0.1:8080/healthz}"
RUN_TESTS="${MEGAVPN_DEPLOY_RUN_TESTS:-1}"
AUTO_YES="${MEGAVPN_DEPLOY_YES:-0}"
ALLOW_DIRTY="${MEGAVPN_DEPLOY_ALLOW_DIRTY:-0}"
ENV_FILE="${MEGAVPN_DEPLOY_ENV_FILE:-/etc/megavpn/megavpn.env}"
SYNC_MODE="${MEGAVPN_DEPLOY_SYNC_MODE:-auto}"
ALLOW_HISTORY_REWRITE="${MEGAVPN_DEPLOY_ALLOW_HISTORY_REWRITE:-0}"

SERVICES=("megavpn-api" "megavpn-worker" "megavpn-agent" "megavpn-migrate")
CORE_SERVICES=("megavpn-api" "megavpn-worker")

log() {
  printf '[deploy] %s\n' "$*"
}

die() {
  printf '[deploy] ERROR: %s\n' "$*" >&2
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

confirm_update() {
  if is_true "$AUTO_YES"; then
    return 0
  fi

  printf 'Update RTIS MegaVPN in %s from GitHub and deploy it? [y/N]: ' "$APP_DIR"
  read -r answer
  case "${answer,,}" in
    y|yes)
      return 0
      ;;
    n|no|'')
      log "deployment canceled by operator"
      exit 0
      ;;
    *)
      die "unknown answer: $answer"
      ;;
  esac
}

confirm_history_rewrite_reset() {
  local branch="$1"
  local local_sha="$2"
  local remote_sha="$3"
  if is_true "$ALLOW_HISTORY_REWRITE"; then
    log "history rewrite reset explicitly allowed by MEGAVPN_DEPLOY_ALLOW_HISTORY_REWRITE=1"
    return 0
  fi
  if is_true "$AUTO_YES"; then
    die "local history diverged from $REMOTE/$branch; set MEGAVPN_DEPLOY_ALLOW_HISTORY_REWRITE=1 to save a backup branch and reset to remote"
  fi
  printf 'Local history diverged from %s/%s.\n' "$REMOTE" "$branch"
  printf 'Current HEAD: %s\nRemote HEAD:  %s\n' "$local_sha" "$remote_sha"
  printf 'Create local backup branch and reset this checkout to remote? [y/N]: '
  read -r answer
  case "${answer,,}" in
    y|yes)
      return 0
      ;;
    n|no|'')
      die "history rewrite reset not approved"
      ;;
    *)
      die "unknown answer: $answer"
      ;;
  esac
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

load_runtime_env() {
  if [[ ! -f "$ENV_FILE" ]]; then
    log "runtime env file not found: $ENV_FILE"
    return 0
  fi
  log "load runtime env: $ENV_FILE"
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
}

service_exists() {
  systemctl cat "$1.service" >/dev/null 2>&1
}

service_active() {
  systemctl is-active --quiet "$1.service"
}

systemctl_if_exists() {
  local action="$1"
  shift
  local selected=()
  local service
  for service in "$@"; do
    if service_exists "$service"; then
      selected+=("$service")
    else
      log "skip $action for missing service: $service"
    fi
  done
  if [[ ${#selected[@]} -gt 0 ]]; then
    systemctl "$action" "${selected[@]}"
  fi
}

current_branch() {
  git branch --show-current
}

backup_branch_name() {
  local head_sha
  head_sha="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  printf 'deploy-backup/%s-%s\n' "$(date -u +%Y%m%d-%H%M%S)" "$head_sha"
}

assert_git_repo_ready() {
  git rev-parse --is-inside-work-tree >/dev/null 2>&1 || die "$APP_DIR is not a git worktree"
  if [[ "$ALLOW_DIRTY" != "1" && -n "$(git status --porcelain)" ]]; then
    git status --short >&2
    die "working tree is dirty; commit/stash changes or set MEGAVPN_DEPLOY_ALLOW_DIRTY=1"
  fi
}

update_from_git() {
  local branch="$BRANCH"
  local local_sha remote_sha backup_branch
  if [[ -z "$branch" ]]; then
    branch="$(current_branch)"
  fi
  [[ -n "$branch" ]] || die "unable to detect git branch; set MEGAVPN_DEPLOY_BRANCH"

  log "fetch $REMOTE/$branch"
  git fetch --prune "$REMOTE" "$branch"

  remote_sha="$(git rev-parse "$REMOTE/$branch" 2>/dev/null)" || die "unable to resolve remote ref: $REMOTE/$branch"
  local_sha="$(git rev-parse HEAD 2>/dev/null)" || die "unable to resolve current HEAD"

  if [[ "$local_sha" == "$remote_sha" ]]; then
    log "already at $REMOTE/$branch ($remote_sha)"
    return 0
  fi

  case "$SYNC_MODE" in
    ff-only)
      log "sync mode ff-only: merge --ff-only $REMOTE/$branch"
      git merge --ff-only "$REMOTE/$branch"
      ;;
    reset-hard)
      backup_branch="$(backup_branch_name)"
      log "sync mode reset-hard: save backup branch $backup_branch -> $local_sha"
      git branch "$backup_branch" "$local_sha"
      log "reset --hard $REMOTE/$branch"
      git reset --hard "$REMOTE/$branch"
      ;;
    auto)
      if git merge-base --is-ancestor HEAD "$REMOTE/$branch"; then
        log "sync mode auto: fast-forward merge to $REMOTE/$branch"
        git merge --ff-only "$REMOTE/$branch"
        return 0
      fi
      confirm_history_rewrite_reset "$branch" "$local_sha" "$remote_sha"
      backup_branch="$(backup_branch_name)"
      log "sync mode auto: history diverged, save backup branch $backup_branch -> $local_sha"
      git branch "$backup_branch" "$local_sha"
      log "sync mode auto: reset --hard $REMOTE/$branch"
      git reset --hard "$REMOTE/$branch"
      ;;
    *)
      die "unknown MEGAVPN_DEPLOY_SYNC_MODE: $SYNC_MODE (expected auto, ff-only or reset-hard)"
      ;;
  esac
}

run_health_check() {
  log "health check: $HEALTH_URL"
  if command -v jq >/dev/null 2>&1; then
    curl -fsS "$HEALTH_URL" | jq
  else
    curl -fsS "$HEALTH_URL"
    echo
  fi
}

require_command git
require_command go
require_command systemctl
require_command curl
require_command rsync

[[ "${EUID:-$(id -u)}" -eq 0 ]] || die "deployment must run as root"
[[ -d "$APP_DIR" ]] || die "application directory does not exist: $APP_DIR"
cd "$APP_DIR"
load_runtime_env

confirm_update
assert_git_repo_ready
AGENT_WAS_ACTIVE=0
if service_active "megavpn-agent"; then
  AGENT_WAS_ACTIVE=1
fi

log "[1/10] Update source from GitHub"
update_from_git
assert_git_repo_ready

log "[2/10] Download Go modules"
go mod download

if [[ "$RUN_TESTS" == "1" || "$RUN_TESTS" == "true" || "$RUN_TESTS" == "yes" ]]; then
  log "[3/10] Run tests"
  go test ./...
else
  log "[3/10] Skip tests by MEGAVPN_DEPLOY_RUN_TESTS=$RUN_TESTS"
fi

log "[4/10] Build binaries"
./scripts/build.sh

log "[5/10] Install Web UI"
./scripts/install-web.sh "$APP_DIR/web"

log "[6/10] Stop core services"
systemctl_if_exists stop "${CORE_SERVICES[@]}"

log "[7/10] Install systemd units"
install -m 0644 deploy/systemd/megavpn-migrate.service /etc/systemd/system/megavpn-migrate.service
install -m 0644 deploy/systemd/megavpn-api.service /etc/systemd/system/megavpn-api.service
install -m 0644 deploy/systemd/megavpn-worker.service /etc/systemd/system/megavpn-worker.service
install -m 0644 deploy/systemd/megavpn-agent.service /etc/systemd/system/megavpn-agent.service
systemctl daemon-reload

log "[8/10] Run migrations"
systemctl start megavpn-migrate.service

log "[9/10] Start services"
systemctl enable --now megavpn-api.service megavpn-worker.service
if [[ "$AGENT_WAS_ACTIVE" == "1" ]]; then
  systemctl restart megavpn-agent.service
else
  log "agent was not active before deploy; leaving it stopped"
fi

log "[10/10] Verify deployment"
sleep 2
run_health_check

echo
log "service status"
systemctl status "${SERVICES[@]/%/.service}" --no-pager -l || true

log "deployment completed"
