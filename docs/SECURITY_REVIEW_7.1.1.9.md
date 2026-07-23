# Security and Release Review: 7.1.1.9

**Release:** `7.1.1.9`

## Decision

This release closes an external egress lifecycle dead-end without weakening
runtime cleanup or routing isolation requirements. It is suitable for
production promotion after the documented release gates pass.

## Security Properties

- A deployment cannot be removed while active, failed, queued or while an
  external egress job is active.
- Runtime cleanup remains mandatory before the profile-to-node assignment can
  be removed.
- Reactivation is serialized by the existing deployment advisory lock.
- Reactivation updates desired state and queues the apply job in one database
  transaction.
- PostgreSQL runtime guards still reject conflicting L2TP/IPsec deployments.
- Provider credentials remain encrypted and are not returned by the removal
  endpoint.
- Removal is soft-delete based, preserving job and audit history.

## Failure Model

Concurrent cleanup, apply and removal requests are serialized per deployment.
An active job causes later conflicting operations to fail visibly. A failed
reactivation leaves the deployment present with its failure evidence and
requires cleanup before removal.

## Residual Risk

Reactivation still depends on node package repositories, provider
reachability and valid credentials. Those failures remain visible in agent job
evidence and do not cause the control plane to silently delete the deployment.

## Verification

Regression coverage exercises cleanup, inactive probe rejection, reactivation,
second cleanup, removal and recreation on the same profile/node. API status
mapping, full Go tests, JavaScript syntax checks, documentation consistency and
release self-tests form the release evidence.
