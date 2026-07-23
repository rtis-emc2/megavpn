#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/megavpn-ops-script-safety.XXXXXX")"
TMP_ROOT="$(cd "$TMP_ROOT" && pwd -P)"

cleanup() {
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT

fail() {
  printf '[ops-script-safety] ERROR: %s\n' "$*" >&2
  exit 1
}

expect_failure() {
  local label="$1"
  local expected="$2"
  shift 2
  if "$@" >"$TMP_ROOT/command.log" 2>&1; then
    fail "$label unexpectedly succeeded"
  fi
  if ! grep -Fq "$expected" "$TMP_ROOT/command.log"; then
    cat "$TMP_ROOT/command.log" >&2
    fail "$label failed without expected evidence: $expected"
  fi
}

cd "$ROOT_DIR"

expect_failure "Web install into filesystem root" "safe absolute directory" scripts/ops/install-web.sh /
expect_failure "Web install into protected system directory" "protected system directory" scripts/ops/install-web.sh /etc
expect_failure "Web install into source parent" "cannot contain the source tree" scripts/ops/install-web.sh "$ROOT_DIR"

expect_failure "Control Plane install into filesystem root" "install directory must be an absolute path" \
  env \
    MEGAVPN_CP_VALIDATE_ONLY=1 \
    MEGAVPN_CP_ASSUME_YES=1 \
    MEGAVPN_CP_TLS_MODE=self-signed-nginx \
    MEGAVPN_CP_DOMAIN=control.example.com \
    MEGAVPN_CP_PUBLIC_BASE_URL=https://control.example.com \
    MEGAVPN_CP_DATABASE_DSN='postgres://megavpn:password@127.0.0.1:5432/megavpn?sslmode=disable' \
    MEGAVPN_CP_ADMIN_PASSWORD='ops-script-safety-password' \
    MEGAVPN_CP_INSTALL_PACKAGES=0 \
    MEGAVPN_CP_APP_DIR=/ \
    scripts/ops/control-plane-install.sh

expect_failure "Control Plane install into protected system directory" "install directory cannot be a protected system directory" \
  env \
    MEGAVPN_CP_VALIDATE_ONLY=1 \
    MEGAVPN_CP_ASSUME_YES=1 \
    MEGAVPN_CP_TLS_MODE=self-signed-nginx \
    MEGAVPN_CP_DOMAIN=control.example.com \
    MEGAVPN_CP_PUBLIC_BASE_URL=https://control.example.com \
    MEGAVPN_CP_DATABASE_DSN='postgres://megavpn:password@127.0.0.1:5432/megavpn?sslmode=disable' \
    MEGAVPN_CP_ADMIN_PASSWORD='ops-script-safety-password' \
    MEGAVPN_CP_INSTALL_PACKAGES=0 \
    MEGAVPN_CP_APP_DIR=/etc \
    scripts/ops/control-plane-install.sh

fake_bin="$TMP_ROOT/bin"
mkdir -p "$fake_bin"
cat >"$fake_bin/pg_restore" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
: >"${MEGAVPN_TEST_PG_RESTORE_MARKER:?}"
EOF
cat >"$fake_bin/pg_dump" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
: >"${MEGAVPN_TEST_PG_DUMP_MARKER:?}"
EOF
chmod 0700 "$fake_bin/pg_restore" "$fake_bin/pg_dump"

safe_payload="$TMP_ROOT/safe"
safe_archive="$TMP_ROOT/safe.tar.gz"
mkdir -p "$safe_payload"
printf 'disposable database dump\n' >"$safe_payload/db.dump"
tar -C "$safe_payload" -czf "$safe_archive" .
restore_marker="$TMP_ROOT/pg-restore.called"
PATH="$fake_bin:$PATH" \
  MEGAVPN_TEST_PG_RESTORE_MARKER="$restore_marker" \
  MEGAVPN_DATABASE_DSN='postgres://restore.invalid/megavpn' \
  MEGAVPN_RESTORE_CONFIRM=1 \
  MEGAVPN_RESTORE_ARTIFACTS=0 \
  MEGAVPN_ARTIFACT_ROOT="$TMP_ROOT/restored-artifacts" \
  scripts/ops/restore.sh "$safe_archive" >"$TMP_ROOT/safe-restore.log"
[[ -f "$restore_marker" ]] || fail "safe backup did not reach pg_restore"

expect_failure "Restore into protected artifact directory" "artifact root cannot be a protected system path" \
  env \
    PATH="$fake_bin:$PATH" \
    MEGAVPN_TEST_PG_RESTORE_MARKER="$restore_marker" \
    MEGAVPN_DATABASE_DSN='postgres://restore.invalid/megavpn' \
    MEGAVPN_RESTORE_CONFIRM=1 \
    MEGAVPN_ARTIFACT_ROOT=/etc \
    scripts/ops/restore.sh "$safe_archive"

traversal_archive="$TMP_ROOT/traversal.tar.gz"
python3 - "$traversal_archive" <<'PY'
import io
import sys
import tarfile

with tarfile.open(sys.argv[1], "w:gz") as archive:
    payload = b"escape"
    entry = tarfile.TarInfo("../../megavpn-restore-escape")
    entry.size = len(payload)
    archive.addfile(entry, io.BytesIO(payload))
PY
rm -f "$restore_marker" "$TMP_ROOT/megavpn-restore-escape"
expect_failure "Traversal backup restore" "contains an unsafe path" \
  env \
    PATH="$fake_bin:$PATH" \
    MEGAVPN_TEST_PG_RESTORE_MARKER="$restore_marker" \
    MEGAVPN_DATABASE_DSN='postgres://restore.invalid/megavpn' \
    MEGAVPN_RESTORE_CONFIRM=1 \
    MEGAVPN_ARTIFACT_ROOT="$TMP_ROOT/restored-artifacts" \
    scripts/ops/restore.sh "$traversal_archive"
[[ ! -e "$restore_marker" ]] || fail "traversal archive reached pg_restore"
[[ ! -e "$TMP_ROOT/megavpn-restore-escape" ]] || fail "traversal archive escaped restore workspace"

absolute_archive="$TMP_ROOT/absolute.tar.gz"
python3 - "$absolute_archive" <<'PY'
import io
import sys
import tarfile

with tarfile.open(sys.argv[1], "w:gz") as archive:
    payload = b"absolute"
    entry = tarfile.TarInfo("/tmp/megavpn-restore-absolute")
    entry.size = len(payload)
    archive.addfile(entry, io.BytesIO(payload))
PY
rm -f "$restore_marker"
expect_failure "Absolute-path backup restore" "contains an unsafe path" \
  env \
    PATH="$fake_bin:$PATH" \
    MEGAVPN_TEST_PG_RESTORE_MARKER="$restore_marker" \
    MEGAVPN_DATABASE_DSN='postgres://restore.invalid/megavpn' \
    MEGAVPN_RESTORE_CONFIRM=1 \
    MEGAVPN_ARTIFACT_ROOT="$TMP_ROOT/restored-artifacts" \
    scripts/ops/restore.sh "$absolute_archive"
[[ ! -e "$restore_marker" ]] || fail "absolute-path archive reached pg_restore"

symlink_payload="$TMP_ROOT/symlink"
symlink_archive="$TMP_ROOT/symlink.tar.gz"
mkdir -p "$symlink_payload"
printf 'disposable database dump\n' >"$symlink_payload/db.dump"
ln -s /etc/passwd "$symlink_payload/linked-secret"
tar -C "$symlink_payload" -czf "$symlink_archive" .
rm -f "$restore_marker"
expect_failure "Symlink backup restore" "contains a link or special file" \
  env \
    PATH="$fake_bin:$PATH" \
    MEGAVPN_TEST_PG_RESTORE_MARKER="$restore_marker" \
    MEGAVPN_DATABASE_DSN='postgres://restore.invalid/megavpn' \
    MEGAVPN_RESTORE_CONFIRM=1 \
    MEGAVPN_ARTIFACT_ROOT="$TMP_ROOT/restored-artifacts" \
    scripts/ops/restore.sh "$symlink_archive"
[[ ! -e "$restore_marker" ]] || fail "symlink archive reached pg_restore"

hardlink_payload="$TMP_ROOT/hardlink"
hardlink_archive="$TMP_ROOT/hardlink.tar.gz"
mkdir -p "$hardlink_payload"
printf 'disposable database dump\n' >"$hardlink_payload/db.dump"
ln "$hardlink_payload/db.dump" "$hardlink_payload/db-copy.dump"
tar -C "$hardlink_payload" -czf "$hardlink_archive" .
rm -f "$restore_marker"
expect_failure "Hardlink backup restore" "contains a link or special file" \
  env \
    PATH="$fake_bin:$PATH" \
    MEGAVPN_TEST_PG_RESTORE_MARKER="$restore_marker" \
    MEGAVPN_DATABASE_DSN='postgres://restore.invalid/megavpn' \
    MEGAVPN_RESTORE_CONFIRM=1 \
    MEGAVPN_ARTIFACT_ROOT="$TMP_ROOT/restored-artifacts" \
    scripts/ops/restore.sh "$hardlink_archive"
[[ ! -e "$restore_marker" ]] || fail "hardlink archive reached pg_restore"

nested_payload="$TMP_ROOT/nested"
nested_artifacts="$TMP_ROOT/nested-artifacts"
nested_archive="$TMP_ROOT/nested.tar.gz"
mkdir -p "$nested_payload" "$nested_artifacts"
printf 'disposable database dump\n' >"$nested_payload/db.dump"
ln -s /etc/passwd "$nested_artifacts/linked-secret"
tar -C "$nested_artifacts" -czf "$nested_payload/artifacts.tar.gz" .
tar -C "$nested_payload" -czf "$nested_archive" .
rm -f "$restore_marker"
expect_failure "Symlink artifact archive restore" "artifact archive contains a link or special file" \
  env \
    PATH="$fake_bin:$PATH" \
    MEGAVPN_TEST_PG_RESTORE_MARKER="$restore_marker" \
    MEGAVPN_DATABASE_DSN='postgres://restore.invalid/megavpn' \
    MEGAVPN_RESTORE_CONFIRM=1 \
    MEGAVPN_ARTIFACT_ROOT="$TMP_ROOT/restored-artifacts" \
    scripts/ops/restore.sh "$nested_archive"
[[ ! -e "$restore_marker" ]] || fail "unsafe artifact archive reached pg_restore"

backup_artifacts="$TMP_ROOT/backup-artifacts"
backup_marker="$TMP_ROOT/pg-dump.called"
mkdir -p "$backup_artifacts"
ln -s /etc/passwd "$backup_artifacts/linked-secret"
expect_failure "Symlink artifact backup" "contains a link or special file" \
  env \
    PATH="$fake_bin:$PATH" \
    MEGAVPN_TEST_PG_DUMP_MARKER="$backup_marker" \
    MEGAVPN_DATABASE_DSN='postgres://backup.invalid/megavpn' \
    MEGAVPN_BACKUP_DIR="$TMP_ROOT/backups" \
    MEGAVPN_ARTIFACT_ROOT="$backup_artifacts" \
    scripts/ops/backup.sh
[[ ! -e "$backup_marker" ]] || fail "unsafe artifact tree reached pg_dump"

hardlink_backup_artifacts="$TMP_ROOT/hardlink-backup-artifacts"
hardlink_source="$TMP_ROOT/hardlink-source"
mkdir -p "$hardlink_backup_artifacts"
printf 'outside artifact root\n' >"$hardlink_source"
ln "$hardlink_source" "$hardlink_backup_artifacts/linked-file"
rm -f "$backup_marker"
expect_failure "Hardlink artifact backup" "contains a link or special file" \
  env \
    PATH="$fake_bin:$PATH" \
    MEGAVPN_TEST_PG_DUMP_MARKER="$backup_marker" \
    MEGAVPN_DATABASE_DSN='postgres://backup.invalid/megavpn' \
    MEGAVPN_BACKUP_DIR="$TMP_ROOT/backups" \
    MEGAVPN_ARTIFACT_ROOT="$hardlink_backup_artifacts" \
    scripts/ops/backup.sh
[[ ! -e "$backup_marker" ]] || fail "hardlink artifact tree reached pg_dump"

printf 'ops script safety smoke passed\n'
