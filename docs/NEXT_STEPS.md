# Next steps

Актуальный baseline: `ROADMAP_V1_AND_TZ.md`.
Текущая точка фиксации: `0.6.7.2-alpha`.
Следующая итерация: `0.6.7.2`.
Канонический репозиторий: `github.com/rtis-emc2/megavpn`.

1. Зафиксировать CI green после последних branding/module path изменений: `go test`, `go vet`, `go build`.
2. Довести удаленный deployment baseline на тестовом сервере: проверить `MEGAVPN_DEPLOY_SYNC_MODE=auto`, backup branch flow и повторный deploy после rewritten history.
3. Проверить service-pack и runtime paths на тестовом сервере: IPsec+XL2TPD, Xray Reality, Xray+Nginx gRPC, Xray HTTP/WebSocket, OpenVPN TCP/UDP, WireGuard, HTTP Proxy, MTProto, Shadowsocks.
4. Запускать API только с явно заданными bootstrap credentials:
   - `MEGAVPN_BOOTSTRAP_ADMIN_USERNAME`
   - `MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD`
5. Создать OpenAPI/public API contract и internal agent API contract.
6. Формализовать typed job payload schemas.
7. Спроектировать и реализовать mTLS или signed jobs/results для agent transport.
8. Вынести service driver contracts для OpenVPN/Xray/WireGuard/Nginx/IPsec/Squid.
9. Довести revision flow до `candidate -> validated -> applied -> rollback`.
10. Добавить integration tests на PostgreSQL-backed jobs, locks, provisioning и agent loop.
