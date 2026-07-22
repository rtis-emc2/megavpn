# Выход через внешний VPN/Proxy-провайдер

**Релиз:** `7.1.1.1`

External egress profile подключает выбранные группы клиентов к коммерческому
или стороннему VPN/proxy-провайдеру. Он не заменяет managed Backhaul и не
перенаправляет весь трафик node.

English companion: [EXTERNAL_EGRESS.md](EXTERNAL_EGRESS.md).

## Назначение и границы

| Объект | Назначение | Какой трафик затрагивает |
| --- | --- | --- |
| Service instance | Принимает клиентский VPN/proxy-трафик | Клиенты, привязанные к inbound |
| Managed Backhaul | Связывает ingress и egress nodes MegaVPN | Управляемый node-to-node путь |
| External egress profile | Подключает runtime node к внешнему провайдеру | Только участники назначенных access groups |

Профиль глобальный. Deployment материализует его на конкретной runtime node.
Client access group один раз ссылается на профиль, но сам профиль должен быть
развернут на каждой node, где находится Xray/VLESS instance из scope группы.

```text
Выбранный клиент
  -> глобальная VLESS access group
  -> отдельный Xray outbound с managed fwmark
  -> отдельные ip rule и routing table
  -> внешний OpenVPN/WireGuard interface
  -> egress провайдера

Другой клиент или процесс node
  -> обычная Xray policy / main routing table node
```

Основная routing table node не меняется. Система не добавляет provider default
route `0.0.0.0/0` в таблицу `main`.

## Статус протоколов

| Протокол | Profile/import | Runtime на node | Активация |
| --- | --- | --- | --- |
| OpenVPN UDP/TCP | `.ovpn`, inline config, отдельные credentials и PEM | Готов | Разрешена |
| WireGuard | `wg-quick` `.conf`, inline или отдельный private key | Готов | Разрешена |
| SOCKS5 | Structured profile | Preview | Только draft |
| HTTP CONNECT / HTTPS proxy | Structured profile | Preview | Только draft |
| Shadowsocks | URL/JSON/structured catalog entry | Preview | Только draft |
| VLESS | URL/JSON/structured catalog entry | Preview | Только draft |
| L2TP | Structured catalog entry | Planned; без IPsec небезопасен | Только draft |
| L2TP over IPsec | Structured catalog entry | Planned | Только draft |
| IKEv2/IPsec | Structured/mobileconfig catalog entry | Planned | Только draft |
| Trojan | URL/JSON/structured catalog entry | Planned | Только draft |
| Hysteria 2 | URL/YAML/structured catalog entry | Planned | Только draft |

`Preview` и `Planned` означают, что data model уже зарезервирована, но runtime
job не может активировать протокол. Это fail-closed контракт, а не заявление о
production-поддержке.

Первая runtime-ready версия маршрутизирует IPv4. Provider default route для IPv6
пока не материализуется.

## Форматы credentials

### OpenVPN

Основной вариант - файл `.ovpn` от провайдера. Control Plane разбирает и
проверяет `remote`, `proto`, `dev`, `auth-user-pass` и ссылки на сертификаты.

Поддерживаются варианты:

- один inline `.ovpn` с CA, certificate, private key и credentials;
- `.ovpn` плюс отдельные login/password;
- `.ovpn` плюс отдельные CA certificate, client certificate и private key;
- `.ovpn` плюс отдельный `tls-auth` или `tls-crypt` key.

Внешние пути к файлам, scripts, plugins, management sockets, hooks и директивы
log/status отклоняются. Агент удаляет provider routes, задает managed TUN name и
добавляет `route-nopull`, `route-noexec`, `auth-nocache`.

Binary PKCS#12 пока не поддерживается как UI upload. Используйте inline PEM в
конфиге либо отдельные PEM certificate/private key.

### WireGuard

Provider `.conf` может содержать private key, либо ключ вводится отдельно.
Директивы `PreUp`, `PostUp`, `PreDown`, `PostDown` и `SaveConfig` отклоняются.

Для runtime-ready external egress в `AllowedIPs` требуется `0.0.0.0/0`.
MegaVPN заменяет `Table` на отдельную managed table и не применяет provider DNS
настройки к node.

### Structured draft profiles

Для preview/planned протоколов можно сохранить endpoint, port, transport,
login/password, UUID, PSK, certificate/private key как draft. Secrets шифруются,
но профиль нельзя применить до выпуска hardened runtime driver.

## Операторский workflow

1. Откройте `External egress`.
2. Нажмите `New profile`.
3. Выберите протокол провайдера.
4. Для OpenVPN/WireGuard вставьте или выберите provider config и выполните
   `Preview import`.
5. Добавьте отдельные credentials/certificate material, если preview показывает
   required secret.
6. Сохраните готовый профиль как `active`. Неготовые протоколы остаются `draft`.
7. Нажмите `Deploy` и выберите каждую runtime node, где есть Xray/VLESS instance
   из scope целевой группы.
8. Дождитесь apply job и выполните `Probe`.
9. Откройте `Clients -> Groups`, отредактируйте глобальную VLESS access group и
   выберите профиль в поле `External provider gateway`.
10. Выполните preview/sync группы и проверьте instance apply jobs.

При изменении runtime-полей или secrets активного профиля все его deployments
переходят в `pending`. До использования обновленного provider config необходимо
повторно выполнить `Apply` для каждого deployment. `Probe` является структурной
проверкой unit/interface/policy rule/route table и не доказывает доступность
произвольного адреса в Интернете через провайдера.

Не назначайте профиль группе до active deployment на всех nodes ее scope.
Materialization работает fail-closed: отсутствующий или unhealthy deployment
блокирует применение группы.

## Модель маршрутизации

Каждый deployment получает уникальные на своей node значения:

- interface в namespace `mgev*`;
- routing table из диапазона `40000..48999`;
- firewall mark из диапазона `0x4d590000..0x4d59ffff`;
- непересекающийся приоритет `ip rule`.

Выделение сериализуется PostgreSQL advisory transaction lock и защищено unique
constraints. Xray ставит mark только outbound-трафику выбранной access group.
Cleanup удаляет точное правило mark/table и очищает только таблицу deployment.

Пока group policy установлен, dedicated table всегда содержит high-metric
`unreachable default`. При исчезновении provider interface маркированный трафик
группы блокируется, а не продолжает lookup через main table node. Только явный
deployment `Cleanup` удаляет policy rule и dedicated table.

## Security model

- Provider configs и credentials шифруются в `secret_refs`.
- Read API возвращает только названия secret purposes, без plaintext.
- Plaintext передается только в `apply` job после claim конкретной node и
  проверки ownership deployment.
- `probe` и `cleanup` вообще не получают secrets.
- При ротации superseded encrypted secret удаляется транзакционно.
- Managed files имеют mode `0600`, route scripts - `0700`.
- Agent повторно валидирует deployment ID, interface, table и mark.
- Apply сначала проверяет либо устанавливает OpenVPN/WireGuard runtime package и
  только затем останавливает существующий deployment.
- Apply/probe/cleanup API требуют одновременно node write и access-group policy
  permissions.

## Диагностика

В Control Plane проверьте:

- profile имеет `active`, runtime support - `ready`;
- deployment имеет `active`;
- последний external-egress apply/probe job завершился успешно;
- group sync и связанные instance apply jobs успешны.

На node:

```bash
systemctl list-units 'megavpn-external-egress-*' --all --no-pager
systemctl status megavpn-external-egress-<deployment-id-без-дефисов>.service --no-pager -l
ip link show
ip rule show
ip route show table <40000-48999>
journalctl -u megavpn-external-egress-<deployment-id-без-дефисов>.service -n 200 --no-pager
```

Ожидаемое состояние:

- managed unit активен;
- interface `mgev*` существует;
- dedicated table содержит default route через этот interface;
- `ip rule` содержит mark и table deployment;
- в main table не появился provider default route.

## Ошибки и rollback

| Ошибка | Поведение | Восстановление |
| --- | --- | --- |
| Parser отклонил config | На node ничего не меняется | Убрать запрещенные директивы или добавить отдельные managed secrets |
| Не установился runtime package | Существующий deployment продолжает работать | Исправить OS repositories/capability и повторить apply |
| Новый runtime не стартовал | Apply job возвращает stage/output | Проверить journal, исправить profile, повторить apply |
| Provider interface отключился | Трафик выбранной группы отклоняется dedicated unreachable route | Восстановить provider runtime либо перевести группу на другой active profile |
| Group ссылается на отсутствующий deployment | Materialization группы блокируется | Deploy/apply profile на каждой scoped node |
| Провайдер недоступен | Probe/deployment получает failed/degraded | Восстановить провайдера либо перевести группу на другой active profile |

Для rollback трафика очистите `External provider gateway` в access group и
выполните sync/apply. Для удаления runtime выполните `Cleanup`. Profile можно
удалить только после снятия ссылок групп и перевода deployments в inactive.

Обязательный порядок удаления: снять profile со всех групп, синхронизировать
затронутые instances, выполнить `Cleanup` всех node deployments, отключить
profile и затем удалить его. PostgreSQL lifecycle guards обеспечивают те же
инварианты, что и API, в том числе при конкурентных запросах операторов.

## Observability, backup и scaling

Lifecycle profile/deployment и jobs видны в audit. Probe возвращает состояние
unit/interface/route/rule. Client traffic accounting остается привязанным к
client/service access; credentials и packet payload не логируются.

Database backup содержит зашифрованные provider secrets. Для restore требуется
тот же external secret master key. Runtime files на nodes являются materialized
state и после disaster recovery создаются повторно из Control Plane.

Один профиль может использоваться несколькими access groups. Количество jobs
масштабируется по deployments и затронутым Xray instances, а не по числу
участников группы. Capacity planning должен учитывать throughput provider
tunnel, CPU node и Xray outbound concurrency.
