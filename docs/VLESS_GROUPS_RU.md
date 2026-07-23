# VLESS Access Groups

**Релиз:** `7.1.1.17`

English companion: [VLESS_GROUPS.md](VLESS_GROUPS.md).

## Назначение

VLESS access groups - это reusable client access groups для клиентской
маршрутизации. Source of truth находится в `Clients -> Groups`: группа
назначается клиенту один раз, а runtime instances получают materialized
`service_accesses` как проекцию. Так политика доступа не размазывается по
каждому VLESS instance и остается аудируемой.

VLESS group глобальная. Участие клиента хранится один раз и материализуется во
все active Xray/VLESS instances, которые поддерживают эту group. VLESS instance
по-прежнему отвечает за listener, certificate/Reality settings и default egress
path; group определяет, что клиенту разрешено на уровне всей fleet.

## Архитектура

```text
Operator
  -> Clients / Groups
  -> client_access_groups table
  -> client_access_group_memberships table
  -> materialized service_accesses per Xray/VLESS instance
  -> Xray instance revision renderer
  -> node agent apply
  -> generated Xray routing rules
```

Control Plane хранит group policy и membership централизованно. Legacy
`vless_group_templates` остается compatibility catalog на один переходный
релиз, но новые операции членства используют generic
`client_access_groups`. Когда создается или обновляется revision Xray/VLESS
instance, активный group catalog встраивается в rendered instance spec. При
apply driver превращает groups в Xray routing rules, ограниченные конкретным
client user/email.

Сохранение, выключение или удаление global VLESS group также запускает catalog
sync для существующих Xray/VLESS instances. Sync создает validated instance
revision с текущим catalog и ставит `instance.apply` для active instances, чтобы
remote nodes получили новый routing config без ручного редактирования instance.

## Режимы

| Режим | Результат |
| --- | --- |
| `Instance default route` | Клиент использует instance-level default outbound. Это стандартный режим для remote egress через managed backhaul. |
| `Current node exit` | Клиент выходит с той node, которая приняла VLESS connection. Использовать только если local breakout нужен явно. |
| `Selected egress node` | Группа требует конкретную egress node. Apply должен резолвить этот выбор через managed routing/backhaul. |
| `Only selected instance` | Клиент может обращаться только к endpoint выбранного service instance. Система генерирует allow rule, остальной трафик блокируется. |
| `Block all traffic` | Трафик клиента запрещен. Подходит для quarantine, suspension или staged provisioning. |

`Block managed ad domains` добавляет Xray rule для managed advertising domain
set до финального outbound группы. В Xray runtime должны быть установлены
совместимые domain data.

## Операторский поток

### Каталог групп

1. Откройте `Clients`.
2. Перейдите в `Groups`.
3. Создайте или измените reusable VLESS access groups.
4. Для `Selected egress node` выберите egress node и убедитесь, что есть рабочий
   backhaul route.
5. Для `Only selected instance` выберите target service instance с валидным
   endpoint host и port.
6. Сохраните group.
7. Проверьте sync result. Любой failed instance будет показан со stage и
   validation error.
8. Откройте нужный VLESS instance в `Instances -> Manage`, если нужно проверить
   runtime materialization или выполнить re-apply.
9. Выберите `Default VLESS group`, если нужен instance-level fallback не
   `Default access`.
10. При provisioning клиента можно выбрать group для inbound, но основной
    массовый workflow должен идти через `Clients -> Groups`.

### Массовое назначение клиентов

Для fleet-операций не нужно открывать каждого клиента отдельно:

1. Откройте `Clients -> Groups`.
2. Нажмите `Members` у нужной group.
3. Найдите клиентов через server-side pagination. Page size выбирает оператор;
   UI больше не полагается на скрытый фиксированный limit.
4. Выберите scope назначения:
   - выбранные клиенты на текущей странице;
   - все клиенты, подходящие под текущий filter;
   - вставленные usernames, emails или client IDs;
   - любая комбинация этих режимов.
5. Выберите `Add only`, если существующих members нельзя переносить между
   groups, или `Add or move`, если existing members можно перевести в выбранную
   group.
6. Нажмите `Preview changes` и проверьте dry-run result:
   `will create`, `will move`, `will skip`, `will fail` и `apply job count`.
7. Нажмите `Apply previewed changes`.

Control Plane записывает желаемое состояние в
`client_access_group_memberships`, затем создает или обновляет materialized
`service_accesses` для всех active Xray/VLESS instances, в catalog которых есть
выбранная group. Повторное добавление клиента в ту же group идемпотентно, move
между группами сохраняет VLESS UUID, а bulk update ставит bounded
`instance.apply` jobs по affected instances, а не по одному apply job на
каждого клиента.

Legacy-секции `Instances -> VLESS groups` и `Instances -> Manage -> Applied
client access groups` остаются read-only/compatibility entry points. Они
показывают catalog/materialization и дают переход в `Clients -> Groups`; это не
означает, что membership принадлежит конкретному instance.

Client configs/artifacts не пересобираются по одному на каждого клиента из
bulk-flow. После изменения membership используйте обычный build/subscription
workflow, если оператору нужно обновить delivery artifacts.

## Правила validation

- Group key - стабильный identifier, его нельзя менять хаотично.
- `Selected egress node` требует egress node.
- `Only selected instance` требует target instance или explicit advanced rules.
- Deleted groups удаляются из будущих rendered revisions, а catalog sync ставит
  apply для active Xray/VLESS instances.
- Disabled groups остаются в catalog для audit/rollback context, но не
  предлагаются как active provisioning choices.
- Advanced route rules должны быть JSON array из Xray-compatible field rules.

## Runtime behavior

- Group data копируется в instance revision при instance save/create, во время
  global catalog sync и on demand при client provisioning. Это предотвращает
  ситуацию, когда свежая active group видна в provisioning UI, но отсутствует в
  revision выбранного Xray instance.
- Bulk membership хранит одну global desired group на клиента в
  `client_access_group_memberships`. Control Plane материализует это desired
  state в `service_accesses` для всех подходящих active Xray/VLESS instances и
  для новых подходящих instances, созданных позже.
- Client provisioning по-прежнему может создать direct VLESS access binding для
  выбранного inbound. Если global membership уже есть, materialization держит
  instance bindings в соответствии с этой global group.
- `VLESS group members` не создает duplicate rows. Уникальная VLESS identity
  клиента сохраняется в `client_service_identities` и переиспользуется при
  смене ingress nodes или переносе клиента между groups.
- Reprovisioning сохраняет group текущей client binding. Rotation Xray UUID
  работает на уровне identity profile: rotation одного VLESS access обновляет
  общий UUID в `client_service_identities` и переводит все active/pending Xray
  accesses этого клиента с тем же identity profile в `pending`, чтобы каждый
  affected ingress был пере-применен с новым credential. Пустой ввод group
  больше не превращается в synthetic `route`; stale implicit metadata
  fallback-ится в active catalog/default group, а явно выбранная неверная group
  остается validation error.
- Provisioning валидирует выбранную group после catalog sync. Если group не
  active или selected egress не резолвится через active backhaul, API возвращает
  available group keys и blocking resolution error.
- Apply генерирует Xray routing rules по client user/email.
- Когда instance-level или group-level remote egress резолвится в managed
  backhaul, driver записывает Xray `freedom.sendThrough` с ingress-side
  backhaul address. `node.route_policy.apply` дополнительно публикует system
  route для этого source address, чтобы локально сгенерированный Xray traffic
  уходил через выбранную egress node, а не через default route ingress node.
- Когда active managed backhaul transport меняется, Control Plane обновляет
  affected Xray instance revisions до route-policy apply. Так Xray outbound
  `sendThrough`, выбранный backhaul interface и kernel policy route сходятся в
  одном controlled convergence cycle.
- Перед применением route policy откройте diagnostics нужной ingress node и
  нажмите `Inspect route policy`. Preview покажет, активен ли VLESS/Xray system
  route, какую managed backhaul table/interface он использует и почему route
  может быть blocked. VLESS UUID-подобные source identities в preview
  редактируются, потому что это credential-like значения.
- Client binding, который ссылается на deleted или unknown group, при render
  fallback-ится в instance default group.
- `Only selected instance` создает allow rule к target endpoint и финальное
  block rule для остального трафика этой группы.
- Instance-level egress по-прежнему определяет default route для
  `Default access`.

## Риски

| Риск | Контроль |
| --- | --- |
| Клиент неожиданно выходит с ingress node | Используйте `Instance default route` плюс instance-level remote egress или принудительный `Selected egress node`. |
| Group изменена, но runtime не изменился | Save/status/delete запускает catalog sync и queue apply; reprovision/rotate сохраняет group binding, если она еще существует, и fallback-ится только для stale implicit metadata. |
| Target-only group не проходит validation | Проверьте endpoint host и port у target instance. |
| Ad blocking не работает | Проверьте Xray geosite data и generated routing rules. |
| Advanced JSON слишком широкий | Держите advanced rules свернутыми по умолчанию и используйте только после review. |

## Audit evidence

Оператор должен иметь возможность ответить:

- кто создал, изменил, выключил или удалил group;
- какие клиенты глобально назначены в group;
- какая VLESS instance revision содержит group catalog;
- какие materialized client bindings используют конкретный group key;
- какой apply job сгенерировал и развернул effective Xray config.

## Troubleshooting

| Симптом | Что проверить |
| --- | --- |
| Group не видна при provisioning | Убедитесь, что она active, и обновите core data. |
| Изменение group не влияет на runtime | Проверьте response изменения group на sync failures, затем queued `instance.apply` job для этого instance. |
| Target-only group не проходит validation | Проверьте endpoint host и port у target instance. |
| Remote egress не используется | Проверьте instance egress mode, выбранную egress node, active backhaul и route-policy sync. На ingress node в результате `node.route_policy.apply` должен быть active `xray_vless_remote_egress` system route, mark rule в `inet megavpn route_policy_output` и kernel rule `ip rule fwmark <mark> table <backhaul_table>`. |
| Ad blocking не работает | Проверьте Xray geosite data и generated routing rules. |
