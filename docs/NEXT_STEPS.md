# Next steps

Актуальный baseline: `ROADMAP_V1_AND_TZ.md`.
Текущая точка фиксации: `0.6.10.3-alpha`.
Следующая итерация: `0.6.10.4-alpha`.
Канонический репозиторий: `github.com/rtis-emc2/megavpn`.

1. Зафиксировать CI green после managed backhaul probe/cleanup lifecycle: `bash -n`, `go test`, `go vet`, `go build`, `scripts/build.sh`, `git diff --check`.
2. Запустить PostgreSQL integration suite с `MEGAVPN_TEST_DATABASE_DSN`; тест создает временную schema, применяет все migrations и проверяет jobs, locks, provisioning и baseline access routes.
3. Проверить `/api/v1/service-drivers`, `/api/v1/instances/runtime-states`, `/api/v1/instances/{id}/runtime-state`, `/api/v1/instances/{id}/runtime-observations` и `/agent/runtime/instances` на тестовом control plane после реального `instance.apply`.
4. Проверить node bootstrap console на удаленном control plane: top tabs, onboarding mode explanation, Agent channel next-step CTA, видимость `MEGAVPN_PUBLIC_BASE_URL` с кастомным HTTPS-портом, Settings -> Control Plane TLS profile + Apply edge, создание SSH access method, rotate enrollment token, queue bootstrap, автоматический переход setup method из SSH bootstrap в agent-managed после успешной установки агента, чтение bootstrap run details в одно-колоночном layout без горизонтальной прокрутки, а также переход `awaiting heartbeat -> online` после первого heartbeat агента.
5. Довести удаленный deployment baseline на тестовом сервере: прогнать `scripts/control-plane-install.sh` на свежей машине, проверить generated env/master key/admin credentials/nginx self-signed edge, затем проверить `MEGAVPN_DEPLOY_SYNC_MODE=auto`, backup branch flow и повторный deploy после rewritten history.
6. Проверить service-pack и runtime paths на тестовом сервере: IPsec+XL2TPD, Xray Reality, Xray+Nginx gRPC, Xray VLESS WebSocket Camouflage с fallback website, OpenVPN TCP/UDP, WireGuard, HTTP Proxy, MTProto, Shadowsocks. Для repeatable smoke использовать `scripts/service-pack-smoke.sh --matrix <node-id> <endpoint-domain> [certificate-id]`; для полного operational-truth включать provisioning и artifact/share-link checks.
7. `Paused`: ACME / Let's Encrypt automation пока сознательно не внедряем. Перед возобновлением нужно выбрать canonical challenge strategy: `HTTP-01`, `DNS-01` или delegated external ACME.
8. Запускать API только с явно заданными bootstrap credentials:
   - `MEGAVPN_BOOTSTRAP_ADMIN_USERNAME`
   - `MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD`
9. Создать OpenAPI/public API contract и internal agent API contract.
10. Формализовать typed job payload schemas поверх текущего `internal/jobschema`.
11. Продолжить agent transport security: принять v1.0 решение по обязательному mTLS поверх уже реализованных HMAC-signed HTTP messages.
12. Продолжить UI split: следующим отдельным шагом вынести node management workspace/bootstrap diagnostics из `app.js`.
13. Довести revision flow от текущего `draft/validated/applied/failed` baseline с rollback/diff UX до полного `candidate -> validated -> applied -> rollback` с apply history и безопасным rollback engine.
14. Продолжить routing hardening: добавить rollback/remove stage для retired/disabled route policies, conntrack visibility, MTU/MSS clamp и health telemetry для route-policy unit.
15. Довести managed backhaul до полного multi-driver enforcement: controlled Xray TUN activation, strongSwan/IKEv2 activation, OpenVPN certificate-mode P2P и traffic/latency health probes.
16. После согласования окна обслуживания переписать Git history для удаления private domain mentions из всех commits/tags, force-push текущей ветки и обновить серверные checkout через `MEGAVPN_DEPLOY_ALLOW_HISTORY_REWRITE=1`.
