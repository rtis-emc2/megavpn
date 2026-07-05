# VLESS Subscriptions

**Release:** `7.0.1.25`

VLESS subscriptions provide a live per-client profile feed for compatible
client applications. They are an operator delivery mechanism for already
provisioned VLESS access, not a replacement for provisioning, access groups or
route policy.

Russian companion: [VLESS_SUBSCRIPTIONS_RU.md](VLESS_SUBSCRIPTIONS_RU.md).

## Security Model

| Item | Behavior |
| --- | --- |
| Token type | Bearer token in the subscription URL. Anyone with the URL can fetch the feed until expiry or revocation. |
| Storage | The database stores `token_hash` and `token_hint`; plaintext is shown only once after rotation. |
| Rotation | Rotating a subscription revokes currently active subscription tokens for the same client. |
| Revocation | Operators can revoke tokens from `Clients -> Access -> VLESS Subscription`. |
| Expiry | Tokens have an explicit TTL. Expired tokens are rejected and marked expired on read. |
| Cache control | Public responses use `Cache-Control: no-store`. |
| Audit | Rotation and revocation create audit events. |

## Data Flow

1. Operator provisions a client and explicitly selects inbound services.
2. Provisioning creates `service_accesses` and service-specific metadata.
3. For VLESS, provisioning stores the generated client UUID in access metadata.
4. Operator opens client access and rotates a VLESS subscription token.
5. The UI shows the full subscription URL once.
6. The public endpoint resolves the bearer token, checks client/subscription
   status and builds a feed from current active VLESS service accesses.

The public endpoint is:

```text
GET /subscribe/vless/{token}
```

By default it returns a base64-encoded newline-separated URI list. For
debugging, operators can append:

```text
?format=plain
```

## Profile Eligibility

A service access is included in the feed only when all conditions are true:

- the client is active and not expired;
- the subscription token is active and not expired;
- the service access status is `active`;
- the instance service code is `xray-core`;
- the instance is enabled and active;
- the access metadata contains the generated VLESS UUID.

If provisioning has not completed yet, the profile is skipped instead of
generating a new secret during subscription download.

## Operator Workflow

1. Open `Clients`.
2. Create or select a client.
3. Run `Provision` and select one or more VLESS inbound instances.
4. Wait for the provisioning job to complete.
5. Open `Access`.
6. In `VLESS Subscription`, click `Rotate subscription`.
7. Copy the generated URL immediately.
8. Send the URL to the user through the approved delivery channel.

If the URL is lost, rotate again. The old token is revoked and a new URL is
generated.

## Failure Scenarios

| Symptom | Likely cause | Action |
| --- | --- | --- |
| Feed is empty | No active VLESS service access or provisioning did not complete | Re-run provisioning and confirm VLESS access metadata exists. |
| `subscription token is not active` | Token was revoked | Rotate a new token. |
| `subscription token has expired` | TTL elapsed | Rotate a new token. |
| `client is not active` | Client status changed | Restore client status only after policy review. |
| Client imports profile but traffic exits wrong node | Instance egress/backhaul policy, not subscription delivery | Check `Instances -> Manage`, backhaul health and access groups. |

## Maintenance Notes

- Do not store plaintext subscription tokens in logs, tickets or audit payloads.
- Keep subscription TTLs short for temporary access.
- Prefer client revocation over only subscription revocation when access must be
  terminated completely.
- Back up subscription rows with the database; they contain hashes only, but
  they are still operational state.
