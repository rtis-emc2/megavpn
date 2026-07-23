# Operations Runbook

**Release:** `7.1.1.10`

## Deployment Model

Production topology:

- `megavpn-api` listens on `127.0.0.1:8080`.
- Nginx terminates public HTTPS and forwards to API.
- `megavpn-worker` runs queued control-plane jobs.
- PostgreSQL runs on a trusted host or managed service with TLS.
- Node agents connect outbound to the public control-plane URL.
- Artifact root is persistent storage under `/var/lib/megavpn/artifacts`.

Use:

- `deploy/env/megavpn.production.env.example`
- `deploy/env/megavpn-agent.production.env.example`
- `deploy/systemd/*.service`
- `deploy/nginx/megavpn-web.conf`

## Pre-Deploy Checklist

1. Run `scripts/ci/release-gate.sh`.
2. Run migrations against a disposable DB using `MEGAVPN_RELEASE_DATABASE_DSN`.
3. Verify `nginx -t` on the target edge host.
4. Verify `systemd-analyze verify deploy/systemd/*.service` on a systemd host.
5. Create or verify `/etc/megavpn/master.key` with mode `0600`.
6. Confirm `MEGAVPN_AGENT_SIGNATURE_ENFORCE=true`.
7. Confirm `MEGAVPN_AGENT_ALLOW_AUTO_REGISTER=false`.
8. Confirm `MEGAVPN_PUBLIC_BASE_URL` exactly matches the public TLS endpoint.
9. Confirm control-plane, worker and node hosts have synchronized UTC clocks.
   The signed agent channel rejects requests outside its replay-protection
   window, and SSH bootstrap blocks nodes whose clock differs by more than two
   minutes.

## Agent Clock Synchronization

The agent protocol signs requests and responses with a Unix timestamp. A node
whose clock differs from the control plane by more than five minutes is rejected
even when its enrollment and agent token are valid. Do not increase the
signature window or disable signature enforcement to work around clock drift.

Run on the control-plane host and the affected node:

```bash
date -u +'%s %FT%T.%3NZ'
timedatectl status
sudo timedatectl set-ntp true
sudo systemctl restart systemd-timesyncd
timedatectl timesync-status
```

For hosts managed by chrony:

```bash
sudo chronyc makestep
chronyc tracking
```

Compare `date -u +%s` on both hosts. After the clocks converge, restart the node
agent:

```bash
sudo systemctl restart megavpn-agent
sudo journalctl -u megavpn-agent -n 50 --no-pager -l
```

Pass criteria:

- the UI reports a fresh heartbeat and no newer `auth_failure`;
- the journal no longer contains `timestamp is outside allowed window`;
- queued node jobs are claimed without re-enrollment;
- SSH bootstrap evidence includes `clock_skew_seconds` within two minutes.

## Repository History Rewrite

Use this only for an approved release maintenance window. Rewriting `main` changes
commit identities, breaks old clones, invalidates old tag pointers and can confuse
automation that pins commits.

Required controls:

1. Freeze deploys and merges.
2. Create an offline mirror backup of the remote repository.
3. Confirm the working tree contains only reviewed release content.
4. Run `scripts/ci/release-gate.sh` and the clean-install checklist.
5. Create a clean import commit from the reviewed tree.
6. Move the release tag to the clean import commit.
7. Force-update the protected branch only with `--force-with-lease`.
8. Verify remote `main`, the release tag and deployed artifacts point to the same
   release version.

Operator command outline:

```bash
git clone --mirror <remote-url> megavpn-history-backup.git
git status --short
git checkout --orphan release-clean
git add -A
git commit -m "Release 7.1.1.10 clean import"
git tag -f v7.1.1.10
git push --force-with-lease origin release-clean:main
git push --force-with-lease origin v7.1.1.10
```

Recovery plan:

1. Stop deployment automation.
2. Push the backed-up `main` reference from the mirror if rollback is required.
3. Recreate old tags from the mirror.
4. Notify operators to reclone or hard-reset their local checkout to the new
   branch tip after the final decision.

## Backup

Standard backup:

```bash
sudo MEGAVPN_DATABASE_DSN='postgres://...' \
  MEGAVPN_BACKUP_DIR=/var/backups/megavpn \
  MEGAVPN_ARTIFACT_ROOT=/var/lib/megavpn/artifacts \
  MEGAVPN_MASTER_KEY_PATH=/etc/megavpn/master.key \
  scripts/ops/backup.sh
```

The master key is not included by default. Store its SHA-256 and sealed copy separately. Losing the key makes encrypted secrets, certificates and bootstrap material unrecoverable.

## Restore Drill

Never drill restore into production. Use a separate disposable database:

```bash
MEGAVPN_RESTORE_CONFIRM=1 \
MEGAVPN_DATABASE_DSN='postgres://restore-target...' \
MEGAVPN_ARTIFACT_ROOT=/tmp/megavpn-artifacts-restore \
scripts/ops/restore.sh /var/backups/megavpn/megavpn-backup-YYYYmmdd-HHMMSS.tar.gz
```

Pass criteria:

- `pg_restore` completes.
- API starts against the restored DB.
- Artifact preview/download works for restored artifacts.
- Audit log and jobs are readable.

## Secret Rotation

### Master Key

Current implementation supports versioned master-key metadata, but bulk re-encryption must be treated as a controlled maintenance task.

Procedure:

1. Freeze bootstrap, certificate import and provisioning writes.
2. Take DB and artifact backup.
3. Generate new key file with `scripts/ops/generate-master-key.sh`.
4. Re-encrypt `secret_refs` in a controlled migration/tooling window.
5. Update `MEGAVPN_MASTER_KEY_VERSION`.
6. Restart API/worker.
7. Verify secret-backed operations: agent token rotation, certificate apply, client provisioning.

### Agent Tokens

Use the typed node token rotation flow. Pass criteria:

- New token is stored as hash plus hint.
- Agent accepts the new signed channel.
- Old token no longer validates.
- Audit event records the rotation request/result.

### Share Links

Share-link plaintext tokens are not recoverable after creation. To rotate a share link, revoke it and publish a new one.

## Incident Response

### Suspected Platform Credential Compromise

1. Disable affected user or rotate password.
2. Revoke active sessions if exposed.
3. Review audit events for settings, bootstrap, apply, capability and share-link actions.
4. Rotate affected service secrets and agent tokens.
5. Preserve DB backup and relevant logs before cleanup.

### Suspected Agent Token Compromise

1. Put node in maintenance.
2. Rotate agent token through typed node flow.
3. Review jobs claimed by that node.
4. Reapply critical runtime state from known-good revisions.
5. Verify signed heartbeat/job result after rotation.

### Public Share Link Leak

1. Revoke the share link.
2. Check download count and audit event.
3. Rebuild client artifacts if the downloaded material is sensitive.
4. Notify affected client owner.

## Upgrade

1. Backup DB/artifacts and seal master-key checksum.
2. Run release gate on the target commit.
3. Stop worker first to drain job creation/claim races.
4. Deploy binaries and web assets.
5. Run migrations.
6. Start API, then worker.
7. Update agents through SSH bootstrap or package rollout.
8. Verify `/api/v1/runtime/preflight` is `ready` and `/api/v1/ready` succeeds with `MEGAVPN_PRODUCTION_MODE=true`.
9. Verify agent version drift is zero or explicitly accepted.

## Rollback

Rollback must be version-specific. Minimum rollback plan:

1. Stop worker.
2. Stop API.
3. Restore previous binaries and web assets.
4. If migrations are not backward-compatible, restore DB from pre-upgrade backup.
5. Restore artifacts if artifact format changed.
6. Start API, then worker.
7. Verify `/health`, `/api/v1/ready`, node heartbeat and job queue. In production mode, `/api/v1/ready` must fail if runtime preflight is degraded or blocked.

Do not roll back only the API binary after schema changes unless the release notes explicitly state schema backward compatibility.
