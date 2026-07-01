# Next steps

Актуальный baseline: `ROADMAP_V1_AND_TZ.md`.
Текущая точка фиксации: `0.7.0.1-beta`.
Следующая итерация: `0.7.0.2-beta`.
Канонический репозиторий: `github.com/rtis-emc2/megavpn`.

1. На реальных ingress/egress nodes повторить Backhaul Apply profiles после обновления API/UI/agent: re-apply должен остановить obsolete managed unit из предыдущего/sibling manifest, удалить предыдущий/целевой `mgbh*` interface, удалить stale managed WireGuard listener с конфликтующим endpoint port, корректно создать nft NAT rule с quoted comment, поднять новый runtime-state и показать одинаковый `/30` profile на ingress/egress. После Delete Backhaul убедиться, что link ушел из активного списка и отображается в `Recently Deleted Backhaul` с cleanup summary. Если OpenVPN profile снова упадет, Jobs/Backhaul summary должен показать unit name, active state и первую полезную строку `systemctl status`/OpenVPN error; эту строку использовать как root cause для следующего исправления.
2. Запустить PostgreSQL integration suite с `MEGAVPN_TEST_DATABASE_DSN`; тест создает временную schema, применяет все migrations и проверяет jobs, locks, provisioning и baseline access routes.
3. Проверить `/api/v1/service-drivers`, `/api/v1/instances/runtime-states`, `/api/v1/instances/{id}/runtime-state`, `/api/v1/instances/{id}/runtime-observations` и `/agent/runtime/instances` на тестовом control plane после реального `instance.apply`.
4. Проверить node bootstrap console на удаленном control plane: top tabs, onboarding mode explanation, Agent channel next-step CTA, видимость `MEGAVPN_PUBLIC_BASE_URL` с кастомным HTTPS-портом, Settings -> Control Plane TLS profile + Apply edge, создание SSH access method, rotate enrollment token, queue bootstrap, кнопки Nodes -> Update / Update all agents с preflight по `node.bootstrap` и enabled SSH access method, автоматический переход setup method из SSH bootstrap в agent-managed после успешной установки агента, чтение bootstrap run details в одно-колоночном layout без горизонтальной прокрутки, а также переход `awaiting heartbeat -> online` после первого heartbeat агента.
5. Довести удаленный deployment baseline на тестовом сервере: прогнать `scripts/control-plane-install.sh` на свежей машине, проверить generated env/master key/admin credentials/nginx self-signed edge, затем проверить `MEGAVPN_DEPLOY_SYNC_MODE=auto`, backup branch flow и повторный deploy после rewritten history.
6. Проверить service-pack и runtime paths на тестовом сервере: IPsec+XL2TPD, Xray Reality, Xray+Nginx gRPC, Xray VLESS WebSocket Camouflage с fallback website, OpenVPN TCP/UDP, WireGuard, HTTP Proxy, MTProto, Shadowsocks. Для repeatable smoke использовать `scripts/service-pack-smoke.sh --matrix <node-id> <endpoint-domain> [certificate-id]`; для полного operational-truth включать provisioning и artifact/share-link checks.
7. Спроектировать topology workspace: node map, node location metadata, role/health/workload badges, backhaul edges, failed-hop diagnostics, route-policy projection and per-node workload drill-down.
8. Спроектировать VLESS subscription endpoint: per-client subscription token, selected inbound services, subscription rotation, cache-control, QR/text export and provisioning result state.
9. Формализовать traffic camouflage profiles: Xray WebSocket/gRPC public edge, hidden path, fallback upstream, public SNI/TLS binding, nginx preview, `nginx -t` validation and rollback.
10. Выделить Nginx edge profile catalog: reusable profile definitions, certificate binding, generated config diff, atomic apply and operator-visible failure reason.
11. `Paused`: ACME / Let's Encrypt automation пока сознательно не внедряем. Перед возобновлением нужно выбрать canonical challenge strategy: `HTTP-01`, `DNS-01` или delegated external ACME.
12. Запускать API только с явно заданными bootstrap credentials:
   - `MEGAVPN_BOOTSTRAP_ADMIN_USERNAME`
   - `MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD`
13. Создать OpenAPI/public API contract и internal agent API contract.
14. Формализовать typed job payload schemas поверх текущего `internal/jobschema`.
15. Продолжить agent transport security: принять v1.0 решение по обязательному mTLS поверх уже реализованных HMAC-signed HTTP messages.
16. Продолжить UI split: следующим отдельным шагом вынести node management workspace/bootstrap diagnostics из `app.js`.
17. Довести revision flow от текущего `draft/validated/applied/failed` baseline с rollback/diff UX до полного `candidate -> validated -> applied -> rollback` с apply history и безопасным rollback engine.
18. Продолжить routing hardening: добавить rollback/remove stage для retired/disabled route policies, conntrack visibility, MTU/MSS clamp и health telemetry для route-policy unit.
19. Довести managed backhaul до полного multi-driver enforcement: controlled Xray TUN activation, strongSwan/IKEv2 activation, OpenVPN certificate-mode P2P и traffic/latency health probes.
20. После согласования окна обслуживания переписать Git history для удаления sensitive historical commits/tags, force-push текущей ветки и обновить серверные checkout через `MEGAVPN_DEPLOY_ALLOW_HISTORY_REWRITE=1`.
