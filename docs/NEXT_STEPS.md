# Next steps

Актуальный baseline: `ROADMAP_V1_AND_TZ.md`.
Текущая точка фиксации: `0.6.8.1-alpha`.
Следующая итерация: `0.6.8.1`.
Канонический репозиторий: `github.com/rtis-emc2/megavpn`.

1. Зафиксировать CI green после последних branding/module path изменений: `go test`, `go vet`, `go build`.
2. Довести удаленный deployment baseline на тестовом сервере: проверить `MEGAVPN_DEPLOY_SYNC_MODE=auto`, backup branch flow и повторный deploy после rewritten history.
3. Проверить service-pack и runtime paths на тестовом сервере: IPsec+XL2TPD, Xray Reality, Xray+Nginx gRPC, Xray HTTP/WebSocket, OpenVPN TCP/UDP, WireGuard, HTTP Proxy, MTProto, Shadowsocks. Для repeatable smoke использовать `scripts/service-pack-smoke.sh --matrix <node-id> <endpoint-domain> [certificate-id]`; для полного operational-truth включать provisioning и artifact/share-link checks.
4. `Paused`: ACME / Let's Encrypt automation пока сознательно не внедряем. Перед возобновлением нужно выбрать canonical challenge strategy: `HTTP-01`, `DNS-01` или delegated external ACME.
5. Запускать API только с явно заданными bootstrap credentials:
   - `MEGAVPN_BOOTSTRAP_ADMIN_USERNAME`
   - `MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD`
6. Создать OpenAPI/public API contract и internal agent API contract.
7. Формализовать typed job payload schemas.
8. Спроектировать и реализовать mTLS или signed jobs/results для agent transport.
9. Вынести service driver contracts для OpenVPN/Xray/WireGuard/Nginx/IPsec/Squid.
10. Довести revision flow до `candidate -> validated -> applied -> rollback`.
11. Добавить integration tests на PostgreSQL-backed jobs, locks, provisioning и agent loop.
