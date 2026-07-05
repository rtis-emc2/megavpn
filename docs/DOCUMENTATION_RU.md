# Индекс документации

**Релиз:** `7.0.1.8`

Этот документ - русская входная точка в документацию RTIS MegaVPN. Он фиксирует,
где находится источник правды по каждому направлению: эксплуатация,
безопасность, релизы, маршрутизация, клиенты и troubleshooting.

English entry point: [DOCUMENTATION.md](DOCUMENTATION.md).

## Рекомендуемый порядок чтения

| Порядок | Документ | Назначение |
| --- | --- | --- |
| 1 | [README_RU](../README_RU.md) | Обзор продукта и модель компонентов |
| 2 | [Руководство пользователя](USER_GUIDE_RU.md) | Полный операторский guide |
| 3 | [Operations runbook](OPERATIONS_RUNBOOK.md) | Эксплуатация, backup, restore, upgrade, rollback |
| 4 | [Release gates](RELEASE_GATES.md) | Release evidence и критерии приемки |
| 5 | [Self-testing](SELF_TESTING.md) | Локальные и live diagnostic gates |
| 6 | [Threat model](THREAT_MODEL.md) | Security model и остаточные риски |
| 7 | [RBAC matrix](RBAC_MATRIX.md) | Роли, permissions и privileged job rules |
| 8 | [Managed backhaul](BACKHAUL.md) | Модель связи ingress -> egress |
| 9 | [Карта нод](NODE_MAP_RU.md) | GeoIP-размещение нод и overlay managed backhaul |
| 10 | [Каталог firewall-политик](FIREWALL_RU.md) | Managed firewall policies, address lists, rules и node apply state |
| 11 | [VLESS access groups](VLESS_GROUPS_RU.md) | Reusable VLESS client routing groups |
| 12 | [VLESS-подписки](VLESS_SUBSCRIPTIONS_RU.md) | Per-client VLESS subscription tokens и workflow доставки |
| 13 | [Roadmap](../ROADMAP_V1_AND_TZ_RU.md) | Roadmap и техническая спецификация |
| 14 | [Next steps](NEXT_STEPS_RU.md) | Текущая engineering-точка |
| 15 | [Security review](SECURITY_REVIEW_7.0.1.8.md) | Security и release review artifact |
| 16 | [English roadmap](../ROADMAP_V1_AND_TZ.md) | Английская версия roadmap |

## Владение документацией

| Направление | Источник правды | Примечания |
| --- | --- | --- |
| Product overview | `README.md`, `README_RU.md` | README должен оставаться краткой входной точкой. |
| Operator usage | `docs/USER_GUIDE_EN.md`, `docs/USER_GUIDE_RU.md` | Английская и русская версии должны быть синхронизированы. |
| Operations | `docs/OPERATIONS_RUNBOOK.md` | Production procedures и controlled maintenance. |
| Release readiness | `docs/RELEASE_GATES.md`, `docs/SELF_TESTING.md` | Release evidence, self-test и smoke gates. |
| Security | `docs/THREAT_MODEL.md`, `docs/RBAC_MATRIX.md` | Threat model, roles, permissions и privileged jobs. |
| Backhaul/routing | `docs/BACKHAUL.md` | Managed links, route projection и troubleshooting. |
| Topology | `docs/NODE_MAP.md`, `docs/NODE_MAP_RU.md` | GeoIP-размещение нод, локальная статичная карта, node owner metadata и backhaul overlay. |
| Firewall | `docs/FIREWALL.md`, `docs/FIREWALL_RU.md` | Managed firewall catalog, address lists, rules и node apply state. |
| VLESS client routing | `docs/VLESS_GROUPS.md`, `docs/VLESS_GROUPS_RU.md` | Reusable access groups, default VLESS group selection и provisioning behavior. |
| VLESS subscriptions | `docs/VLESS_SUBSCRIPTIONS.md`, `docs/VLESS_SUBSCRIPTIONS_RU.md` | Per-client bearer-token rotation, public feed behavior и operator delivery workflow. |
| Roadmap | `ROADMAP_V1_AND_TZ.md`, `ROADMAP_V1_AND_TZ_RU.md`, `docs/NEXT_STEPS.md`, `docs/NEXT_STEPS_RU.md` | Strategic и tactical product planning. |

## Языковая политика

- `README.md` пишется только на английском.
- `README_RU.md` пишется только на русском.
- Английские документы используют базовое имя файла.
- Русские парные документы используют суффикс `_RU.md`.
- Исторические приложения могут сохранять доменную терминологию, но
  поддерживаемые входные точки должны оставаться языково разделенными.
- Каждый пользовательский workflow должен иметь русское и английское описание до
  того, как он считается production-ready.

## Корпоративные правила

- README остается краткой входной точкой, а не changelog.
- Операционные документы должны содержать prerequisites, команды, expected result
  и rollback/failure behavior.
- Security-sensitive процедуры должны явно описывать trust boundary и audit
  evidence.
- Release-документы должны разделять `PASS`, `FAIL`, `SKIP` и waiver.
- В примерах используются нейтральные placeholder domains:
  `control.example.com`, `edge.example.com`, `vpn.example.com`.
- Если в Control Plane есть managed workflow, документация не должна предлагать
  ручное изменение node как основной путь.
- Каждый поддерживаемый документационный файл должен содержать release banner:
  `7.0.1.8`.

## Текущие gaps документации

Что нужно закрыть до stable release:

- OpenAPI/public API contract.
- Internal agent API contract.
- Environment-specific install appendices для external TLS/LB, managed
  PostgreSQL и offline install.
- Двуязычная troubleshooting matrix по всем сервисам.
- Service-specific примеры клиентских конфигов.
