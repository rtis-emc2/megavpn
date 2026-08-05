# Control Plane Frontend

**Release:** `8.0.0-pre.1`

The supported Control Plane UI is the React and TypeScript application in
`frontend/`. There is no second maintained UI implementation.

## Ownership And Build

| Path | Purpose |
| --- | --- |
| `frontend/src` | Maintained application source |
| `frontend/dist` | Reproducible, committed production build |
| `/opt/megavpn/web` | Deployment target on the Control Plane host |

`frontend/go.mod` is an intentional module boundary: root-level Go build and
test commands must never traverse npm dependencies.

The API serves `index.html` for known frontend routes and serves hashed assets
with immutable caching. API, agent, public download and health routes are never
handled by the SPA fallback.

```bash
cd frontend
npm ci
npm run typecheck
npm run lint
npm test
npm run i18n:check
npm run build
```

CI rebuilds `frontend/dist` and rejects a diff. A source change without the
matching production bundle is therefore not releasable.

## Deployment

`deploy-local.sh` runs the verified frontend installer. The installer copies
`frontend/dist` to `/opt/megavpn/web` with deletion of obsolete hashed assets.
It rejects custom destinations by default and rejects symbolic-link targets.
When Git synchronization changes the checked-out revision, `deploy-local.sh`
restarts itself before build or installation. This prevents an old in-flight
script body from calling removed deployment entrypoints from the new revision.

The frontend and API use one origin. The browser cannot configure an alternate
API origin, and authentication material is never stored in browser storage.

## Security Boundary

- Session authentication uses the HttpOnly Control Plane cookie.
- Unsafe requests require the CSRF header enforced by the API.
- The Content Security Policy permits scripts, styles and connections only from
  the Control Plane origin.
- Terminal WebSocket tickets are accepted only for the selected node and the
  current Control Plane origin.
- HTML injection, browser secret storage, direct `fetch` calls outside the
  shared API client and legacy route references are blocked by CI guards.

## Rollback And Failure Modes

Rollback the complete release, including API binary and `frontend/dist`; do not
mix frontend assets from one release with an API from another. If deployment is
interrupted, rerun `deploy-local.sh`: installation is idempotent and removes
stale bundle files. A missing `index.html` or asset directory fails deployment
instead of publishing an incomplete UI.
