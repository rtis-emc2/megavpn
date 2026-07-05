# Каталог firewall-политик

**Релиз:** `7.0.1.3`

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

## Workflow в UI

Откройте `Firewall` в control menu.

- `Overview`: счетчики и общий posture.
- `Policies`: карточки policy, metadata default chain и быстрый apply.
- `Rules`: общий список правил по priority.
- `Address lists`: управление lists и entries.
- `Node state`: последнее состояние apply по каждой node.

Верхние workflow-кнопки переключают на нужный этап. В редакторе правил есть
presets для типовых случаев: SSH management, HTTPS control, WireGuard UDP и
drop invalid packets.

## Security model

- `firewall.read` разрешает просмотр.
- `firewall.manage` разрешает менять policies, rules и address lists.
- `firewall.apply` разрешает ставить node apply jobs.
- Все create/update/delete/apply действия пишут audit events.
- Rules хранятся как catalog data и рендерятся worker-ом в managed firewall
  payload для node.

## Граница enforcement в текущем релизе

Текущий релиз применяет explicit allow/drop/reject rules в managed nftables
chains. Поля default chain policy пока хранятся как rollout metadata; strict
default-policy enforcement надо включать только после controlled migration plan,
иначе оператор может заблокировать себе доступ.

DNS entries в address lists в этом релизе хранятся только как catalog context.
Node-side nftables apply рендерит CIDR, одиночные IP-адреса и IP ranges;
DNS-only list нельзя использовать как active matcher в rule.

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
