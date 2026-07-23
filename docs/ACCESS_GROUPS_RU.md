# Client Access Groups

**Релиз:** `7.1.1.13`

English companion: [ACCESS_GROUPS.md](ACCESS_GROUPS.md).

## Назначение

Client access groups - это источник истины для назначения клиентов к политикам
доступа. Оператор управляет ими только в `Clients -> Groups`.

`Instances` не являются местом управления участниками groups. Instance - это
runtime target: он получает материализованные `service_accesses`, применяет
revision и разворачивает конфигурацию на node.

## Модель данных

```text
Clients -> Groups
  -> client_access_groups
  -> client_access_group_memberships
  -> service_accesses as runtime projection
  -> instance revision
  -> agent apply
```

| Объект | Назначение |
| --- | --- |
| `client_access_groups` | Глобальная политика доступа: service, key, route/policy, scope. |
| `client_access_group_memberships` | Желаемое членство клиента в группе. |
| `service_accesses` | Runtime-проекция для конкретных service instances. |
| `instances` | Listener/runtime, куда materialization попадает через revision/apply. |

## Service Catalog

Форма создания group использует `GET /api/v1/client-access-services`.
Каталог показывает все известные client access services:

| Service | Статус |
| --- | --- |
| VLESS / Xray | active, поддерживает groups, membership и materialization. |
| OpenVPN | coming soon/catalog-only в groups UI до включения materialization. |
| WireGuard | coming soon/catalog-only в groups UI до включения materialization. |
| L2TP/IPsec | coming soon/catalog-only в groups UI до включения materialization. |
| HTTP Proxy | coming soon/catalog-only. |
| SOCKS Proxy | planned/catalog-only. |
| Shadowsocks | coming soon/catalog-only. |
| MTProto | coming soon/catalog-only. |

Unsupported services видны оператору, но не применяются молча. Если service не
поддерживает безопасное создание groups, UI блокирует создание и backend
возвращает validation error.

## Операторский поток

1. Откройте `Clients -> Groups`.
2. Выберите service filter или оставьте `All services`.
3. Создайте group только для service, который поддерживает groups.
4. Откройте `Members`.
5. Загрузите клиентов через search/status/assignment filters и pagination.
6. Выберите visible clients, all filtered clients или вставленные
   usernames/emails/client IDs.
7. Нажмите `Preview`.
8. Проверьте create/move/skip/fail и affected instances.
9. Нажмите `Apply changes`.

`Apply` недоступен до успешного preview. Любое изменение selection, paste,
filter или mode инвалидирует preview.

## Runtime Behavior

- VLESS memberships материализуются во все active Xray/VLESS instances, catalog
  которых содержит выбранную group.
- Bulk assignment ставит bounded apply jobs по affected instances, а не по
  одному job на клиента.
- VLESS UUID сохраняется при переносе клиента между groups, потому что
  credential identity хранится отдельно от runtime projection.
- Новые подходящие VLESS instances получают существующие memberships через
  group materialization/sync.
- Instance detail может показывать applied access groups, materialized members,
  sync/apply state и ссылки на `Clients -> Groups`, но не должен добавлять
  участников напрямую.

## Security And Audit

- Group create/update/member changes требуют соответствующих RBAC permissions.
- Preview не должен мутировать PostgreSQL.
- Apply revalidates payload server-side.
- Unsupported services fail closed.
- Duplicate `service_accesses` не создаются.
- Audit trail должен отвечать: кто изменил group, кто изменил membership,
  какие instances были затронуты и какие jobs применили runtime state.

## Troubleshooting

| Симптом | Проверка |
| --- | --- |
| Service виден, но group нельзя создать | Service пока catalog-only или planned. Используйте VLESS или дождитесь materialization. |
| Apply недоступен | Сначала выполните Preview; затем не меняйте selection/paste/filter до Apply. |
| Клиент не появился в runtime | Проверьте affected instances в preview/apply result, sync state и `instance.apply` jobs. |
| У клиента остался старый UUID | Это ожидаемо: VLESS UUID является stable service identity и сохраняется при move. |
