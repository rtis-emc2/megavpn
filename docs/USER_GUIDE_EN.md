# User Guide

**Release:** `7.1.0.15`

This document describes the full RTIS MegaVPN operator workflow: installing the
Control Plane on a clean host, configuring the platform, enrolling nodes,
creating runtime capabilities, service instances, backhaul links, clients and
client artifacts.

## 1. Core Concepts

| Term | Meaning |
| --- | --- |
| Control Plane | Central API/UI that stores state and orchestrates infrastructure. |
| Node | Server that runs VPN, proxy or edge services. |
| Agent | Node-side process that receives jobs, applies configs and reports runtime state. |
| Ingress node | Node that accepts client connections. |
| Egress node | Node through which client traffic should leave. |
| Service | Runtime type: OpenVPN, WireGuard, Xray/VLESS, Shadowsocks, Nginx and others. |
| Service pack | Template that creates one or more service instances with safe defaults. |
| Instance | Concrete service on a concrete node: endpoint, spec, revision and runtime state. |
| Revision | Desired config version for an instance. Only apply-ready revisions should be applied. |
| Runtime capability | Required binary/package on a node, for example `openvpn`, `xray`, `ss-server`. |
| Backhaul | Managed ingress-to-egress link for remote traffic exit. |
| Client | Client account that receives selected inbound services. |
| Artifact | Generated client config or bundle. |
| Share link | Temporary artifact URL. The plaintext token is shown only once. |
| VLESS subscription | Per-client bearer URL that returns current active VLESS profiles. The plaintext token is shown only once after rotation. |

## 2. Prepare The Control Plane Host

Minimum production model:

- Ubuntu/Linux host with systemd.
- PostgreSQL database reachable by the Control Plane.
- Public HTTPS endpoint.
- Nginx as TLS reverse proxy.
- Go toolchain for building from a source checkout.
- Persistent storage for `/var/lib/megavpn/artifacts`.
- Secret master key stored outside database backups.

Base system dependencies:

```bash
sudo apt-get update
sudo apt-get install -y git curl rsync openssl ca-certificates nginx postgresql-client
```

If PostgreSQL runs on the same host, create the database and user separately.
For production, prefer a TLS DSN with certificate verification. `sslmode=disable`
is suitable only for lab or trusted local-only PostgreSQL.

## 3. Install The Control Plane

The recommended path is `scripts/control-plane-install.sh`. The installer
performs the full bootstrap:

- validates parameters;
- optionally installs base apt packages;
- syncs the source tree into `/opt/megavpn`;
- creates `/etc/megavpn/megavpn.env`;
- creates or preserves `/etc/megavpn/master.key`;
- builds binaries;
- installs the Web UI;
- installs systemd units;
- runs migrations;
- starts API and worker;
- creates a local HTTPS edge in `self-signed-nginx` mode;
- performs a health check.

Interactive run:

```bash
sudo ./scripts/control-plane-install.sh
```

Example non-interactive run:

```bash
sudo MEGAVPN_CP_ASSUME_YES=1 \
  MEGAVPN_CP_TLS_MODE=self-signed-nginx \
  MEGAVPN_CP_PUBLIC_BASE_URL=https://control.example.com \
  MEGAVPN_CP_DATABASE_DSN='postgres://megavpn:password@127.0.0.1:5432/megavpn?sslmode=disable' \
  MEGAVPN_CP_ADMIN_USERNAME=superadmin \
  MEGAVPN_CP_ADMIN_EMAIL=admin@control.example.com \
  ./scripts/control-plane-install.sh
```

Validate the same inputs without changing the host:

```bash
sudo MEGAVPN_CP_VALIDATE_ONLY=1 \
  MEGAVPN_CP_ASSUME_YES=1 \
  MEGAVPN_CP_TLS_MODE=self-signed-nginx \
  MEGAVPN_CP_PUBLIC_BASE_URL=https://control.example.com \
  MEGAVPN_CP_DATABASE_DSN='postgres://megavpn:password@127.0.0.1:5432/megavpn?sslmode=disable' \
  MEGAVPN_CP_ADMIN_PASSWORD='replace-this-before-real-install' \
  ./scripts/control-plane-install.sh
```

Key install variables:

| Variable | Purpose |
| --- | --- |
| `MEGAVPN_CP_PUBLIC_BASE_URL` | Public URL used by browsers and agents. |
| `MEGAVPN_CP_TLS_MODE` | `self-signed-nginx`, `external-https` or lab-only `http-direct`. |
| `MEGAVPN_CP_DATABASE_DSN` | PostgreSQL DSN. |
| `MEGAVPN_CP_APP_DIR` | Install directory, default `/opt/megavpn`. |
| `MEGAVPN_CP_ENV_FILE` | Runtime env file, default `/etc/megavpn/megavpn.env`. |
| `MEGAVPN_CP_MASTER_KEY_PATH` | Secret master key path. |
| `MEGAVPN_CP_ARTIFACT_ROOT` | Persistent artifact storage. |
| `MEGAVPN_CP_ADMIN_USERNAME` | Bootstrap admin username. |
| `MEGAVPN_CP_ADMIN_EMAIL` | Bootstrap admin email. |
| `MEGAVPN_CP_ADMIN_PASSWORD` | Bootstrap admin password; generated when empty. |
| `MEGAVPN_CP_RUN_TESTS` | Run `go test ./...` during installation. |
| `MEGAVPN_CP_VALIDATE_ONLY` | Validate inputs and exit before host changes. |
| `MEGAVPN_CP_GO_TARBALL_URL` | Optional pinned Go toolchain tarball URL when the host Go version is too old. |
| `MEGAVPN_CP_GO_TARBALL_SHA256` | Required SHA-256 pin when `MEGAVPN_CP_GO_TARBALL_URL` is used. |

Runtime GeoIP variables in `/etc/megavpn/megavpn.env`:

| Variable | Purpose |
| --- | --- |
| `MEGAVPN_GEOIP_LOOKUP_URL_TEMPLATE` | HTTPS GeoIP URL template with `{ip}` placeholder; set to `disabled` to turn off automatic node map lookup. |
| `MEGAVPN_GEOIP_TIMEOUT` | Per-request timeout for GeoIP lookup. |
| `MEGAVPN_GEOIP_AUTO_ENRICH_LIMIT` | Maximum nodes enriched during one API list request. |

The installer verifies that the Go toolchain satisfies `go.mod`. If the host Go
version is too old, either allow the installer to use the OS package manager or
provide a pinned tarball URL plus SHA-256. Unpinned toolchain downloads are
rejected.

If the installer generates a password, it stores it in a root-only file:

```bash
sudo cat /root/megavpn-control-plane-admin.txt
```

After first successful login and operator creation, remove the bootstrap password
from runtime environment or replace the env file with a version that does not
include `MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD`, then restart the API. Bootstrap env
does not reset existing users, but keeping the password in env longer than
needed is undesirable.

## 4. Manual Installation

Use the manual path for controlled production environments where the installer
must not install packages or write Nginx config.

1. Copy the source tree to `/opt/megavpn`:

```bash
sudo install -d -m 0755 /opt/megavpn
sudo rsync -a --delete ./ /opt/megavpn/
cd /opt/megavpn
```

2. Create env:

```bash
sudo install -d -m 0750 /etc/megavpn
sudo install -m 0600 deploy/env/megavpn.production.env.example /etc/megavpn/megavpn.env
sudo editor /etc/megavpn/megavpn.env
```

3. Create the master key:

```bash
sudo MEGAVPN_MASTER_KEY_PATH=/etc/megavpn/master.key scripts/generate-master-key.sh
```

4. Build binaries and Web UI. `scripts/build.sh` must run from
   `/opt/megavpn`, so the binaries are created in `/opt/megavpn/bin`:

```bash
./scripts/build.sh
sudo ./scripts/install-web.sh /opt/megavpn/web
```

5. Install systemd units:

```bash
sudo install -m 0644 deploy/systemd/megavpn-*.service /etc/systemd/system/
sudo systemctl daemon-reload
```

6. Run migrations:

```bash
sudo systemctl start megavpn-migrate.service
sudo systemctl status megavpn-migrate.service --no-pager -l
```

7. Start API and worker:

```bash
sudo systemctl enable --now megavpn-api.service megavpn-worker.service
sudo systemctl status megavpn-api.service megavpn-worker.service --no-pager -l
```

8. Configure Nginx reverse proxy. Base example:
   `deploy/nginx/megavpn-web.conf`.

```bash
sudo install -m 0644 deploy/nginx/megavpn-web.conf /etc/nginx/conf.d/megavpn-web.conf
sudo editor /etc/nginx/conf.d/megavpn-web.conf
```

Before enabling it, replace `server_name`, certificate paths and
`X-Forwarded-Port`. Keep the template `Upgrade`/`Connection` headers: they are
required for the WebSocket terminal and long-lived browser connections.

9. Verify:

```bash
sudo nginx -t
sudo systemctl reload nginx
curl -fsS http://127.0.0.1:8080/healthz
```

## 5. Post-Install Validation

After installation, verify:

```bash
sudo systemctl status megavpn-api megavpn-worker --no-pager -l
sudo journalctl -u megavpn-api -u megavpn-worker -n 120 --no-pager
curl -fsS http://127.0.0.1:8080/healthz
```

In the UI, verify:

1. Login works.
2. Dashboard opens without 500 errors.
3. `Settings -> Control Plane TLS` contains the correct public URL.
4. `/api/v1/ready` reports `ready` only when production preflight is correct.
5. `Jobs`, `Nodes`, `Services`, `Instances`, `Clients`, `Backhaul` and
   `Certificates` open without errors.
6. `Instances` shows the workspace tabs: instance list, create from pack,
   manual instance, service-pack catalog and VLESS groups.
7. `Instances -> Create from pack` shows the service-pack catalog. Default
   templates are created by the ordered migration set; if the list is empty,
   verify that every migration has run against the same database used by the
   API.
8. `Instances -> VLESS groups` shows default groups for default route, current
   node exit, ad-blocked default and blocked access.

If the installer used self-signed TLS, replace it through:

1. `Certificates -> Add certificate`.
2. `Settings -> Control Plane TLS`.
3. Select imported/managed certificate.
4. `Apply edge`.
5. Verify `nginx -t` and the public HTTPS URL.

## 6. Initial System Configuration

Before adding production nodes, configure:

- SMTP settings if invite/email delivery is required.
- Artifact root and backup policy.
- Secret master key backup policy.
- Operator roles and least-privilege permissions.
- Control Plane TLS profile.
- Runtime binary repository for services that cannot be installed from OS repo.
- Address pools for OpenVPN/WireGuard/client networks.

Production defaults:

- `MEGAVPN_PRODUCTION_MODE=true`;
- `MEGAVPN_AGENT_ALLOW_AUTO_REGISTER=false`;
- `MEGAVPN_AGENT_SIGNATURE_ENFORCE=true`;
- `MEGAVPN_TRUST_PROXY_HEADERS=true` only behind a trusted reverse proxy;
- API listens on loopback and public access goes through HTTPS edge.

## 7. First Sign-In And Readiness

1. Open the public HTTPS Control Plane URL.
2. Sign in with an operator account.
3. Check the top-right status:
   - `ready` means API runtime preflight is healthy;
   - degraded/blocked requires checking Settings, Jobs or Runtime preflight.
4. Open Dashboard and verify that API, Jobs and Nodes render without 500 errors.

## 8. Add A Node

1. Open `Nodes`.
2. Select `Add node`.
3. Set:
   - human-readable name;
   - role: `ingress`, `egress` or a runtime-specific role;
   - public or management address;
   - setup method.
4. For SSH bootstrap, add an SSH access method:
   - `ssh_user`;
   - `ssh_host`;
   - `ssh_port`;
   - `ssh_host_key_sha256`;
   - private key secret.
5. Start bootstrap or enrollment.
6. Wait for heartbeat: the node should become `online`.

`ssh_host_key_sha256` protects bootstrap from MITM. It must match the real node
host key fingerprint.

After agent reinstall or host repair, use `Nodes -> Node -> Runtime reconcile`
to queue desired-state recovery for managed services, backhaul, route policy and
the existing firewall policy. Use `Reboot node` only for a controlled
maintenance window; it is executed by the enrolled agent, requires typing the
node name and records the reboot job in audit/job history.

## 9. Runtime Capabilities

Before applying a service instance, the node must have the required runtime:

- OpenVPN: `openvpn`;
- WireGuard: `wg`, `wg-quick`;
- Xray/VLESS: `xray`;
- Shadowsocks: `ss-server`;
- Nginx edge: `nginx`.

Workflow:

1. Open `Services`.
2. Select the target node.
3. Select `Verify` to read the actual state.
4. If the runtime is missing, use `Install runtime` or the runtime binary
   repository flow.
5. Run `Verify` again after installation.

If a package cannot be safely installed from OS repositories, upload the runtime
to `Runtime Binary Repository`. The Control Plane stores the artifact, calculates
SHA-256 and later issues a short-lived signed node download ticket to the agent.

## 10. Certificates And PKI

There are two different certificate classes:

| Type | Used by |
| --- | --- |
| TLS edge certificate | Nginx/Xray TLS-facing endpoints, public HTTPS/SNI. |
| Service CA profile | OpenVPN CA root for server/client certificates. |

For an OpenVPN fleet where multiple ingress nodes serve one shared endpoint, use
one shared OpenVPN CA profile. Client certificates will trust the same CA, and
instance server certificates will be issued from that same service CA.

The managed certificate selector in service-pack forms is for TLS-facing
components: Nginx edge or Xray TLS. OpenVPN uses the Service CA profile, not the
TLS edge certificate.

## 11. Address Pools

Address pools should be centralized. Operators should not need to remember which
subnet is free.

Operational model:

1. Configure a base range in address pools.
2. The system allocates free subnets for OpenVPN, WireGuard and service
   instances.
3. If no free subnet exists, the UI must show a clear action: add or expand a
   pool.
4. Inter-pool routing can be enabled or blocked by policy.

Default pool `remote_access_v4` is created by the current migration set.
Pack/manual specs with `address_pool_mode=auto` receive a free subnet from the
catalog. Use manual CIDR only as an intentional override; active allocations
lock pool deletion.

## 12. Managed Backhaul

Backhaul is required when the ingress node accepts traffic but traffic must exit
through an egress node.

Workflow:

1. Open `Backhaul`.
2. Create a link: ingress node -> egress node.
3. Select `Active backhaul transport`: this is the active node-to-node transport
   used for apply, health checks and route projection. It is not a client VPN
   protocol selector.
4. In `Optional standby transports`, leave extra transports unchecked unless you
   want generated fallback profiles such as OpenVPN for later promotion or
   diagnostics.
5. Select `Apply`.
6. Wait for successful apply on both sides.
7. If the active transport is unhealthy but a standby is `standby ready`, open
   `Manage` and select `Promote to active` on the standby transport.
8. Select `Test`.
9. Verify:
   - both sides are `healthy`;
   - packet loss is `0`;
   - latency is visible;
   - route lookup uses the managed interface.

Backhaul apply and service instance apply are separate operations. Backhaul
creates node-to-node transport. Instance route policy uses that transport for
client traffic exit.

When several active backhaul links exist from the same ingress node to the same
egress node, they form a failover set. The control plane orders candidates by
`route_metric`: lower metric is preferred, higher metric remains as backup. The
agent installs all candidates in the selected policy table and refreshes the
kernel routes on a systemd timer. If a candidate interface disappears or its
peer probe fails, only that candidate route is removed; the next metric remains
available for traffic.

### 12.1 Node Map

Open `Node Map` to inspect approximate node placement and managed backhaul
overlay. Node coordinates, country, city, ASN and network owner are resolved
automatically from the public node address through GeoIP. Operators do not enter
map coordinates manually.

Use the map for topology orientation:

- node pins show resolved GeoIP placement and node role/status;
- node cards show country, city, network owner, GeoIP source and related
  backhaul links;
- backhaul lines show drawable ingress-to-egress links when both endpoint nodes
  have coordinates;
- the topology list below the map shows every non-deleted backhaul link with
  direction, driver, metric, endpoint and selected transport status.

Use `Backhaul` for apply/probe/cleanup and transport diagnostics. Node Map is a
visual topology view.

## 13. Create Service Instances

There are two supported paths.

### 13.1 Create From Pack

Use this for standard production baselines.

1. Open `Instances`.
2. Select `Create from pack`.
3. Select a service pack.
4. In `Services to create`, keep only the components that must be created.
   Each selected component becomes one instance; unchecked components are not
   created, do not require runtime preflight and do not submit their per-service
   overrides. Use the per-component `Listen port` field when the pack default
   port would conflict on the selected node. For OpenVPN components, either use
   the pack-level OpenVPN CA profile or override it on the selected component.
5. Select a node.
6. Set base name and endpoint host.
7. Select a TLS edge certificate if the selected components include Nginx/Xray
   TLS.
8. Select an OpenVPN CA profile if the selected components include OpenVPN.
9. If the selected components include traffic camouflage, configure
   `Traffic camouflage`:
   - `Fallback website` is required and must be an absolute `http://` or
     `https://` URL for the real site. Its host must not be the same as the
     public ingress endpoint;
   - when `Hidden VLESS path` is shown, it must not be `/`, must not contain a
     query/fragment and should look like an ordinary asset/API path;
   - `Fallback Host header` and `Fallback SNI` can be left empty: the control
     plane derives them from the fallback URL. If they are set explicitly, they
     must not point back to the ingress endpoint.
10. If the selected components include VLESS, select instance-level
    `VLESS routing`:
   - `Auto through managed backhaul` for a single unambiguous active backhaul;
   - `Use selected egress node` when the VLESS instance must exit through a
     specific remote egress node;
   - `Local breakout on ingress node` only when direct exit from the ingress
     node is intended.
11. Create the instances.
12. Select `Apply` or `Install + apply` if runtime is missing.

Service packs must not store runtime secrets. Passwords, private keys, UUIDs,
Reality keys and similar secrets should be generated during revision/apply and
stored as secret refs.

Default OpenVPN packs are full-tunnel baselines: they push `redirect-gateway`
and public DNS to clients. Apply also materializes managed node network policy
for IP forwarding and nftables `postrouting` masquerade from the OpenVPN client
pool. If a site needs split-tunnel OpenVPN, remove the redirect push lines and
explicitly review `nat_rules` before applying the revision.

Traffic camouflage packs create two instances: an Nginx public TLS edge and an
Xray backend bound to `127.0.0.1`. Nginx proxies only the hidden VLESS/gRPC path
to Xray, while ordinary browser traffic on `/` is reverse-proxied to the
fallback website. This masks ingress behavior deliberately, but it does not
replace correct TLS/SNI, certificate and DNS configuration for the endpoint.
Generated TLS-enabled Nginx edge configs can also listen on HTTP port `80` and
redirect plain HTTP requests to HTTPS before applying camouflage routing. The
instance form exposes this as `Redirect HTTP to HTTPS`; leave the redirect
server name empty to reuse the primary `server_name`, or set a wildcard such as
`*.example.com` when one edge should redirect a DNS wildcard.
For repeatable smoke tests, pass the same fallback explicitly:
`MEGAVPN_FALLBACK_UPSTREAM_URL=https://target.example.com
scripts/service-pack-smoke.sh --matrix <node-id> <endpoint-domain>
[certificate-id]`. Matrix smoke skips camouflage packs when this value is not
set, because using the ingress host itself as fallback can create a proxy loop.
To test protocols in batches and avoid unnecessary port conflicts on one node,
limit the matrix with `--packs` or `MEGAVPN_SMOKE_PACKS`:
`scripts/service-pack-smoke.sh --matrix <node-id> <endpoint-domain>
[certificate-id] --packs openvpn_tcp_11994,openvpn_udp_1194,wireguard_roadwarrior`.
Use `--exclude` or `MEGAVPN_SMOKE_EXCLUDE_PACKS` to temporarily skip a pack.
Before a real run, use `--plan` or `MEGAVPN_SMOKE_PLAN_ONLY=1`: the smoke
script prints selected packs, endpoint hosts, required certificate/fallback
inputs and possible listen-port overlaps without creating instances.
For staged validation of the main protocol groups, use the batch runner:
`scripts/service-pack-staged-smoke.sh --plan <node-id> <endpoint-domain> [certificate-id]`,
then rerun without `--plan`. Available batches are `remote_access_l3`
for OpenVPN/WireGuard, `proxy_access` for HTTP Proxy/MTProto/Shadowsocks,
`xray_reality`, `xray_nginx_http`, `xray_nginx_grpc` and `legacy_l2tp`
for IPsec/L2TP. A normal all-batches run on one node requires `--cleanup`
because several batches reuse public port 443; without cleanup the runner stops
before creating resources. Limit a run with
`--batches remote_access_l3,proxy_access`. The staged runner prints a
`staged_summary:` path and writes a top-level `_staged-summary.json` under the
evidence root; this file shows every batch status and its `_matrix-summary.json`
path. Override it with `MEGAVPN_SMOKE_STAGED_SUMMARY_FILE` when needed.
For repeated diagnostic runs on the same disposable node, enable
`MEGAVPN_SMOKE_CLEANUP=1`: after a successful smoke the script deletes the
created smoke client, its artifacts/share-links and waits for `instance.delete`
jobs for created instances. Add `MEGAVPN_SMOKE_CLEANUP_ON_FAILURE=1` when a
failed smoke should also remove partially created resources automatically. Keep
cleanup disabled for release evidence until runtime state has been reviewed.
For machine-readable evidence, set `MEGAVPN_SMOKE_EVIDENCE_DIR`, for example
`MEGAVPN_SMOKE_EVIDENCE_DIR=tmp/service-pack-evidence`. Each successful pack
writes one JSON file with input parameters, created instances, runtime install
jobs, applied instance snapshots, runtime states, provision result and
artifacts. Matrix runs also write `_matrix-summary.json` with totals and
OK/FAILED/SKIPPED rows; override the path with
`MEGAVPN_SMOKE_MATRIX_SUMMARY_FILE` when needed. After a matrix run, render and
validate the saved files offline:
`scripts/service-pack-evidence-report.js tmp/service-pack-evidence/_matrix-summary.json`.
For acceptance of a staged batch, add
`--require-pack openvpn_tcp_11994,openvpn_udp_1194,wireguard_roadwarrior`; the
script exits non-zero when a pack did not produce OK evidence, runtime is not
ready or the client has no active service access with a ready artifact of the
expected type.
The API, Web UI, Nginx renderer and smoke script reject fallback URL/Host/SNI
values that target the same public ingress host.
By default the smoke script also waits for each created instance runtime
projection to become `runtime_status=active`, `health_status=healthy` and
`drift_status=in_sync` after apply. Set `MEGAVPN_SMOKE_RUNTIME_CHECK=0` only
for create/provision diagnostics where runtime convergence is intentionally out
of scope. Set `MEGAVPN_SMOKE_REQUIRE_AGENT_REPORT=1` when release evidence must
prove that the agent reported live systemd/listening-port state, not only the
job-derived runtime projection.
When a clean node is missing a runtime capability, service-pack creation can
queue `runtime_install_jobs`. The smoke script waits for those jobs before
queuing or polling instance apply, so a clean-node matrix validates installer
convergence and not just preinstalled runtimes. After client provisioning, the
smoke script also waits for post-provision `instance.apply` jobs that deliver
client UUIDs/keys into runtime, then verifies that every selected service access
is `active` and has its own ready artifact of the expected protocol type.

### 13.2 Manual Instance

Use this for detailed control:

- additional OpenVPN service on the same node;
- dedicated VLESS endpoint;
- Nginx edge profile;
- custom Shadowsocks port;
- manual route or network policy.

After editing the spec, check revision status. A draft revision cannot be
applied until validation makes it apply-ready.

## 14. Apply And Diagnose An Instance

Lifecycle:

1. `draft` - config is being edited or failed validation.
2. `apply-ready` - revision can be applied.
3. `applying/provisioning` - job is queued or running.
4. `active` - desired and runtime state match.
5. `degraded` - service partially works or has runtime warnings.
6. `failed` - apply/runtime validation failed.

Immediately after creating an instance, `provisioning` is not an error. The UI
should show failure only after the job finishes or a failed runtime report is
received.

`Manage` should expose:

- latest job;
- job timeline;
- service logs;
- runtime capability state;
- unit status;
- rendered config diagnostics without revealing secrets.

## 15. VLESS And Egress

A VLESS instance is the entry point. Traffic exit must be controlled at
instance/backhaul/route-policy level.

Correct model:

1. Client connects to VLESS on an ingress node.
2. Xray inbound accepts the connection.
3. Instance config selects default outbound:
   - local breakout when ingress exit is allowed;
   - managed egress through backhaul when traffic must exit from an egress node.
4. Route policy and managed backhaul provide the deterministic path.

Where to configure it:

- `Backhaul`: create the `ingress -> egress` link, click `Apply`, then `Test`.
- `Instances -> Create from pack`: for packs that contain VLESS, use
  `VLESS routing` before creation. This writes the egress choice into every
  generated Xray/VLESS instance without changing the reusable service-pack
  template.
- `Instances -> Manage` for the Xray/VLESS instance: select `Egress mode`.
  Use `egress node` when the whole VLESS instance must exit through a remote
  egress node. Select the exact `Egress node` when more than one link exists or
  deterministic output is required.
- `Instances -> VLESS groups`: configure reusable VLESS access groups once.
  These groups are provisioning-time access policies, not just labels:
  - `Use instance default route`: follow the instance-level egress policy.
  - `Exit from current node`: force local breakout on the VLESS node.
  - `Exit from selected egress node`: resolve the selected egress node through
    active managed backhaul during apply and generate a dedicated Xray outbound.
  - `Allow only selected instance`: allow traffic only to the selected service
    instance endpoint and block everything else for users in that group.
  - `Block all traffic`: deny traffic for quarantine or suspended access.
  - `Block ads`: add a managed Xray `geosite:category-ads-all` rule for users
    in that group. The Xray runtime must include the required geosite data.
  Saving, disabling or deleting a group automatically syncs the catalog into
  existing Xray/VLESS instances and queues apply jobs for active instances. If
  sync fails for a specific instance, the UI shows the stage and error.
- `Instances -> Manage` for the Xray/VLESS instance: `Default VLESS group`
  selects the group used when a client binding does not specify one. Advanced
  JSON override is intentionally collapsed and should be used only for
  non-standard transport experiments.
- `Clients -> Provision`: when selecting a VLESS inbound, choose the access
  group. The group is saved in the client access binding and used for
  provisioning. Provisioning synchronizes active catalog groups into the
  selected Xray instance before validation, so a freshly created active group
  can be selected immediately. If the group is disabled/deleted or selected
  egress cannot be resolved through active backhaul, the API returns the
  available groups and the blocking resolution error.
- `Clients -> Access -> Add route`: for individual route-policy rules, select
  `Egress mode = egress node` and choose the egress node. After route-policy
  changes, run `Nodes -> Manage -> Inspect route policy` first; if the preview
  has no blocking warning for the intended path, run `Sync route policy` on the
  ingress node.
- `Nodes -> Manage -> Inspect route policy`: read-only projection for the
  ingress node. It shows enforceable routes, observe-only routes, VLESS/Xray
  system egress routes, blocked reasons, managed table/interface and the
  nft/ip-rule primitives that the agent will own. VLESS UUID-like source
  identities are redacted.
- `Nodes -> Manage -> Sync route policy`: the ingress agent writes the signed
  snapshot and installs managed kernel rules. Client traffic is marked in
  `inet megavpn route_policy_prerouting`; local Xray/VLESS `sendThrough`
  traffic is marked in `inet megavpn route_policy_output`; `ip rule fwmark`
  then sends the marked flow into the selected backhaul route table and `mgbh*`
  interface. The job result includes telemetry for the route-policy unit/timer,
  `ip rule show`, managed nftables chains and `ip route show table` for managed
  route tables referenced by the current or previous snapshot.
- `Nodes -> Manage -> Clean route policy`: explicit rollback for managed
  route-policy runtime. Use it when a node was removed from the ingress path,
  stale route-policy state is suspected after instance/client deletion, or a
  previous apply must be cleared before rebuilding. It removes only MegaVPN
  managed route-policy files, reserved `ip rule` priorities and managed nftables
  route-policy chains.

Minimal path for “enter through VLESS and exit through another node”:

1. The ingress node and egress node must be online.
2. Backhaul between them must be `active` and tested.
3. The VLESS instance must run on the ingress node.
4. Either select `Use selected egress node` during `Create from pack`, or open
   `Instances -> Manage`, set `Egress mode = egress node` for the VLESS
   instance and select the target egress node.
5. Open `Instances -> VLESS groups` if a client-specific group is required.
   For example, create `Exit from selected egress node` for users that must use
   a specific egress node, or `Allow only selected instance` for restricted
   access.
6. Click `Apply` on the instance if you changed instance-level `Egress mode`.
   VLESS group changes themselves are propagated automatically by catalog sync.
7. Run `Inspect route policy` on the ingress node and verify that the
   VLESS/Xray system route is active and uses the expected backhaul table and
   `mgbh*` interface.
8. If client route rules are used, run `Sync route policy`.
9. Inspect the `node.route_policy.apply` job result telemetry first. If it is
   inconclusive, verify on the ingress node with `nft list chain inet megavpn
   route_policy_output`, `nft list chain inet megavpn
   route_policy_prerouting`, `ip rule show` and
   `ip route show table <backhaul_table>`. If the job says `has no ready
   managed backhaul candidate`, promote a healthy standby transport or re-apply
   the selected backhaul, then re-apply the Xray instance so `sendThrough`
   points to the live `mgbh*` source address.

See [VLESS access groups](VLESS_GROUPS.md) for the detailed group model,
runtime behavior and validation rules.

## 16. Clients And Provisioning

1. Open `Clients`.
2. Create a client account.
3. Select `Provision`.
4. Select the exact service instances the client may use.
5. Queue the provisioning job.
6. After queueing, the UI should show job id, selected services, status and next
   action.
7. After successful provisioning, open client access.
8. Build artifacts: `.ovpn`, VLESS URL, WireGuard config, Shadowsocks URI or
   bundle.
9. Preview/download generated material before sending it to the client.
10. Optionally publish a share link, rotate a VLESS subscription URL or send
    email.
11. To rebuild delivery material without removing service access, use `Clients ->
    Access -> Client Configs -> Clear configs`, then run `Build configs` again.
12. To fully remove a client, use `Clients -> Delete`. The operation removes the
    client account, service accesses, routes, generated configs, delivery links,
    VLESS subscriptions and service-access secret refs, then queues apply jobs
    for affected service instances.

Provisioning must not silently grant every compatible service. Operators choose
the exact inbound services per client.

For Xray/VLESS, the client UUID is treated as the client's reusable service
identity. When the same client is provisioned onto an additional VLESS ingress,
the Control Plane reuses the existing UUID, writes it into the new instance's
managed client list and queues instance apply. The access stays `pending` until
the agent reports a successful apply; only then does it become `active`.

`Delete` on one row in `Client Configs` or `Artifacts` removes only that
generated config and delivery links pointing to it. `Clear configs` removes all
generated configs, share links and subscription tokens for the client. Neither
operation revokes service access: service bindings stay in place and the
operator can build fresh artifacts. `Delete client` is irreversible removal from
the runtime model; audit and job history remain for traceability.

## 17. Share Links, VLESS Subscriptions And Email

A share link is a bearer URL. Its safety depends on:

- high-entropy token;
- expiry;
- revocation;
- `token_hash` in the database instead of plaintext token;
- audit events.

The plaintext token is shown only when the link is created. If it is lost, create
a new share link.

A VLESS subscription is also a bearer URL, but it does not serve a static
artifact. It resolves the current active VLESS service accesses for the client
and returns a newline-separated profile feed. Use it only after provisioning has
completed, because the feed requires the generated VLESS UUID stored in service
access metadata.

Operator workflow:

1. Open `Clients -> Access`.
2. Confirm the client has active VLESS inbound access.
3. In `VLESS Subscription`, click `Rotate subscription`.
4. Copy the generated URL immediately. The plaintext token is not stored.
5. Revoke the subscription when the URL is no longer trusted.

See [VLESS subscriptions](VLESS_SUBSCRIPTIONS.md) for token lifecycle,
failure scenarios and public endpoint behavior.

## 18. Firewall Policies

`Firewall` is a staged policy catalog, not an immediate node-side editor.

Recommended workflow:

1. Open `Firewall -> Address lists` and create reusable source or destination
   lists.
2. Add entries. Leave entry type on auto-detect unless you need to force CIDR,
   single IP, range or DNS.
3. Open `Firewall -> Rules` and create ordered rules. Use presets for SSH,
   HTTPS, WireGuard, OpenVPN, IPsec/L2TP, Shadowsocks, HTTP proxy, MTProto,
   Nginx edge or invalid-packet cases, then adjust source lists and ports. Use
   the rule filters when the catalog has multiple policies or chains.
4. Open `Firewall -> Policies` to review defaults and rule count.
5. Open `Firewall -> Node state` or policy apply action and queue apply for the
   selected node.

Catalog changes become effective only after `node.firewall.apply` completes.
The apply dialog has two modes:

- Default mode installs explicit catalog rules and keeps managed base chains at
  `accept`.
- Strict mode enables `enforce_default_policy` and applies the policy default
  input/forward/output actions. Use it only after management access and
  required protocol listener rules are present.

When strict output default is `drop` or `reject`, the agent requires either an
IP-pinned control-plane URL or an explicit active output accept rule for the
control-plane TCP port. If that guard is missing, the job fails during render
before changing nftables.

## 19. Jobs, Audit And Troubleshooting

`Jobs` shows queue status, result and failure reason.

Common cases:

| Symptom | Where to look | What to check |
| --- | --- | --- |
| Node offline | Nodes -> Manage | agent service, heartbeat, public URL, token |
| Runtime missing | Services | capability verify/install result |
| Apply failed | Instances -> Manage, Jobs | latest apply job, unit status, config validation |
| OpenVPN stuck activating | Instance logs, systemd state | config path, port, CA profile, unit status |
| Shadowsocks config missing | Instance logs | generated config path, package install, password/spec |
| VLESS does not use egress | Instance config, Backhaul, route policy | default outbound, active backhaul, policy projection |
| Backhaul failed | Backhaul modal, Jobs | ingress/egress side, interface, route lookup, packet loss |
| Firewall apply failed | Firewall -> Node state, Jobs | rendered policy, agent logs, nftables support |
| Client config invalid | Clients -> Access/Artifacts | selected services, revision applied, artifact build result |

Audit should answer:

- who created or modified a node;
- who ran bootstrap/update/cleanup;
- who applied an instance;
- who installed a runtime capability;
- who created or revoked a share link;
- who rotated or revoked a VLESS subscription;
- who changed or applied firewall policy;
- who changed settings/certificates.

## 20. Safe Deletion

Instance deletion must not be only a database row update.

Correct sequence:

1. Revoke client access that depends on the instance.
2. Stop/disable the instance if required.
3. Delete the instance through UI/API.
4. Wait for `instance.delete` cleanup job on the node.
5. Verify that systemd unit, config files and managed policy are removed.

Emergency node cleanup:

- requires confirmation by target node name;
- removes only managed state;
- can optionally remove the agent;
- must not break unrelated backhaul/routes outside managed scopes.

## 21. Production Checklist

Before production rollout:

1. `scripts/release-gate.sh` with no unexplained skips.
2. Disposable PostgreSQL migration test.
3. Backup/restore drill.
4. `nginx -t` on the edge host.
5. `systemd-analyze verify` for systemd units.
6. Agent enrollment on a test node.
7. Service smoke matrix.
8. Backhaul apply/probe.
9. Client provisioning and artifact preview/download.
10. Audit review.
11. Rollback plan.

## 21. Roles

Short version:

- `readonly`: reads state and audit.
- `engineer`: clients/artifacts/share links without node/bootstrap/apply authority.
- `admin`: operates nodes/instances/jobs/settings without unrestricted secret
  reveal.
- `superadmin`: full permission set.

Details: [RBAC matrix](RBAC_MATRIX.md).
