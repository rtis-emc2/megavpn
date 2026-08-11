# Database Model

**Release:** `8.0.0-pre.2`

This document records the supported PostgreSQL ownership and integrity model.
It is the source of truth for schema changes and database reviews.

## Canonical Ownership

| Concern | Canonical tables | Runtime projection |
| --- | --- | --- |
| Client access groups | `client_access_groups`, `client_access_group_memberships`, `client_access_group_instance_scopes` | `service_accesses`, instance revisions and apply jobs |
| VLESS membership | Client access-group tables with `service_code='vless'` | Materialized Xray clients on every in-scope active instance |
| Client email attachments | `client_email_delivery_artifacts`, `client_email_delivery_share_links` | Mail job payload |
| Backhaul credentials | `backhaul_transport_secrets` | Encrypted material in `secret_refs` |
| External provider credentials | `external_egress_profile_secrets` | Encrypted material in `secret_refs` |

The retired `vless_group_templates`, `vless_group_memberships` and
`client_access_group_migration_conflicts` tables are not part of the supported
schema. `client_access_groups` is the only membership source of truth.

## Service Code Vocabularies

Two different vocabularies are intentional:

- `client_access_groups.service_code` is a logical client protocol identifier,
  such as `vless` or `l2tp`. It is validated as a stable identifier and is not a
  foreign key to the runtime catalog.
- `service_definitions.code` is a runtime driver identifier, such as
  `xray-core`, `xl2tpd` or `ipsec`.
- `client_service_identities.service_code` belongs to the runtime vocabulary and
  therefore references `service_definitions(code)`.

Do not introduce a direct logical-protocol-to-runtime-driver foreign key. One
logical protocol may require multiple runtime drivers.

## Integrity Guarantees

- Instance current/applied revision references are foreign keys and ownership
  triggers reject a revision belonging to another instance.
- Audit actors reference `platform_users` and become `NULL` when the user row is
  deliberately removed; the audit event itself remains.
- Email attachment join tables use composite client ownership foreign keys, so
  an email cannot attach another client's artifact or share link.
- Backhaul and external-egress secret relationships use typed join tables with
  foreign keys. Secret values remain encrypted in `secret_refs`.
- `service_definitions.updated_at` is maintained by a database trigger.

Migration `000028_schema_normalization` records counts and bounded diagnostic
samples of unresolved historical JSON references in `audit_events` before
removing the legacy JSON columns. Samples contain identifiers, not secret
material. New writes are always protected by relational constraints.

## Node History And Retention

Application lifecycle removal is a soft retirement:

1. Active workloads and jobs are reconciled or force-retired explicitly.
2. The node becomes `status='retired'`.
3. Traffic accounting and inventory history remain available for their normal
   retention period.

PostgreSQL blocks physical `DELETE FROM nodes` by default because cascades would
otherwise remove retained traffic and inventory evidence. A permanent purge is
an exceptional, audited maintenance operation. Inside its explicit transaction,
an authorized DBA must first run:

```sql
set local megavpn.allow_node_delete = 'on';
```

The application does not set this override.

## Address Families

Managed address pools are intentionally IPv4-only in this release.
Firewall IPv6 matching does not imply IPv6 tunnel address allocation support.
Adding IPv6 pools requires allocator, overlap, route projection, client artifact
and live data-plane coverage before extending `address_pool_spaces.family`.

## Naming Rules For New Migrations

- Prefer descriptive booleans without an `is_` prefix: `enabled`,
  `routing_enabled`, `auto_apply_new_instances`.
- Do not rename existing stable columns only for style consistency.
- Use `timestamptz` for timestamps, native `inet`/`cidr` for addresses where the
  value is not an opaque provider string, and `bigint` for byte/packet counters.
- Stable entity relationships require foreign keys and join tables. JSONB is for
  extensible payload/config metadata, not UUID relationship arrays.
- Every foreign key used for lookup or cleanup requires a supporting index.

## Verification

On a disposable empty PostgreSQL database:

```bash
MEGAVPN_TEST_DATABASE_DSN='postgres://...' scripts/ci/postgres-migration-drill.sh
```

On an upgraded environment:

```bash
MEGAVPN_DATABASE_DSN='postgres://...' scripts/ops/database-integrity-audit.sh
MEGAVPN_DATABASE_DSN='postgres://...' scripts/ops/database-integrity-audit.sh --validate-constraints
```

The second command is safe only after the first reports zero critical rows.
