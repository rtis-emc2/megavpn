# FE8 8.0.0 Responsive Evidence Plan

Branch: `release/8.0.0-frontend-console`

Generated UTC: `2026-07-09T09:57:57Z`

Status: **OPEN**.

Final cutover impact: responsive desktop, tablet and phone evidence must be
captured against a real disposable backend before 8.0.0 can be marked GO.
Screenshots without real workflow data are not release evidence.

## Required Viewports

| Profile | Size |
| --- | --- |
| Desktop | `1440x900` |
| Dense desktop | `1280x800` |
| Tablet landscape | `1024x768` |
| Tablet portrait | `768x1024` |
| Phone | `390x844` |
| Narrow phone | `360x800` |

## Required Pages And Workflows

- Dashboard and global navigation.
- Clients list/detail, Groups, Routes/Maintenance, Artifacts and Delivery.
- Firewall inventory, CRUD, preview/apply and emergency disable.
- Instances, Service Packs and Runtime Artifacts.
- Nodes overview, inventory, diagnostics, capabilities, bootstrap and security.
- Platform Certificates/PKI.
- Platform Settings, Mail/Delivery and Access/RBAC.
- Infrastructure Backhaul.
- Network Policy Route Policy.
- Confirmation dialogs, destructive actions and one-time secret panels.

## Evidence Rules

- Capture screenshots from a real disposable API environment.
- Confirm text does not overlap or clip inside buttons, tabs, cards, tables,
  drawers and modals.
- Confirm destructive controls remain reachable.
- Confirm drawers and confirmation dialogs fit on phone viewports.
- Confirm responsive tables/cards remain readable.
- Confirm private keys, tokens, subscription URLs, share URLs, enrollment
  secrets and terminal tickets are not persisted or accidentally visible after
  close/success.
- Record the SHA, browser, viewport, route and operator role for each capture.

## Current Blocker

No disposable backend/API data environment was available in this workstation
session. Responsive evidence remains OPEN and final 8.0.0 cutover remains
NO-GO.
