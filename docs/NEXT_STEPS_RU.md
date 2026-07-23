# Следующие шаги

**Релиз:** `7.1.1.14`

Актуальный baseline: [`ROADMAP_V1_AND_TZ_RU.md`](../ROADMAP_V1_AND_TZ_RU.md).
Английская версия: [`NEXT_STEPS.md`](NEXT_STEPS.md).
Канонический репозиторий: `github.com/rtis-emc2/megavpn`.

1. Проверить traffic accounting collectors на живых нодах после повторного
   apply managed Xray, OpenVPN и WireGuard instances: Xray Stats API, WireGuard
   `wg show <interface> transfer`, OpenVPN status files, attribution к
   `service_accesses`, Traffic Accounting `Collector status`
   active/degraded/missing/inactive freshness, expected/observed/missing
   instance coverage, reconnect/restart behavior и измеренную реальную
   cardinality перед решением о table partitioning или cold archive storage.
   Затем добавить explicit collector heartbeat, если operators нужен collector
   health независимо от user traffic deltas.
2. На реальных ingress/egress nodes повторить Backhaul Apply profiles после обновления API/UI/agent: re-apply должен остановить obsolete managed unit из предыдущего/sibling manifest, удалить предыдущий/целевой `mgbh*` interface, удалить stale managed WireGuard listener с конфликтующим endpoint port, корректно создать nft NAT rule с quoted comment, поднять новый runtime-state и показать одинаковый `/30` profile на ingress/egress. После Delete Backhaul убедиться, что link ушел из активного списка и отображается в `Recently Deleted Backhaul` с cleanup summary. Если OpenVPN profile снова упадет, Jobs/Backhaul summary должен показать unit name, active state и первую полезную строку `systemctl status`/OpenVPN error; эту строку использовать как root cause для следующего исправления.
3. Запустить PostgreSQL integration suite с `MEGAVPN_TEST_DATABASE_DSN`; тест создает временную schema, применяет все migrations и проверяет jobs, locks, provisioning и baseline access routes.
4. Проверить `/api/v1/service-drivers`, `/api/v1/instances/runtime-states`, `/api/v1/instances/{id}/runtime-state`, `/api/v1/instances/{id}/runtime-observations` и `/agent/runtime/instances` на тестовом control plane после реального `instance.apply`.
5. Проверить node bootstrap console на удаленном control plane: top tabs, onboarding mode explanation, Agent channel next-step CTA, видимость `MEGAVPN_PUBLIC_BASE_URL` с кастомным HTTPS-портом, Settings -> Control Plane TLS profile + Apply edge, создание SSH access method, rotate enrollment token, queue bootstrap, кнопки Nodes -> Update / Update all agents с preflight по `node.bootstrap` и enabled SSH access method, автоматический переход setup method из SSH bootstrap в agent-managed после успешной установки агента, чтение bootstrap run details в одно-колоночном layout без горизонтальной прокрутки, а также переход `awaiting heartbeat -> online` после первого heartbeat агента.
6. Довести удаленный deployment baseline на тестовом сервере: прогнать `scripts/ops/control-plane-install.sh` на свежей машине, проверить generated env/master key/admin credentials/nginx self-signed edge, затем проверить `MEGAVPN_DEPLOY_SYNC_MODE=auto`, backup branch flow и повторный deploy после rewritten history.
7. Проверить service-pack и runtime paths на тестовом сервере: IPsec+XL2TPD, Xray Reality, Xray+Nginx gRPC, Xray VLESS WebSocket Camouflage с fallback website, OpenVPN TCP/UDP, WireGuard, HTTP Proxy, MTProto, Shadowsocks. Для repeatable smoke использовать `scripts/smoke/service-pack-smoke.sh --matrix <node-id> <endpoint-domain> [certificate-id]`; перед реальным запуском делать `--plan`, чтобы увидеть выбранные packs, endpoint hosts, certificate/fallback requirements и port overlaps без создания instances. Для camouflage matrix обязательно задать `MEGAVPN_FALLBACK_UPSTREAM_URL` на реальный fallback website, иначе эти packs будут намеренно пропущены. Чтобы не создавать лишние port conflicts на одной node, тестировать партиями через `--packs` / `MEGAVPN_SMOKE_PACKS`, затем собрать полный evidence по всем packs. Evidence staged batch сохранять в `MEGAVPN_SMOKE_EVIDENCE_DIR` и принимать только после `scripts/ci/service-pack-evidence-report.js` по `_matrix-summary.json`. Для operator runs предпочитать `scripts/smoke/service-pack-staged-smoke.sh`, потому что он группирует протоколы в conflict-aware batches, валидирует evidence после каждой партии и пишет общий `_staged-summary.json` для трассировки статуса всех партий. Все 443-based batches гонять с `--cleanup` для диагностики или на изолированных disposable nodes для финального release evidence. Для полного operational-truth включать provisioning и artifact/share-link checks.
8. Проверить и усилить topology workspace: локальная статичная world map, GeoIP node placement, node owner metadata, role/health badges, backhaul edges, operator-facing route-toggle UX на реальных nodes, failed-hop diagnostics и per-node workload drill-down. Backend schema для route-toggle, cleanup batch metadata и regression coverage route-policy refresh уже добавлены.
9. Проверить VLESS access groups end to end: default route, local breakout, selected egress node, target-only access, blocked access, ad-block rule, выбор группы при provisioning и on-demand catalog sync для свежесозданных active groups.
10. Проверить VLESS subscriptions end-to-end: rotate/revoke token, one-time URL display, public `Cache-Control: no-store` feed, фильтрация active access, QR/text export и visibility provisioning result.
11. Довести hardening traffic camouflage после baseline HTTP-to-HTTPS redirect и shared Nginx cleanup: Nginx config preview, `nginx -t` evidence surface, live smoke fallback site и проверка generated VLESS subscription.
12. Выделить Nginx edge profile catalog: reusable profile definitions, certificate binding, generated config diff, atomic apply and operator-visible failure reason.
13. `Paused`: ACME / Let's Encrypt automation пока сознательно не внедряем. Перед возобновлением нужно выбрать canonical challenge strategy: `HTTP-01`, `DNS-01` или delegated external ACME.
14. Запускать API только с явно заданными bootstrap credentials:
   - `MEGAVPN_BOOTSTRAP_ADMIN_USERNAME`
   - `MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD`
15. Создать OpenAPI/public API contract и internal agent API contract.
16. Формализовать typed job payload schemas поверх текущего `internal/jobschema`.
17. Продолжить agent transport security: принять v1.0 решение по обязательному mTLS поверх уже реализованных HMAC-signed HTTP messages.
18. Продолжить UI split: следующим отдельным шагом вынести node management workspace/bootstrap diagnostics из `app.js`.
19. Довести revision flow от текущего `draft/validated/applied/failed` baseline с rollback/diff UX до полного `candidate -> validated -> applied -> rollback` с apply history и безопасным rollback engine.
20. Продолжить routing hardening после baseline `Inspect route policy`, apply telemetry и explicit cleanup: conntrack visibility, MTU/MSS clamp и live telemetry validation на реальных nodes.
21. Довести managed backhaul до полного multi-driver enforcement: controlled Xray TUN activation, strongSwan/IKEv2 activation, OpenVPN certificate-mode P2P и traffic/latency health probes.
22. После согласования окна обслуживания переписать Git history для удаления sensitive historical commits/tags, force-push текущей ветки и обновить серверные checkout через `MEGAVPN_DEPLOY_ALLOW_HISTORY_REWRITE=1`.
