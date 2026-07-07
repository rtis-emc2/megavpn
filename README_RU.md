# RTIS MegaVPN

**Релиз:** `7.1.0.23`

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
- [VLESS access groups](docs/VLESS_GROUPS_RU.md)
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

`7.1.0.23` усиливает stabilization baseline после работ по lost-node cleanup:
форма SSH bootstrap access в Node management больше не перерисовывается
фоновым auto-refresh, пока оператор вводит access credentials, а runtime
accounting игнорирует stale agent reports для deleted instances и очищает
orphan runtime state/observation rows. Основной фокус:

- установка и обновление на чистом Ubuntu host;
- миграции PostgreSQL на disposable DB;
- подписанный agent channel с replay protection;
- typed privileged job APIs и permission matrix;
- bootstrap/update/emergency cleanup для nodes;
- создание и применение instances через packs/manual flow;
- централизованные VLESS access groups для client routing policy;
- понятный managed backhaul UX: один active transport ingress -> egress,
  optional standby transports только по явному выбору оператора и controlled
  standby promotion;
- traffic-camouflage Nginx/Xray ingress с явным fallback website и managed
  rollback при failed validation/apply;
- managed firewall catalog с explicit protocol presets и controlled
  default-policy enforcement для nftables apply;
- default node firewall baseline со strict input/forward deny, allow rules для
  HTTP/HTTPS edge, ICMP/ICMPv6 diagnostics, VPN client forwarding ranges и
  выделенной table `inet megavpn_firewall`;
- операторская firewall-диаграмма, которая связывает address groups, rules,
  policies, apply jobs и node apply status в один workflow от каталога до node;
- repair firewall schema для upgraded installations и упрощенный workflow
  address groups без internal identity fields;
- hardening layout для Firewall workspace: full-width tab panels, понятное
  разделение Address groups и entries, корректные счетчики tabs и row-scoped
  preview/apply actions для node;
- hardening firewall lifecycle: typed `node.firewall.disable`, idempotent
  agent removal для managed table `inet megavpn_firewall` и явный disabled
  state node после успешного completion;
- safety для bootstrap/update: SSH bootstrap и `Update all agents` не ставят
  job, если applied enforced firewall на node не содержит active input accept
  rule для настроенного SSH-порта;
- foundation учета трафика: signed agent ingest API, PostgreSQL aggregate
  storage, retention cleanup 180 дней, RBAC `traffic.read`, отдельная UI
  страница, managed Xray/WireGuard/OpenVPN byte-counter collectors и no-store
  CSV export для aggregate audit rows;
- bounded batched cleanup для retention учета трафика и PostgreSQL indexes под
  overview/export filters по bucket, client, node и protocol;
- operator visibility учета трафика: summary показывает total traffic,
  received/sent bytes, retained samples, clients, nodes, collectors и retention
  вместо внутренних prune-полей backend;
- Traffic Accounting использует верхние вкладки Overview, Clients, Collectors,
  Samples и Export, чтобы оператор переключал представления без длинного
  скролла;
- Traffic Accounting filters для date range, protocol, client, node и row limit
  теперь применяются backend-ом к overview summary, recent rows и no-store CSV
  export через одну retained-dataset query model;
- Traffic Accounting collector status показывает freshness по
  node/source/protocol, active/degraded/inactive streams, last report time,
  last bucket и aggregate client/sample coverage для выбранного retained
  dataset;
- expected collector coverage сравнивает active traffic-accounting-enabled
  Xray, WireGuard и OpenVPN runtime revisions с реально observed sample streams
  и показывает missing или partial retained sample streams в Traffic Accounting
  UI;
- semantic service-pack deduplication в API/UI и database repair для
  исторических duplicate default pack rows;
- VLESS client provisioning теперь синхронизирует active access-group catalog в
  выбранный Xray instance до проверки выбранной группы и materializes
  selected-egress groups в конкретные Xray outbound/source-route metadata;
- VLESS client identity теперь стабилен между Xray/VLESS ingress instances:
  provisioning клиента на новый ingress переиспользует уже выданный client UUID
  и ставит apply, чтобы новый сервер принял существующий клиентский credential;
- rotation Xray UUID сохраняет выбранную VLESS access group клиента вместо
  fallback в устаревшее implicit значение `route`; stale implicit group metadata
  fallback-ится в active catalog group, а явный неверный выбор оператора
  по-прежнему fail-closed;
- client provisioning action rows используют компактные scoped buttons вместо
  inherited full-width modal bars;
- resilient Nginx capability install для camouflage ingress: fallback с
  nginx.org на Ubuntu repo при repository/package-stage failures;
- safer Ubuntu Nginx fallback: existing package сохраняется до проверки distro
  candidate, а apt failures показывают точную failed command;
- selective service-pack creation: оператор может создать только выбранные
  components, переопределить listen ports на карточках и выбрать OpenVPN CA
  material без установки всех сервисов template-а;
- service-pack creation теперь имеет явное completed-состояние: отправленная
  форма сохраняется и блокируется после success, а оператор получает прямые
  actions для открытия созданных instances или отдельного нового rollout;
- hardening route-policy enforcement: ingress client traffic и локальный
  Xray/VLESS system egress маркируются через managed nftables chains и
  маршрутизируются по `fwmark` в managed backhaul tables;
- read-only route-policy preview для nodes: оператор может увидеть projected
  client routes, VLESS/Xray system egress routes, blocked/observe-only reasons
  и managed nft/ip-rule primitives до запуска `node.route_policy.apply`;
- telemetry результата `node.route_policy.apply`: после apply agent сохраняет
  state systemd unit/timer, `ip rule show` и managed nftables route-policy
  chains для диагностики VLESS/backhaul;
- route-policy/netpolicy nft comments теперь рендерятся как nft string
  literals, а route-policy fail-closed вместо маркировки traffic, если нет
  готового managed backhaul candidate;
- Xray/VLESS remote-egress convergence: при promote/enable/apply активного
  backhaul transport Control Plane сначала обновляет affected Xray revisions,
  чтобы `freedom.sendThrough` совпадал с выбранным живым backhaul interface, и
  только после успешного `instance.apply` ставит route-policy refresh;
- idempotent backhaul promote/enable теперь работает как managed repair-trigger
  для существующих Xray instances со stale remote-egress metadata;
- explicit route-policy cleanup: оператор может поставить typed node job,
  который останавливает route-policy runtime, удаляет managed `fwmark`
  rules/chains и чистит stale destinations из предыдущего snapshot ноды;
- TLS-enabled Nginx edge configs теперь генерируют HTTP listener, который
  отправляет обычный HTTP traffic на HTTPS до camouflage/fallback routing;
- Nginx instance и emergency cleanup теперь reload shared Nginx, если managed
  configs еще остались, и останавливают его, когда все MegaVPN-managed edge
  configs удалены;
- Nginx capability recovery теперь работает на стороне agent: урезанный systemd
  PATH все равно находит `/usr/sbin/nginx`, а Nginx instance apply может
  восстановить missing binary через managed nginx.org-to-Ubuntu fallback
  installer;
- UX для VLESS camouflage у клиентов: публичный client endpoint отделен от
  локального Xray backend endpoint, а pending provisioning state стал
  actionable;
- hard delete клиента с PostgreSQL cleanup coverage для service access, routes,
  generated artifacts, share links, subscriptions, delivery records и
  service-access scoped secrets;
- удаление instance теперь каскадно чистит client service-access после
  успешного managed cleanup на ноде, а отдельный stale service access можно
  удалить вручную из Client Access modal;
- operator-grade Firewall UI с posture cards, rule filters, grouped protocol
  presets и explicit apply modes;
- hardening типографики и layout operator console: единый UI font stack,
  безопасные переносы текста и мобильная grid-сетка вкладок;
- OpenVPN full-tunnel defaults с managed forwarding и NAT policy;
- smoke matrix для OpenVPN, WireGuard, Xray/VLESS, Shadowsocks, Nginx,
  Backhaul;
- provisioning клиентов и проверка route policy;
- диагностика jobs, runtime capabilities и service logs в интерфейсе.

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
go build ./cmd/api ./cmd/worker ./cmd/agent ./cmd/migrate
scripts/self-test.sh
```

Release evidence строже локальной диагностики:

```bash
scripts/release-gate.sh
```

`scripts/release-gate.sh` работает fail-closed. Он падает, если не хватает
production evidence: disposable PostgreSQL, backup/restore drill, systemd,
nginx, deployed API или service smoke matrix.

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
