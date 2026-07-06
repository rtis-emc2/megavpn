# VLESS-подписки

**Релиз:** `7.0.1.43`

VLESS-подписки дают клиентскому приложению live feed с профилями конкретного
клиента. Это способ доставки уже выданного VLESS-доступа, а не замена
provisioning, access groups или route policy.

English companion: [VLESS_SUBSCRIPTIONS.md](VLESS_SUBSCRIPTIONS.md).

## Модель безопасности

| Элемент | Поведение |
| --- | --- |
| Тип токена | Bearer token в URL подписки. Любой, у кого есть URL, может скачать feed до expiry или revocation. |
| Хранение | В базе хранится `token_hash` и `token_hint`; plaintext показывается только один раз после rotation. |
| Rotation | Новая rotation отзывает все активные subscription tokens этого клиента. |
| Revocation | Оператор может отозвать token в `Clients -> Access -> VLESS Subscription`. |
| Expiry | У token есть TTL. Просроченные token отклоняются и помечаются expired при чтении. |
| Cache control | Public response содержит `Cache-Control: no-store`. |
| Audit | Rotation и revocation создают audit events. |

## Поток данных

1. Оператор делает provisioning клиента и явно выбирает входные сервисы.
2. Provisioning создает `service_accesses` и service-specific metadata.
3. Для VLESS provisioning сохраняет client UUID в access metadata.
   Дополнительные VLESS ingress bindings для того же client переиспользуют
   существующий UUID, если оператор явно не запускает rotation этого access.
4. Оператор открывает client access и делает rotation VLESS subscription token.
5. UI показывает полный URL подписки один раз.
6. Public endpoint проверяет bearer token, статус клиента и token, затем строит
   feed из текущих активных VLESS service accesses.

Public endpoint:

```text
GET /subscribe/vless/{token}
```

По умолчанию он возвращает base64-encoded список URI, разделенных переносами
строк. Для диагностики можно добавить:

```text
?format=plain
```

## Когда профиль попадает в подписку

Service access попадает в feed только если выполнены все условия:

- client активен и не просрочен;
- subscription token активен и не просрочен;
- service access имеет статус `active`;
- service code instance равен `xray-core`;
- instance enabled и active;
- access metadata содержит сгенерированный VLESS UUID.

Если provisioning еще не завершился, профиль пропускается. Endpoint не
генерирует новый секрет во время скачивания подписки.

## Операторский workflow

1. Откройте `Clients`.
2. Создайте или выберите клиента.
3. Нажмите `Provision` и выберите один или несколько VLESS inbound instances.
4. Дождитесь завершения provisioning job.
5. Откройте `Access`.
6. В блоке `VLESS Subscription` нажмите `Rotate subscription`.
7. Сразу скопируйте сгенерированный URL.
8. Передайте URL пользователю через утвержденный канал доставки.

Если URL потерян, выполните rotation снова. Старый token будет отозван, новый
URL будет показан один раз.

## Типовые проблемы

| Симптом | Вероятная причина | Действие |
| --- | --- | --- |
| Feed пустой | Нет активного VLESS service access или provisioning не завершился | Повторите provisioning и проверьте наличие VLESS metadata. |
| `subscription token is not active` | Token отозван | Выполните rotation нового token. |
| `subscription token has expired` | TTL истек | Выполните rotation нового token. |
| `client is not active` | Client status изменен | Восстанавливайте статус только после проверки политики доступа. |
| Профиль импортируется, но трафик выходит не через нужную node | Это instance egress/backhaul policy, а не проблема подписки | Проверьте `Instances -> Manage`, состояние backhaul и access groups. |

## Эксплуатационные правила

- Не храните plaintext subscription tokens в логах, тикетах или audit payload.
- Для временного доступа используйте короткий TTL.
- Если доступ нужно полностью прекратить, отзывайте client access/client, а не
  только subscription token.
- Subscription rows бэкапятся вместе с database; там только hashes, но это все
  равно operational state.
