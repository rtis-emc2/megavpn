# RTIS MegaVPN

**Релиз:** `7.1.1.15`

- **English README:** [README.md](README.md)
- **Лицензия:** Apache License 2.0. См. [LICENSE](LICENSE).
- **Репозиторий:** `github.com/rtis-emc2/megavpn`

RTIS MegaVPN - self-hosted платформа для централизованного управления VPN,
proxy и edge-инфраструктурой. Control Plane управляет удаленными nodes,
агентами, service instances, клиентским доступом, runtime artifacts,
маршрутизацией и аудитом.

```text
Operator
  -> Control Plane API / Web UI
  -> Worker queue
  -> Remote node agents
  -> Ingress / egress / runtime nodes
  -> VPN, proxy and edge services
```

## Документация

Начинать отсюда:

- [Индекс документации](docs/DOCUMENTATION_RU.md)
- [Documentation index](docs/DOCUMENTATION.md)
- [Руководство пользователя](docs/USER_GUIDE_RU.md)
- [User guide](docs/USER_GUIDE_EN.md)
- [Operations runbook](docs/OPERATIONS_RUNBOOK.md)
- [Release gates](docs/RELEASE_GATES.md)
- [Threat model](docs/THREAT_MODEL.md)
- [RBAC matrix](docs/RBAC_MATRIX.md)
- [Managed backhaul](docs/BACKHAUL.md)
- [Карта нод](docs/NODE_MAP_RU.md)
- [Учет трафика](docs/TRAFFIC_ACCOUNTING_RU.md)
- [Client access groups](docs/ACCESS_GROUPS_RU.md)
- [VLESS access groups](docs/VLESS_GROUPS_RU.md)
- [Выход через внешний VPN/proxy-провайдер](docs/EXTERNAL_EGRESS_RU.md)
- [Self-testing](docs/SELF_TESTING.md)
- [Roadmap и техническая спецификация](ROADMAP_V1_AND_TZ_RU.md)

## Назначение

RTIS MegaVPN предназначен для production-oriented эксплуатации распределенной
инфраструктуры доступа:

- подключение и сопровождение nodes;
- подписанный канал agent <-> Control Plane;
- создание service instances через service packs или ручную настройку;
- workflows для OpenVPN, WireGuard, Xray/VLESS, Shadowsocks, HTTP Proxy,
  MTProto, IPsec/L2TP и Nginx edge;
- управляемая связь ingress -> egress и projection route policy;
- provisioning клиентов, генерация конфигов, artifacts, share links и email;
- внутренний runtime binary repository;
- audit, diagnostics, backup/restore и release gates.

## Текущий статус релиза

Текущий релиз: `7.1.1.15`.

Подробные release notes и stabilization baseline ведутся в
[docs/releases/7.1.1.15.md](docs/releases/7.1.1.15.md). Release readiness gates
описаны в [docs/RELEASE_GATES.md](docs/RELEASE_GATES.md).

## Компоненты

| Component | Назначение |
| --- | --- |
| `cmd/api` | Control Plane API и Web UI backend |
| `cmd/worker` | Асинхронный orchestration worker |
| `cmd/agent` | Runtime agent на удаленной node |
| `cmd/migrate` | Runner миграций database |
| PostgreSQL | Persistent state store |
| Nginx | Публичный HTTPS edge |

## Production-принципы

- Публичный Control Plane доступен только через HTTPS.
- API должен слушать loopback за доверенным reverse proxy.
- Bootstrap credentials задаются явно, default password отсутствует.
- Secret master key хранится отдельно от database backups.
- Agents используют per-node identity и подписанные HTTP messages.
- Privileged operations выполняются через typed endpoints.
- Runtime artifacts фиксируются SHA-256 до установки на node.
- Backup/restore drill является частью release evidence.

## Быстрые локальные команды

```bash
go test ./...
go test -race ./...
go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate ./cmd/admin
scripts/ci/self-test.sh
```

Release evidence строже локальной диагностики:

```bash
scripts/ci/release-gate.sh
```

`scripts/ci/release-gate.sh` работает fail-closed. Он падает, если не хватает
production evidence: disposable PostgreSQL, backup/restore drill, systemd,
nginx, deployed API или service smoke matrix.

Исторические `scripts/*.sh` entrypoints сохранены как compatibility wrappers.
Новая автоматизация должна использовать `scripts/ci`, `scripts/smoke`,
`scripts/ops` и `scripts/lib`.

## Минимальный операторский поток

1. Установить Control Plane и применить migrations.
2. Настроить публичный HTTPS edge и production environment variables.
3. Создать первого оператора через явные bootstrap credentials.
4. Добавить nodes и enroll agents.
5. Проверить heartbeat, inventory и runtime capabilities.
6. Добавить runtime artifacts, если сервис нельзя безопасно установить из OS
   repositories.
7. Создать managed backhaul между ingress и egress nodes, если нужен remote
   egress.
8. Создать service instances из pack или вручную.
9. Применить instances и дождаться runtime convergence.
10. Создать clients, выбрать доступные входные services и выполнить
    provisioning.
11. Собрать client artifacts, проверить preview/download и при необходимости
    опубликовать share links или отправить email.
12. Мониторить Jobs, Audit, Runtime state и Backhaul health.

Полный процесс описан в [руководстве пользователя](docs/USER_GUIDE_RU.md).

## Security baseline

Security model описан в [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md).
Ключевые defaults:

- unsigned agent responses отклоняются обновленными agents;
- пустые job-poll `204` responses подписываются;
- job completion от agents требует текущий non-expired lease;
- agent file writes ограничены path roots, canonicalization и systemd unit
  allowlists;
- SSH bootstrap использует строгую проверку host-key fingerprint;
- public share links хранят token hashes, а не plaintext tokens.

## Лицензия

Apache License 2.0. См. [LICENSE](LICENSE).
