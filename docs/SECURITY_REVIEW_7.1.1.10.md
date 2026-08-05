# Security and Release Review: 7.1.1.10

**Release:** `7.1.1.10`

## Decision

This release corrects dependency recovery for an existing privileged runtime
installation path without expanding the package or command allowlist. It is
suitable for production promotion after the documented release gates pass.

## Security Properties

- Runtime installation remains restricted to the existing fixed package names
  and command argument sets.
- No provider-controlled value is interpolated into apt or dpkg commands.
- Package service auto-start suppression remains active throughout repair and
  retry operations.
- Every repair command records its arguments, exit code and bounded output in
  job evidence.
- Package commands retain the existing execution timeout, apt lock timeout and
  noninteractive configuration.
- Existing external egress runtime is not replaced until capability
  installation and verification succeed.
- Profile credentials remain encrypted and are not involved in package repair.

## Failure Model

An unresolved apt dependency or repository failure leaves the deployment job
failed with the last command and package-manager output. A failed first
`dpkg --configure -a` no longer hides the recoverable dependency path; a failed
`apt-get -f install` still stops the operation explicitly.

## Residual Risk

The node package database and configured Ubuntu repositories remain external
dependencies. Maintainer scripts supplied by the operating-system repository
run with root privileges as part of normal package installation. Operators
must use trusted, authenticated repositories and monitor repository drift.

## Verification

Regression tests reproduce an XL2TPD dependency failure, assert the recovery
command order and verify explicit failure reporting when dependency repair
cannot complete. Full release gates provide the remaining evidence.
