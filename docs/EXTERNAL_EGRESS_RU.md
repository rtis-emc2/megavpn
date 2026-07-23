# Выход через внешний VPN/Proxy-провайдер

**Релиз:** `7.1.1.17`

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
Выбранный клиент с L3-провайдером
  -> глобальная VLESS access group
  -> отдельный Xray outbound с managed fwmark
  -> отдельные ip rule и routing table
  -> внешний OpenVPN/WireGuard/L2TP over IPsec interface
  -> egress провайдера

Выбранный клиент с proxy-провайдером
  -> глобальная VLESS access group
  -> отдельный Xray SOCKS outbound
  -> loopback-only SOCKS inbound provider Xray-процесса
  -> внешний VLESS или Shadowsocks provider

Другой клиент или процесс node
  -> обычная Xray policy / main routing table node
```

Основная routing table node не меняется. Система не добавляет provider default
route `0.0.0.0/0` в таблицу `main`.

## Доступные протоколы

Операторский список показывает только протоколы, для которых есть рабочий
runtime driver. Roadmap-состояния `planned`/`preview` во формы не попадают.

| Протокол | Profile/import | Runtime на node |
| --- | --- | --- |
| OpenVPN UDP/TCP | `.ovpn`, inline config, отдельные credentials и PEM | Готов |
| WireGuard | `wg-quick` `.conf`, inline или отдельный private key | Готов |
| Shadowsocks | SIP002 URL или strict JSON | Готов |
| VLESS | URI или strict JSON; TLS/REALITY обязателен | Готов |
| L2TP over IPsec | Отдельные поля подключения; PSK или certificate authentication | Готов |

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

### L2TP over IPsec

Форма отдельно запрашивает provider server, optional remote identity, L2TP/PPP
login и password, а затем способ IPsec-аутентификации:

- pre-shared key;
- CA certificate, client certificate и соответствующий RSA/EC private key.

Provider server принимает DNS name, IP или host-CIDR (`/32` для IPv4, `/128`
для IPv6). Сетевой CIDR с несколькими адресами не является endpoint одного
L2TP-сервера и отклоняется. Certificate chain, срок действия сертификата и
соответствие private key клиентскому сертификату проверяются до сохранения и
повторно на agent.

Node driver использует strongSwan, `xl2tpd` и `pppd`, назначает детерминированный
managed PPP interface и не устанавливает provider default route в main table.
На одной node разрешен один active external L2TP/IPsec deployment, поскольку
`xl2tpd` и UDP/1701 являются node-scoped ресурсами. Эта же node не может
одновременно содержать managed XL2TPD server instance; agent preflight также
блокирует apply, если UDP/1701 уже занят unmanaged-процессом.

### VLESS

Вставьте стандартный `vless://` URI либо strict JSON. UUID может находиться в
URI/JSON либо храниться отдельно как secret `uuid`. Provider connection должен
использовать TLS или REALITY; plaintext VLESS и `allowInsecure` отклоняются.
Поддерживаются TCP, WebSocket, gRPC, HTTP Upgrade и XHTTP с проверкой
совместимости transport/security. Public key, short ID, SNI, fingerprint,
path/host и service name переносятся в managed Xray config.

### Shadowsocks

Вставьте SIP002 `ss://` URI либо strict JSON. Password может быть встроен или
храниться отдельным secret. Разрешены только поддерживаемые Xray AEAD/2022
ciphers. SIP003 plugins и произвольные plugin options отклоняются: provider
config не может запускать локальные команды на node.

## Операторский workflow

Страница разделяет два уровня lifecycle:

- `Profiles` содержит переиспользуемые credentials провайдера, endpoint
  settings и действия `Deploy`, `Edit`, `Delete`.
- `Deployments` содержит состояние runtime на конкретной node, диагностику и
  действия `Apply`, `Probe`, `Cleanup`, `Reactivate`, `Remove`.

Во вкладке `Profiles` оператор определяет, что должно подключаться к
провайдеру. Во вкладке `Deployments` он управляет тем, где профиль установлен.
Поэтому ошибка runtime одной node больше не сжимает и не перекрывает строку
переиспользуемого профиля.

1. Откройте `External egress`.
2. Нажмите `New profile`.
3. Выберите один из доступных протоколов. Форма перестроится под него.
4. Для OpenVPN/WireGuard/VLESS/Shadowsocks вставьте или выберите provider config.
   Для L2TP/IPsec заполните отдельные connection и authentication fields.
5. Нажмите `Validate settings`, затем сохраните профиль как `Ready to deploy`.
   Внутренний `profile_key` система создаёт сама и оператор его не вводит.
6. Во вкладке `Profiles` нажмите `Deploy` и выберите каждую runtime node, где
   есть Xray/VLESS instance из scope целевой группы.
7. Откройте `Deployments`, дождитесь apply job и выполните `Probe`.
8. Откройте `Clients -> Groups`, отредактируйте глобальную VLESS access group и
   выберите профиль в поле `External provider gateway`.
9. Выполните preview/sync группы и проверьте instance apply jobs.

При изменении runtime-полей или secrets активного профиля все его deployments
переходят в `pending`. До использования обновленного provider config необходимо
повторно выполнить `Apply` для каждого deployment. `Probe` является структурной
проверкой unit/interface/policy rule/route table и не доказывает доступность
произвольного адреса в Интернете через провайдера.

`Cleanup` сохраняет provider profile и зашифрованные credentials, но переводит
deployment выбранной node в `inactive`. После этого `Reactivate` повторно
разворачивает тот же profile на node, а `Remove deployment` удаляет только
привязку profile к node. Удаление блокируется, пока cleanup не завершён или
deployment job ещё активен.

Не назначайте профиль группе до active deployment на всех nodes ее scope.
Materialization работает fail-closed: отсутствующий или unhealthy deployment
блокирует применение группы.

## Модель маршрутизации

Каждый L3 deployment получает уникальные на своей node значения:

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

VLESS и Shadowsocks deployment используют отдельный Xray-процесс с SOCKS
inbound, привязанным строго к `127.0.0.1` на детерминированном managed port.
Outbound выбранной access group подключается к этому loopback listener. Такие
proxy deployments не получают `ip rule`, не меняют main table и fail closed,
если provider process или listener недоступен.

## Security model

- Provider configs и credentials шифруются в `secret_refs`.
- Read API возвращает только названия secret purposes, без plaintext.
- Plaintext передается только в `apply` job после claim конкретной node и
  проверки ownership deployment.
- `probe` и `cleanup` вообще не получают secrets.
- При ротации superseded encrypted secret удаляется транзакционно.
- Managed files имеют mode `0600`, route scripts - `0700`.
- Agent повторно валидирует deployment ID, interface, table и mark.
- Apply сначала проверяет либо устанавливает OpenVPN, WireGuard,
  strongSwan/xl2tpd/pppd или Xray runtime и только затем заменяет deployment.
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
journalctl -u strongswan-starter -u strongswan -u xl2tpd -n 200 --no-pager
ss -lntp | grep '127.0.0.1:'
journalctl -u megavpn-external-egress-<deployment-id-без-дефисов>.service -n 200 --no-pager
```

Ожидаемое состояние:

- managed unit активен;
- L3 deployment имеет `mgev*`/managed PPP interface, dedicated default route и
  соответствующий fwmark `ip rule`;
- VLESS/Shadowsocks deployment слушает только назначенный `127.0.0.1` SOCKS
  port и не создает provider route в main table;
- в main table не появился provider default route.

Если L2TP/IPsec apply сообщает, что `dpkg --configure -a` не может настроить
`xl2tpd`, восстановите прерванную package transaction на проблемной node:

```bash
sudo dpkg --audit
sudo apt-get -o Acquire::Retries=3 -o Dpkg::Lock::Timeout=120 update
sudo env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a \
  apt-get -o Dpkg::Lock::Timeout=120 -f install -y
sudo env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a \
  dpkg --configure -a
sudo env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a \
  apt-get -o Dpkg::Lock::Timeout=120 install -y strongswan ppp xl2tpd
command -v ipsec pppd xl2tpd
sudo dpkg --audit
```

Управляемые package-операции выполняются в ограниченном по времени transient
systemd helper, а не напрямую внутри sandbox долгоживущего agent unit. Helper
разрешен только для фиксированных команд `apt-get` и `dpkg`, удаляется после
завершения и сохраняет имя unit и command output в job evidence. Основной
agent при этом сохраняет свои hardening-ограничения, а доверенные package
maintainer scripts операционной системы могут корректно завершиться.

При ошибке восстановления job result показывает последние содержательные
строки `apt`/`dpkg`. Перед изменением provider profile проверяйте полный job
evidence: недоступный repository, held packages и прерванная package
transaction относятся к состоянию ОС node, а не к provider credentials.

L2TP/IPsec Apply получает управление UDP/1701 на выбранной provider node. Он
останавливает системный `xl2tpd.service`, заменяет stale MegaVPN-managed L2TP
runtime и записывает отдельную XL2TPD/PPP/IPsec конфигурацию deployment.
Обычный Cleanup и emergency services cleanup удаляют managed unit, policy
route, runtime directory, временное состояние, MegaVPN certificate files и
точные глобальные IPsec include directives, когда управляемых L2TP deployment
больше нет.
До удаления runtime directory Cleanup проверяет сохраненный XL2TPD PID через
`/proc/<pid>/exe`, завершает только процесс, подтвержденный как `xl2tpd`, и
ждет освобождения UDP/1701. Apply выполняет такую же ownership-aware очистку
для stale managed deployments. Глобальное завершение процессов по имени не
используется, чтобы не остановить чужой сервис оператора.
Если старый Cleanup уже удалил PID file, Apply и Cleanup могут восстановить
PID listener через `ss`, но завершают его только когда executable и
`/proc/<pid>/cmdline` подтверждают MegaVPN-managed configuration или runtime
path. Emergency cleanup использует те же проверки и сохраняет runtime
evidence, если безопасно доказать ownership невозможно.
Системный daemon обрабатывается отдельно: после
`systemctl disable --now xl2tpd.service` оставшийся listener можно завершить
только если `/proc/<pid>/cgroup` по-прежнему указывает на точный unit
`xl2tpd.service`. Executable и ownership evidence повторно проверяются
непосредственно перед сигналом. Процесс из любого другого cgroup остается
нетронутым.

Если UDP/1701 остается занят, job завершается fail-closed и сохраняет в
evidence соответствующую строку `ss -lunp`. Ее можно проверить без изменения
runtime:

```bash
sudo ss -H -lunp | grep ':1701 '
sudo systemctl status xl2tpd 'megavpn-external-egress-*' --no-pager -l
```

Не завершайте найденный процесс, пока его принадлежность не подтверждена.

`Full node wipe` дополнительно удаляет executable packages `xl2tpd`, `ppp` и
strongSwan. Ошибка package purge считается ошибкой wipe: агент не удаляет сам
себя, поэтому оператор может восстановить apt/dpkg и повторить full wipe.
Обычный deployment cleanup сохраняет packages как повторно используемые node
capabilities.

Последний `dpkg --audit` не должен показывать unconfigured packages, а все три
binary должны находиться. После этого повторите `Apply` deployment. При этой
ошибке package manager не надо удалять profile или менять provider credentials.

## Ошибки и rollback

| Ошибка | Поведение | Восстановление |
| --- | --- | --- |
| Parser отклонил config | На node ничего не меняется | Убрать запрещенные директивы или добавить отдельные managed secrets |
| Не установился runtime package | Существующий deployment продолжает работать | Исправить OS repositories/capability и повторить apply |
| `xl2tpd` остался unconfigured | Apply завершается до замены runtime | Восстановить apt/dpkg по инструкции и повторить apply |
| UDP/1701 остается занят после managed teardown | Apply или Cleanup завершается ошибкой и показывает владельца listener | Проверить evidence `ss -lunp`, затем остановить или перенести только подтвержденный конфликтующий runtime |
| Full wipe не смог удалить L2TP packages | Wipe завершается ошибкой, agent сохраняется | Восстановить apt/dpkg, проверить job evidence и повторить full wipe |
| Новый runtime не стартовал | Apply job возвращает stage/output | Проверить journal, исправить profile, повторить apply |
| Provider interface отключился | Трафик выбранной группы отклоняется dedicated unreachable route | Восстановить provider runtime либо перевести группу на другой active profile |
| Group ссылается на отсутствующий deployment | Materialization группы блокируется | Deploy/apply profile на каждой scoped node |
| Провайдер недоступен | Probe/deployment получает failed/degraded | Восстановить провайдера либо перевести группу на другой active profile |

Для rollback трафика очистите `External provider gateway` в access group и
выполните sync/apply. Для удаления runtime выполните `Cleanup`. Inactive
deployment можно затем реактивировать или удалить отдельно. Profile можно
удалить только после снятия ссылок групп и перевода deployments в inactive либо
их удаления.

Обязательный порядок удаления: снять profile со всех групп, синхронизировать
затронутые instances, выполнить `Cleanup` всех node deployments, при
необходимости удалить node deployments, отключить profile и затем удалить его.
PostgreSQL lifecycle guards обеспечивают те же инварианты, что и API, в том
числе при конкурентных запросах операторов.

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
