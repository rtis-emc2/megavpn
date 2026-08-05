# Модель базы данных

**Релиз:** `8.0.0-pre.1`

Этот документ фиксирует поддерживаемую модель владения и целостности PostgreSQL.
Он является источником истины для изменений схемы и аудитов базы данных.

## Канонические сущности

| Назначение | Канонические таблицы | Runtime-проекция |
| --- | --- | --- |
| Группы клиентского доступа | `client_access_groups`, `client_access_group_memberships`, `client_access_group_instance_scopes` | `service_accesses`, ревизии instance и apply jobs |
| Членство VLESS | Таблицы групп доступа с `service_code='vless'` | Клиенты Xray на всех активных instance в scope группы |
| Вложения email | `client_email_delivery_artifacts`, `client_email_delivery_share_links` | Payload задания отправки |
| Секреты backhaul | `backhaul_transport_secrets` | Зашифрованные данные в `secret_refs` |
| Секреты внешнего egress | `external_egress_profile_secrets` | Зашифрованные данные в `secret_refs` |

Удалённые таблицы `vless_group_templates`, `vless_group_memberships` и
`client_access_group_migration_conflicts` не входят в поддерживаемую схему.
Единственный источник истины для членства клиентов — `client_access_groups`.

## Словари service code

Два разных словаря используются намеренно:

- `client_access_groups.service_code` — логический клиентский протокол, например
  `vless` или `l2tp`. Это стабильный идентификатор, но не FK на runtime-каталог.
- `service_definitions.code` — runtime-драйвер, например `xray-core`, `xl2tpd`
  или `ipsec`.
- `client_service_identities.service_code` относится к runtime-словарю и поэтому
  ссылается на `service_definitions(code)`.

Нельзя добавлять прямой FK между логическим протоколом и runtime-драйвером:
один логический протокол может обслуживаться несколькими драйверами.

## Гарантии целостности

- Ссылки instance на текущую и применённую ревизию защищены FK, а ownership
  triggers запрещают назначить ревизию другого instance.
- Actor в audit event ссылается на `platform_users`; при штатном удалении
  пользователя actor становится `NULL`, но событие аудита сохраняется.
- Composite FK в таблицах email-вложений не позволяет приложить артефакт или
  ссылку другого клиента.
- Backhaul и external egress используют типизированные join-таблицы секретов.
  Секретные значения остаются зашифрованными в `secret_refs`.
- `service_definitions.updated_at` автоматически поддерживается trigger.

Миграция `000028_schema_normalization` записывает количество и ограниченные
диагностические выборки неразрешённых исторических JSON-ссылок в `audit_events`,
затем удаляет legacy JSON-колонки. В выборках сохраняются идентификаторы, но не
секретные значения. Новые записи всегда защищены реляционными ограничениями.

## История нод и retention

Удаление ноды в application lifecycle является soft retirement:

1. Рабочие нагрузки и jobs штатно завершаются или явно force-retire.
2. Нода получает `status='retired'`.
3. История traffic accounting и inventory живёт до конца своей retention policy.

PostgreSQL по умолчанию блокирует физический `DELETE FROM nodes`, потому что
каскады уничтожили бы сохранённые счётчики и inventory evidence. Безвозвратный
purge является отдельной согласованной операцией DBA. Внутри явной транзакции
авторизованный администратор сначала выполняет:

```sql
set local megavpn.allow_node_delete = 'on';
```

Приложение никогда не устанавливает этот override.

## Семейства адресов

Address pools в этом релизе намеренно поддерживают только IPv4. Поддержка IPv6
в firewall не означает поддержку IPv6-адресов туннелей. Перед расширением
`address_pool_spaces.family` нужны allocator, overlap checks, route projection,
client artifacts и live data-plane tests для IPv6.

## Правила новых миграций

- Для boolean используем понятные имена без `is_`: `enabled`,
  `routing_enabled`, `auto_apply_new_instances`.
- Существующие стабильные колонки не переименовываются только ради стиля.
- Время хранится в `timestamptz`, адреса — в `inet`/`cidr`, когда это не
  непрозрачная строка провайдера, счётчики — в `bigint`.
- Стабильные связи сущностей оформляются FK и join-таблицами. JSONB предназначен
  для расширяемых payload/config metadata, а не для массивов UUID-ссылок.
- Для FK, участвующих в lookup или cleanup, обязателен индекс.

## Проверка

На пустой disposable PostgreSQL:

```bash
MEGAVPN_TEST_DATABASE_DSN='postgres://...' scripts/ci/postgres-migration-drill.sh
```

На обновлённом окружении:

```bash
MEGAVPN_DATABASE_DSN='postgres://...' scripts/ops/database-integrity-audit.sh
MEGAVPN_DATABASE_DSN='postgres://...' scripts/ops/database-integrity-audit.sh --validate-constraints
```

Вторую команду следует запускать только после нулевого количества critical rows
в первой проверке.
