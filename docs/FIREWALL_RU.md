# Каталог firewall-политик

**Релиз:** `7.0.1.20`

Firewall - это managed workspace для границ control-plane и node. Он специально
сделан как каталог перед применением: оператор готовит address lists,
упорядоченные rules и policies, затем ставит apply job на выбранную node.

English companion: [FIREWALL.md](FIREWALL.md).

## Операционная модель

Рабочий порядок:

1. Создать reusable address lists для операторов, доверенных сетей, VPN-пулов
   или заблокированных destinations.
2. Добавить entries в address lists. Тип можно оставить auto-detect для CIDR,
   одиночного IP или IP range.
3. Создать ordered rules внутри policy. Меньший priority применяется раньше.
4. Применить policy на node и проверить node firewall state.

Так редактирование отделено от rollout. Изменение каталога не меняет node, пока
apply job не поставлен в очередь и не завершился.

Инсталляции, обновленные с более раннего `7.0.1`, должны выполнить database
migrations до `000009_firewall_schema_repair` перед созданием address lists.
Если миграции не применены, API может вернуть
`relation "firewall_address_lists" does not exist`.

## Workflow в UI

Откройте `Firewall` в control menu.

- `Overview`: счетчики и общий posture.
- `Policies`: карточки policy, metadata default chain и быстрый apply.
- `Rules`: общий список правил по priority.
- `Address lists`: управление lists и entries.
- `Node state`: последнее состояние apply по каждой node.

Верхние workflow-кнопки переключают на нужный этап. В редакторе правил есть
presets для SSH management, HTTPS control, WireGuard, OpenVPN TCP/UDP, IPsec
IKE/NAT-T, L2TP, Shadowsocks TCP/UDP, HTTP proxy, MTProto, Nginx edge HTTP(S)
и drop invalid packets.

Вкладка `Policies` показывает posture каждой policy, default
input/forward/output actions и короткий preview правил. Вкладка `Rules`
содержит локальные filters по policy, chain, action и текстовый поиск по
CIDR/list/port/comment fields. Вкладка `Address lists` содержит локальный поиск
по metadata list и values entries.

Apply dialog разделен на два явных режима:

- `Rules only`: base chains остаются в `accept`; устанавливаются explicit
  catalog rules.
- `Strict defaults`: agent применяет default input/forward/output policies.

`Node state` показывает последний observed enforcement mode, число explicit
rules и число system safety rules, которые вернул agent.

## Security model

- `firewall.read` разрешает просмотр.
- `firewall.manage` разрешает менять policies, rules и address lists.
- `firewall.apply` разрешает ставить node apply jobs.
- Все create/update/delete/apply действия пишут audit events.
- Rules хранятся как catalog data и рендерятся worker-ом в managed firewall
  payload для node.

## Граница enforcement

По умолчанию apply job устанавливает explicit allow/drop/reject rules в managed
nftables chains, но оставляет base chain policy в `accept`. Это безопасный
staging mode для первого rollout и проверки каталога.

Strict default-policy enforcement доступен на каждый apply job через флаг
`enforce_default_policy` в API/UI. В strict mode agent атомарно заменяет
managed table `inet megavpn` через `nft -f`, пересоздает input, forward и
output base chains и применяет default policies:

- `accept` рендерится как base chain policy `accept`.
- `drop` рендерится как base chain policy `drop`.
- `reject` рендерится как base chain policy `drop` плюс terminal `reject`
  rule, потому что nftables base chain policy не поддерживает `reject`.

Agent также добавляет system safety rules для established/related traffic и
loopback перед catalog rules. Если output default policy равен `drop` или
`reject`, agent должен сохранить control-plane egress. Для этого он:

- генерирует TCP egress allow rule, если host control-plane URL у agent задан
  IP-адресом; или
- принимает explicit active catalog rule `output accept` для TCP-порта
  control-plane, если host control-plane URL задан DNS-именем.

Если ни одно условие не выполнено, render завершается ошибкой до изменения
nftables. Это защищает strict output rollout от тихой изоляции node.

DNS entries в address lists в этом релизе хранятся только как catalog context.
Node-side nftables apply рендерит CIDR, одиночные IP-адреса и IP ranges;
DNS-only list нельзя использовать как active matcher в rule.

Managed table принадлежит MegaVPN. Не размещайте hand-written rules в
`inet megavpn`; strict apply заменяет эту table как единый managed unit.

## Обработка ошибок

Если apply завершился ошибкой:

1. Откройте `Firewall -> Node state`.
2. Найдите failed node и последнюю policy.
3. Откройте `Jobs` для соответствующего `node.firewall.apply`.
4. Проверьте agent logs и rendered payload details.
5. Исправьте catalog rule и повторите apply.

Не делайте постоянные node-side firewall изменения вне managed catalog.
Временные emergency changes надо задокументировать и затем перенести в managed
policy rule.
