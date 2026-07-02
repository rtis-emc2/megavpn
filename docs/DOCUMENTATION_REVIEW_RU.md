# Ревью документации

**Релиз:** `7.0.1.2`

**Дата:** 2026-07-02

Этот review фиксирует текущую структуру документации, найденные проблемы и
исправления без изменения версии релиза.

English review: [DOCUMENTATION_REVIEW.md](DOCUMENTATION_REVIEW.md).

## Найденные проблемы

| Проблема | Влияние | Исправление |
| --- | --- | --- |
| README смешивал overview, status, changelog, operations и roadmap в одном длинном файле | Оператору было сложно понять, с чего начинать; у инженеров не было четких ownership boundaries | README переписан как краткая английская входная точка, добавлен `README_RU.md` |
| Русский и английский текст смешивались внутри одних и тех же разделов | Документацию было сложно сопровождать и ревьюить | Пользовательские документы переведены на paired English/Russian files |
| Не было единой карты документации | Новые операторы вынуждены были угадывать source of truth | Добавлены `docs/DOCUMENTATION.md` и `docs/DOCUMENTATION_RU.md` |
| Не было operator usage guide | UI flows описывались косвенно через release notes и status text | Добавлены `docs/USER_GUIDE_EN.md` и `docs/USER_GUIDE_RU.md` |
| Operator guide не покрывал установку на чистый host | Оператору приходилось восстанавливать installer/env/migration/systemd/nginx шаги из scripts | EN/RU guides расширены от установки до первичной настройки и runtime validation |
| Release gate не требовал новые user-facing docs | Документация могла регрессировать незаметно | Обновлены `docs/RELEASE_GATES.md` и `scripts/self-test.sh` |
| Roadmap, release evidence и operational procedures были плохо разделены | Был риск stale operational instructions в overview docs | README теперь ссылается на source-of-truth docs, а не дублирует runbooks |
| Текущий релиз не был виден в каждом основном документе | Оператор мог спутать текущий release baseline с историческими roadmap notes | Добавлен release banner `7.0.1.2` в поддерживаемые docs |
| Roadmap и next-step notes смешивали языки под базовыми именами файлов | Ownership документации был неочевиден для русской и английской аудитории | Roadmap и next-step notes разделены на английские defaults и `_RU` пары |
| VLESS access groups вынесены из instance manage, но docs описывали старый workflow | Оператор мог искать группы не в той вкладке или забыть re-apply VLESS instance | Добавлены парные документы по VLESS access groups и обновлен operator guide под `Instances -> VLESS groups` |

## Новая структура

| Документ | Назначение |
| --- | --- |
| `README.md` | Английский обзор продукта, component model и starting links |
| `README_RU.md` | Русский обзор продукта, component model и starting links |
| `docs/DOCUMENTATION.md` | Английский индекс документации, ownership и corporate rules |
| `docs/DOCUMENTATION_RU.md` | Русский индекс документации, ownership и corporate rules |
| `docs/USER_GUIDE_EN.md` | English operator guide |
| `docs/USER_GUIDE_RU.md` | Русский operator guide |
| `docs/OPERATIONS_RUNBOOK.md` | Production operations, backup, restore, upgrade and rollback |
| `docs/RELEASE_GATES.md` | Release acceptance criteria |
| `docs/SELF_TESTING.md` | Diagnostic and release-test commands |
| `docs/THREAT_MODEL.md` | Threat model and residual risks |
| `docs/RBAC_MATRIX.md` | Roles, permissions and privileged job rules |
| `docs/BACKHAUL.md` | Managed ingress-to-egress transport model |
| `docs/VLESS_GROUPS.md` | Английская модель VLESS access groups |
| `docs/VLESS_GROUPS_RU.md` | Русская модель VLESS access groups |
| `ROADMAP_V1_AND_TZ.md` | Product roadmap and technical specification |
| `ROADMAP_V1_AND_TZ_RU.md` | Русский roadmap и техническая спецификация |
| `docs/NEXT_STEPS.md` | Английская тактическая точка next-step |
| `docs/NEXT_STEPS_RU.md` | Русская тактическая точка next-step |

## Корпоративные правила

- `README.md` - английская входная точка; `README_RU.md` - русская входная
  точка.
- Длинные процедуры живут в runbooks или guides.
- User-facing workflows требуют русского и английского описания.
- Operational instructions должны содержать prerequisites, expected result и
  failure behavior.
- Security-sensitive workflows должны описывать trust boundaries и audit
  evidence.
- Примеры должны использовать нейтральные placeholders.
- Release evidence должен разделять passed, failed, skipped и waived checks.
- Поддерживаемые docs должны показывать текущий release banner.

## Оставшиеся gaps

Что нужно отслеживать до stable release:

1. Environment-specific installation appendices для external TLS/LB, managed
   PostgreSQL и offline install.
2. OpenAPI/public API contract.
3. Internal agent API contract.
4. Service-specific troubleshooting matrix на русском и английском.
5. Client configuration examples по каждому сервису.
6. Документация traffic camouflage и Nginx edge profile после реализации.

## Правила сопровождения

Каждая новая operator-visible feature должна обновлять:

1. `docs/DOCUMENTATION.md` и `docs/DOCUMENTATION_RU.md`, если добавляется новый
   документ или ownership area.
2. `docs/USER_GUIDE_EN.md` и `docs/USER_GUIDE_RU.md`, если меняется UI workflow.
3. `docs/THREAT_MODEL.md`, если меняются trust boundaries, secrets или public
   endpoints.
4. `docs/RELEASE_GATES.md`, если feature требует release evidence.
5. `docs/OPERATIONS_RUNBOOK.md`, если feature влияет на deployment, backup,
   restore, upgrade или incident response.
