# VLESS Access Groups

**Релиз:** `7.0.1.1`

English companion: [VLESS_GROUPS.md](VLESS_GROUPS.md).

## Назначение

VLESS access groups - это reusable templates для клиентской маршрутизации. Они
настраиваются один раз в `Instances -> VLESS groups`, а затем выбираются при
provisioning клиента. Так политика доступа не размазывается по каждому VLESS
instance и остается аудируемой.

VLESS instance по-прежнему отвечает за listener, certificate/Reality settings и
default egress path. VLESS group определяет, что разрешено конкретной client
binding поверх этого instance.

## Архитектура

```text
Operator
  -> Instances / VLESS groups
  -> vless_group_templates table
  -> Xray instance revision renderer
  -> node agent apply
  -> generated Xray routing rules
  -> client provisioning bindings
```

Control Plane хранит group templates централизованно. Когда создается или
обновляется revision Xray/VLESS instance, активный group catalog встраивается в
rendered instance spec. При apply driver превращает groups в Xray routing rules,
ограниченные конкретным client user/email.

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

1. Откройте `Instances`.
2. Перейдите в `VLESS groups`.
3. Создайте или измените reusable groups.
4. Для `Selected egress node` выберите egress node и убедитесь, что есть рабочий
   backhaul route.
5. Для `Only selected instance` выберите target service instance с валидным
   endpoint host и port.
6. Сохраните group.
7. Откройте нужный VLESS instance в `Instances -> Manage`.
8. Выберите `Default VLESS group`, если нужен default не `Default access`.
9. Примените VLESS instance, чтобы текущий group catalog попал в Xray config.
10. При provisioning клиента выберите нужную group для каждого VLESS inbound.

## Правила validation

- Group key - стабильный identifier, его нельзя менять хаотично.
- `Selected egress node` требует egress node.
- `Only selected instance` требует target instance или explicit advanced rules.
- Deleted groups удаляются из будущих rendered revisions, но уже примененные
  configs не меняются до save/apply VLESS instance.
- Disabled groups остаются в catalog для audit/rollback context, но не
  предлагаются как active provisioning choices.
- Advanced route rules должны быть JSON array из Xray-compatible field rules.

## Runtime behavior

- Group data копируется в instance revision при save/create.
- Client provisioning сохраняет выбранный group key в client access binding.
- Apply генерирует Xray routing rules по client user/email.
- `Only selected instance` создает allow rule к target endpoint и финальное
  block rule для остального трафика этой группы.
- Instance-level egress по-прежнему определяет default route для
  `Default access`.

## Риски

| Риск | Контроль |
| --- | --- |
| Клиент неожиданно выходит с ingress node | Используйте `Instance default route` плюс instance-level remote egress или принудительный `Selected egress node`. |
| Group изменена, но runtime не изменился | После изменения group повторно примените затронутые VLESS instances. |
| Target-only group не проходит validation | Проверьте endpoint host и port у target instance. |
| Ad blocking не работает | Проверьте Xray geosite data и generated routing rules. |
| Advanced JSON слишком широкий | Держите advanced rules свернутыми по умолчанию и используйте только после review. |

## Audit evidence

Оператор должен иметь возможность ответить:

- кто создал, изменил, выключил или удалил group;
- какая VLESS instance revision содержит group catalog;
- какие client bindings используют конкретный group key;
- какой apply job сгенерировал и развернул effective Xray config.

## Troubleshooting

| Симптом | Что проверить |
| --- | --- |
| Group не видна при provisioning | Убедитесь, что она active, и обновите core data. |
| Изменение group не влияет на runtime | Повторно примените затронутый VLESS instance. |
| Target-only group не проходит validation | Проверьте endpoint host и port у target instance. |
| Remote egress не используется | Проверьте instance egress mode, выбранную egress node, active backhaul и route-policy sync. |
| Ad blocking не работает | Проверьте Xray geosite data и generated routing rules. |
