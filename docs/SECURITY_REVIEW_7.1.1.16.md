# Security and Release Review: 7.1.1.16

**Release:** `7.1.1.16`

## Decision

This patch closes destructive-script path and archive trust gaps. No unresolved
critical or high-severity finding was identified in the changed surface.

## Reviewed Surface

- Shell entrypoints and compatibility wrappers.
- Web UI installation with `rsync --delete`.
- Control Plane installation directories.
- PostgreSQL backup and restore lifecycle.
- Artifact and optional configuration archives.
- Release and documentation consistency gates.

## Security Properties

- Destructive directory targets must be explicit safe absolute paths.
- Filesystem root and protected system roots are never accepted as direct
  install or restore directories.
- Web UI source and target trees cannot contain each other.
- Backup rejects symbolic links, hardlinked regular files, device nodes,
  sockets and FIFOs before database dump or archive creation.
- Restore copies the supplied archive into a private workspace before
  inspection and extraction.
- The outer and nested artifact archives allow only regular files and
  directories and reject absolute paths and parent traversal.
- All archive validation completes before destructive `pg_restore`.
- Archive ownership and permissions are not restored.
- CI and release self-test exercise the negative paths on every change.
- Web UI cache keys must match the current release for all supported
  four-component versions.

## Residual Risk

Operational scripts run with elevated privileges and rely on the integrity of
the host filesystem, `tar`, PostgreSQL client tools and root-controlled
environment files. A root-level host compromise remains outside this boundary.
Archive metadata parsing is intentionally restrictive; backups requiring links
or special files are unsupported rather than silently weakened.

## Verification

The shell and JavaScript audits cover all repository script entrypoints and
compatibility wrappers. Safety smoke covers unsafe install roots, source/target
overlap, absolute and parent-traversal archive paths, outer and nested links,
protected restore targets and unsafe backup source trees. Go tests, race tests,
static security checks, documentation consistency and the full release
self-test are required before tag publication.
