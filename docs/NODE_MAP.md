# Node Map

**Release:** `7.0.1.39`

Node Map is the topology workspace for approximate node placement and managed
backhaul visibility. Operators do not enter latitude, longitude or datacenter
labels manually. The control plane derives map metadata from the public node
address through a configured GeoIP endpoint and stores the result as a cache on
the node record.

Russian companion: [NODE_MAP_RU.md](NODE_MAP_RU.md).

## Data Model

The node map uses the following data:

| Data | Source | Notes |
| --- | --- | --- |
| Node marker | `nodes.address` plus GeoIP cache | Only public IP addresses are resolved. Private, loopback, multicast and link-local addresses are skipped. |
| Country, region, city | GeoIP response | Approximate data only; it is for operational orientation, not billing or compliance proof. |
| Network owner | GeoIP `org`, `isp`, `asn`, `company`, `traits` or equivalent fields | This usually identifies the hosting provider, carrier, cloud, or datacenter network owner. |
| GeoIP source | Configured provider hostname and resolved IP | Shown in the UI so operators know where the data came from. |
| Backhaul edge | `backhaul_links` and selected transport | Drawn only when both endpoint nodes have resolved coordinates. |

## Runtime Flow

1. Operator creates or edits a node and sets its public address.
2. The API extracts a host/IP from the address.
3. If the address is private or internal, GeoIP is skipped and no external
   provider is called.
4. If the address is public, the API queries the configured GeoIP endpoint.
5. Country, city, coordinates, ASN and network owner fields are cached in the
   `nodes` table.
6. The Node Map page renders the local static world map, node pins,
   selected-node details and drawable backhaul edges.

List requests also refresh stale or missing GeoIP cache entries in small batches
controlled by `MEGAVPN_GEOIP_AUTO_ENRICH_LIMIT`.

## Configuration

Runtime variables in `/etc/megavpn/megavpn.env`:

| Variable | Default | Purpose |
| --- | --- | --- |
| `MEGAVPN_GEOIP_LOOKUP_URL_TEMPLATE` | `https://ipapi.co/{ip}/json/` | HTTPS endpoint template used by the control plane. Set to `disabled` to turn off automatic lookup. |
| `MEGAVPN_GEOIP_TIMEOUT` | `3s` | Per-request lookup timeout. |
| `MEGAVPN_GEOIP_AUTO_ENRICH_LIMIT` | `5` | Maximum node records refreshed during one node list request. |

Use an internal GeoIP service in regulated environments. External GeoIP
providers will see the public node IPs that are looked up.

## UI Behavior

The page is intentionally map-first:

- the map uses a bundled static world-map asset and does not require operators
  to enter coordinates or load external map imagery;
- clicking a node pin selects the node and opens an inspector on the same page;
- the inspector shows address, role, status, city/country, coordinates, network
  owner, ASN/source metadata and related backhaul;
- nodes without resolved coordinates are shown in a compact pending GeoIP block;
- the compact mapped-node list is a selector, not a replacement for the Nodes
  manage page.

## Backhaul Rendering

The map overlays managed ingress-to-egress backhaul as directed lines between
node pins:

- active/applied transports are rendered as healthy links;
- provisioning/degraded links are rendered as warning links;
- failed links are rendered as failed links;
- disabled links remain visible as disabled topology and are excluded from
  managed route-policy selection until enabled again;
- deleted links are not shown;
- links with unresolved node coordinates are counted in the summary but not
  drawn;
- metric labels are shown on mapped links when the route metric is configured;
- the topology cards below the map show direction, driver, metric, endpoint and
  selected transport status for every non-deleted managed link;
- the route switch in each topology card queues managed enable/disable jobs for
  the selected link and shows the accepted job types returned by the API;
- topology cards show active backhaul cleanup/apply jobs and ingress node
  route-policy jobs while convergence is still pending;
- selected transport errors are shown directly on the topology card so failed
  links have an immediate operator hint without leaving the map page.

The Backhaul page remains the source of truth for apply, probe, cleanup and
detailed transport diagnostics. Node Map is a topology view, not a replacement
for operational troubleshooting.

## Security Rules

- The resolver only accepts HTTPS GeoIP templates with the `{ip}` placeholder.
- The resolver rejects private, loopback, link-local, multicast and unspecified
  addresses before any provider call.
- Lookup response bodies are size-limited and parsed as JSON.
- GeoIP is best-effort: lookup failures never block node creation or update.
- The map background is served from local web assets; CSP does not require any
  external image source for the topology view.

## Operational Notes

- Coordinates are approximate and can point to provider POP, city, region or
  registered network location.
- Network owner can identify the carrier or hosting provider, not necessarily
  the exact physical datacenter.
- After changing GeoIP settings, restart the API service.
- Existing nodes are enriched automatically after migration when the UI or API
  lists nodes.
