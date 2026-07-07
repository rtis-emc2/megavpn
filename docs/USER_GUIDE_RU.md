# Руководство пользователя

**Релиз:** `7.1.0.12`

Документ описывает полный операторский путь RTIS MegaVPN: от установки Control
Plane на чистый сервер до настройки nodes, runtime capabilities, service
instances, backhaul, клиентов и клиентских artifacts.

## 1. Базовые понятия

| Термин | Значение |
| --- | --- |
| Control Plane | Центральный API/UI, который хранит состояние и управляет инфраструктурой. |
| Node | Сервер, на котором запускаются VPN/proxy/edge сервисы. |
| Agent | Приложение на node, которое получает jobs, применяет конфигурации и отправляет runtime reports. |
| Ingress node | Node, принимающая клиентские подключения. |
| Egress node | Node, через которую должен выходить клиентский трафик. |
| Service | Тип runtime: OpenVPN, WireGuard, Xray/VLESS, Shadowsocks, Nginx и другие. |
| Service pack | Шаблон, который создает один или несколько service instances с безопасными defaults. |
| Instance | Конкретный сервис на конкретной node: endpoint, spec, revision, runtime state. |
| Revision | Версия desired config для instance. Применять можно только apply-ready revision. |
| Runtime capability | Наличие нужного бинарника/пакета на node, например `openvpn`, `xray`, `ss-server`. |
| Backhaul | Управляемая связь ingress -> egress для удаленного выхода трафика. |
| Client | Клиентская учетная запись, для которой выбираются доступные входные сервисы. |
| Artifact | Сгенерированный клиентский конфиг или bundle. |
| Share link | Временная ссылка на artifact. Plaintext token показывается только один раз. |
| VLESS subscription | Per-client bearer URL, который возвращает текущие активные VLESS profiles. Plaintext token показывается только один раз после rotation. |

## 2. Подготовка сервера Control Plane

Минимальная production-модель:

- Ubuntu/Linux host с systemd.
- PostgreSQL database, доступная Control Plane.
- Публичный HTTPS endpoint.
- Nginx как TLS reverse proxy.
- Go toolchain для сборки из source checkout.
- Persistent storage для `/var/lib/megavpn/artifacts`.
- Secret master key вне database backup.

Базовые системные зависимости:

```bash
sudo apt-get update
sudo apt-get install -y git curl rsync openssl ca-certificates nginx postgresql-client
```

Если PostgreSQL работает на том же сервере, создайте database/user отдельно.
Для production предпочтителен TLS DSN с проверкой сертификата. `sslmode=disable`
допустим только для lab или доверенного local-only PostgreSQL.

## 3. Установка Control Plane

Рекомендуемый путь - `scripts/control-plane-install.sh`. Скрипт выполняет
полный bootstrap:

- проверяет параметры;
- при необходимости устанавливает базовые apt packages;
- копирует source tree в `/opt/megavpn`;
- создает `/etc/megavpn/megavpn.env`;
- создает или сохраняет `/etc/megavpn/master.key`;
- собирает binaries;
- устанавливает Web UI;
- устанавливает systemd units;
- запускает migrations;
- запускает API и worker;
- в режиме `self-signed-nginx` создает локальный HTTPS edge;
- выполняет health check.

Интерактивный запуск:

```bash
sudo ./scripts/control-plane-install.sh
```

Пример non-interactive запуска:

```bash
sudo MEGAVPN_CP_ASSUME_YES=1 \
  MEGAVPN_CP_TLS_MODE=self-signed-nginx \
  MEGAVPN_CP_PUBLIC_BASE_URL=https://control.example.com \
  MEGAVPN_CP_DATABASE_DSN='postgres://megavpn:password@127.0.0.1:5432/megavpn?sslmode=disable' \
  MEGAVPN_CP_ADMIN_USERNAME=superadmin \
  MEGAVPN_CP_ADMIN_EMAIL=admin@control.example.com \
  ./scripts/control-plane-install.sh
```

Проверить те же параметры без изменений на host:

```bash
sudo MEGAVPN_CP_VALIDATE_ONLY=1 \
  MEGAVPN_CP_ASSUME_YES=1 \
  MEGAVPN_CP_TLS_MODE=self-signed-nginx \
  MEGAVPN_CP_PUBLIC_BASE_URL=https://control.example.com \
  MEGAVPN_CP_DATABASE_DSN='postgres://megavpn:password@127.0.0.1:5432/megavpn?sslmode=disable' \
  MEGAVPN_CP_ADMIN_PASSWORD='replace-this-before-real-install' \
  ./scripts/control-plane-install.sh
```

Основные install variables:

| Variable | Назначение |
| --- | --- |
| `MEGAVPN_CP_PUBLIC_BASE_URL` | Публичный URL, который будут использовать браузер и agents. |
| `MEGAVPN_CP_TLS_MODE` | `self-signed-nginx`, `external-https` или lab-only `http-direct`. |
| `MEGAVPN_CP_DATABASE_DSN` | PostgreSQL DSN. |
| `MEGAVPN_CP_APP_DIR` | Каталог установки, по умолчанию `/opt/megavpn`. |
| `MEGAVPN_CP_ENV_FILE` | Runtime env file, по умолчанию `/etc/megavpn/megavpn.env`. |
| `MEGAVPN_CP_MASTER_KEY_PATH` | Secret master key path. |
| `MEGAVPN_CP_ARTIFACT_ROOT` | Persistent artifact storage. |
| `MEGAVPN_CP_ADMIN_USERNAME` | Bootstrap admin username. |
| `MEGAVPN_CP_ADMIN_EMAIL` | Bootstrap admin email. |
| `MEGAVPN_CP_ADMIN_PASSWORD` | Bootstrap admin password; если пусто, installer сгенерирует его. |
| `MEGAVPN_CP_RUN_TESTS` | Запустить `go test ./...` во время установки. |
| `MEGAVPN_CP_VALIDATE_ONLY` | Проверить параметры и выйти до изменений на host. |
| `MEGAVPN_CP_GO_TARBALL_URL` | Optional pinned Go toolchain tarball URL, если версия Go на host слишком старая. |
| `MEGAVPN_CP_GO_TARBALL_SHA256` | Обязательный SHA-256 pin, если задан `MEGAVPN_CP_GO_TARBALL_URL`. |

Runtime GeoIP variables в `/etc/megavpn/megavpn.env`:

| Variable | Назначение |
| --- | --- |
| `MEGAVPN_GEOIP_LOOKUP_URL_TEMPLATE` | HTTPS GeoIP URL template с `{ip}` placeholder; значение `disabled` отключает автоматическое определение нод на карте. |
| `MEGAVPN_GEOIP_TIMEOUT` | Timeout одного GeoIP lookup. |
| `MEGAVPN_GEOIP_AUTO_ENRICH_LIMIT` | Максимум нод, которые API дообогащает за один list request. |

Installer проверяет, что Go toolchain соответствует `go.mod`. Если версия Go на
host слишком старая, разрешите установку через OS package manager или задайте
pinned tarball URL вместе с SHA-256. Непривязанные downloads toolchain
отклоняются.

Если installer генерирует пароль, он сохраняет root-only файл:

```bash
sudo cat /root/megavpn-control-plane-admin.txt
```

После первого успешного входа и создания operator account уберите bootstrap
password из runtime environment или замените env file на версию без
`MEGAVPN_BOOTSTRAP_ADMIN_PASSWORD`, затем перезапустите API. Bootstrap env не
сбрасывает существующих пользователей, но хранить пароль в env дольше
нежелательно.

## 4. Ручная установка

Ручной путь нужен для контролируемых production-окружений, где installer не
должен сам ставить пакеты или писать Nginx config.

1. Скопируйте source tree в `/opt/megavpn`:

```bash
sudo install -d -m 0755 /opt/megavpn
sudo rsync -a --delete ./ /opt/megavpn/
cd /opt/megavpn
```

2. Создайте env:

```bash
sudo install -d -m 0750 /etc/megavpn
sudo install -m 0600 deploy/env/megavpn.production.env.example /etc/megavpn/megavpn.env
sudo editor /etc/megavpn/megavpn.env
```

3. Создайте master key:

```bash
sudo MEGAVPN_MASTER_KEY_PATH=/etc/megavpn/master.key scripts/generate-master-key.sh
```

4. Соберите binaries и Web UI. `scripts/build.sh` должен выполняться из
   `/opt/megavpn`, чтобы binaries оказались в `/opt/megavpn/bin`:

```bash
./scripts/build.sh
sudo ./scripts/install-web.sh /opt/megavpn/web
```

5. Установите systemd units:

```bash
sudo install -m 0644 deploy/systemd/megavpn-*.service /etc/systemd/system/
sudo systemctl daemon-reload
```

6. Запустите migrations:

```bash
sudo systemctl start megavpn-migrate.service
sudo systemctl status megavpn-migrate.service --no-pager -l
```

7. Запустите API и worker:

```bash
sudo systemctl enable --now megavpn-api.service megavpn-worker.service
sudo systemctl status megavpn-api.service megavpn-worker.service --no-pager -l
```

8. Настройте Nginx reverse proxy. Базовый пример:
   `deploy/nginx/megavpn-web.conf`.

```bash
sudo install -m 0644 deploy/nginx/megavpn-web.conf /etc/nginx/conf.d/megavpn-web.conf
sudo editor /etc/nginx/conf.d/megavpn-web.conf
```

Перед включением замените `server_name`, пути к сертификатам и
`X-Forwarded-Port`. Оставьте `Upgrade`/`Connection` headers из template: они
нужны для WebSocket terminal и долгоживущих browser connections.

9. Проверьте:

```bash
sudo nginx -t
sudo systemctl reload nginx
curl -fsS http://127.0.0.1:8080/healthz
```

## 5. Проверка после установки

После установки проверьте:

```bash
sudo systemctl status megavpn-api megavpn-worker --no-pager -l
sudo journalctl -u megavpn-api -u megavpn-worker -n 120 --no-pager
curl -fsS http://127.0.0.1:8080/healthz
```

В UI проверьте:

1. Login работает.
2. Dashboard открывается без 500 ошибок.
3. `Settings -> Control Plane TLS` содержит корректный public URL.
4. `/api/v1/ready` показывает `ready` только при корректном production preflight.
5. `Jobs`, `Nodes`, `Services`, `Instances`, `Clients`, `Backhaul`,
   `Certificates` открываются без ошибок.
6. `Instances` показывает вкладки workspace: список instances, create from
   pack, manual instance, service-pack catalog и VLESS groups.
7. `Instances -> Create from pack` показывает каталог service packs. Default
   templates создаются полным набором ordered migrations; если список пустой,
   проверьте, что все migrations применены к той же базе, которую использует
   API.
8. `Instances -> VLESS groups` показывает default groups для default route,
   current node exit, ad-blocked default и blocked access.

Если installer использовал self-signed TLS, замените его через:

1. `Certificates -> Add certificate`.
2. `Settings -> Control Plane TLS`.
3. Выбор imported/managed certificate.
4. `Apply edge`.
5. Проверка `nginx -t` и публичного HTTPS URL.

## 6. Первичная настройка системы

До добавления production nodes настройте:

- SMTP settings, если нужны invite/email delivery.
- Artifact root и backup policy.
- Secret master key backup policy.
- Operator roles и минимальные permissions.
- Control Plane TLS profile.
- Runtime binary repository для сервисов, которые нельзя ставить из OS repo.
- Address pools для OpenVPN/WireGuard/client networks.

Production defaults:

- `MEGAVPN_PRODUCTION_MODE=true`;
- `MEGAVPN_AGENT_ALLOW_AUTO_REGISTER=false`;
- `MEGAVPN_AGENT_SIGNATURE_ENFORCE=true`;
- `MEGAVPN_TRUST_PROXY_HEADERS=true` только за доверенным reverse proxy;
- API слушает loopback, публичный доступ идет через HTTPS edge.

## 7. Первый вход и readiness

1. Откройте публичный HTTPS URL Control Plane.
2. Войдите под операторской учетной записью.
3. Проверьте верхний правый статус:
   - `ready` означает, что API считает runtime preflight успешным;
   - degraded/blocked требует проверки Settings, Jobs или Runtime preflight.
4. Откройте Dashboard и убедитесь, что API, Jobs и Nodes отображаются без 500
   ошибок.

## 8. Добавление node

1. Откройте `Nodes`.
2. Нажмите `Add node`.
3. Укажите:
   - понятное имя;
   - role: `ingress`, `egress` или runtime-specific role;
   - публичный или management address;
   - setup method.
4. Для SSH bootstrap добавьте SSH access method:
   - `ssh_user`;
   - `ssh_host`;
   - `ssh_port`;
   - `ssh_host_key_sha256`;
   - private key secret.
5. Запустите bootstrap или enrollment flow.
6. Дождитесь heartbeat: node должна перейти в `online`.

`ssh_host_key_sha256` защищает bootstrap от MITM. Fingerprint должен
соответствовать реальному host key node.

После переустановки агента или ремонта host используйте `Nodes -> Node ->
Runtime reconcile`, чтобы поставить восстановление desired-state для managed
services, backhaul, route policy и существующей firewall policy. `Reboot node`
используйте только в controlled maintenance window: команду выполняет enrolled
agent, UI требует ввести имя node, а результат остается в audit/job history.

## 9. Runtime capabilities

Перед применением service instance node должна иметь нужный runtime:

- OpenVPN: `openvpn`;
- WireGuard: `wg`, `wg-quick`;
- Xray/VLESS: `xray`;
- Shadowsocks: `ss-server`;
- Nginx edge: `nginx`.

Workflow:

1. Откройте `Services`.
2. Выберите target node.
3. Нажмите `Verify`, чтобы получить фактическое состояние.
4. Если runtime отсутствует, используйте `Install runtime` или установку через
   runtime artifact.
5. После установки повторите `Verify`.

Если пакет нельзя надежно установить из OS repository, загрузите runtime в
`Runtime Binary Repository`. Control Plane сохранит artifact, рассчитает SHA-256
и выдаст agent короткоживущий signed download ticket.

## 10. Certificates и PKI

Есть два разных класса certificate material:

| Тип | Где используется |
| --- | --- |
| TLS edge certificate | Nginx/Xray TLS-facing endpoints, public HTTPS/SNI. |
| Service CA profile | OpenVPN CA root для server/client certificates. |

Для OpenVPN fleet, где несколько ingress nodes обслуживают один общий endpoint,
используйте общий OpenVPN CA profile. Тогда client certificates будут доверять
одному CA, а instance server certificates будут выпускаться из той же service CA.

Managed certificate в service-pack форме нужен для TLS-facing компонентов:
Nginx edge или Xray TLS. OpenVPN использует Service CA profile, а не TLS edge
certificate.

## 11. Address pools

Address pools должны быть централизованы. Оператор не должен вручную вспоминать,
какая подсеть свободна.

Рабочий принцип:

1. В разделе address pools задается базовый диапазон.
2. Система выделяет свободные подсети для OpenVPN/WireGuard/service instances.
3. Если свободных подсетей нет, оператор должен получить понятное сообщение:
   нужно добавить новый pool или расширить существующий.
4. Между pool можно включать или запрещать маршрутизацию согласно policy.

Default pool `remote_access_v4` создается текущим набором migrations.
Pack/manual specs с `address_pool_mode=auto` получают свободную подсеть из
catalog. Ручной CIDR используйте только как осознанное переопределение;
активные allocations блокируют удаление pool.

## 12. Managed backhaul

Backhaul нужен, когда вход находится на ingress node, а выход трафика должен
быть через egress node.

Workflow:

1. Откройте `Backhaul`.
2. Создайте link: ingress node -> egress node.
3. Выберите `Active backhaul transport`: это активный node-to-node transport для
   apply, health checks и route projection. Это не выбор клиентского
   VPN-протокола.
4. В `Optional standby transports` оставьте дополнительные транспорты
   выключенными, если не нужны fallback-профили, например OpenVPN, для
   последующего promotion или диагностики.
5. Нажмите `Apply`.
6. Дождитесь успешного apply на обеих сторонах.
7. Если active transport unhealthy, а standby показывает `standby ready`,
   откройте `Manage` и нажмите `Promote to active` на standby transport.
8. Нажмите `Test`.
9. Проверьте:
   - обе стороны `healthy`;
   - packet loss `0`;
   - latency видна;
   - route lookup идет через managed interface.

Backhaul apply и service instance apply - разные операции. Backhaul создает
transport между nodes. Instance route policy использует этот transport для
выхода клиентского трафика.

Если от одной ingress node к одной egress node есть несколько active backhaul
links, они работают как failover set. Control plane сортирует candidates по
`route_metric`: меньшая metric используется первой, большая остается backup.
Agent устанавливает все candidates в выбранную policy table и обновляет kernel
routes через systemd timer. Если candidate interface пропал или peer probe не
прошел, удаляется только этот route candidate; следующий route с большей metric
остается доступным для трафика.

### 12.1 Карта нод

Откройте `Node Map`, чтобы увидеть примерное расположение нод и overlay managed
backhaul. Координаты, страна, город, ASN и владелец сети определяются
автоматически по публичному адресу ноды через GeoIP. Оператор не вводит
координаты вручную.

Используйте карту для ориентации в топологии:

- node pins показывают GeoIP placement и role/status ноды;
- node cards показывают страну, город, владельца сети, источник GeoIP и
  связанные backhaul links;
- backhaul lines показывают drawable ingress-to-egress links, когда обе
  endpoint-ноды имеют координаты;
- topology list под картой показывает все non-deleted backhaul links:
  направление, driver, metric, endpoint и статус выбранного транспорта.

Для apply/probe/cleanup и transport diagnostics используйте страницу `Backhaul`.
Node Map - это визуальный topology view.

## 13. Создание service instances

Есть два способа.

### 13.1 Create from pack

Используйте для типовых production-baselines.

1. Откройте `Instances`.
2. Нажмите `Create from pack`.
3. Выберите service pack.
4. В `Services to create` оставьте отмеченными только те компоненты, которые
   нужно создать. Каждый выбранный компонент станет отдельным instance;
   снятые компоненты не создаются, не требуют runtime preflight и не отправляют
   свои per-service overrides. Используйте `Listen port` на карточке
   компонента, если default port конфликтует на выбранной node. Для OpenVPN
   можно использовать общий OpenVPN CA profile pack-а или задать override на
   конкретном выбранном компоненте.
5. Выберите node.
6. Укажите base name и endpoint host.
7. Выберите TLS edge certificate, если выбранные компоненты содержат
   Nginx/Xray TLS component.
8. Выберите OpenVPN CA profile, если выбранные компоненты содержат OpenVPN.
9. Если выбранные компоненты содержат traffic camouflage, настройте
   `Traffic camouflage`:
   - `Fallback website` обязателен и должен быть абсолютным `http://` или
     `https://` URL реального сайта. Его host не должен совпадать с публичным
     ingress endpoint;
   - если показан `Hidden VLESS path`, он не должен быть `/`, не должен
     содержать query/fragment и должен выглядеть как обычный asset/API path;
   - `Fallback Host header` и `Fallback SNI` можно оставить пустыми: control
     plane выведет их из fallback URL. Если они заданы явно, они не должны
     указывать обратно на ingress endpoint.
10. Если выбранные компоненты содержат VLESS, настройте instance-level
    `VLESS routing`:
   - `Auto through managed backhaul` для одного однозначного active backhaul;
   - `Use selected egress node`, если весь VLESS instance должен выходить через
     конкретную удаленную egress node;
   - `Local breakout on ingress node` только если прямой выход с ingress node
     действительно нужен.
11. Создайте instances.
12. Нажмите `Apply` или `Install + apply`, если runtime отсутствует.

Service pack не должен хранить runtime secrets. Пароли, private keys, UUID,
Reality keys и похожие secrets должны генерироваться на этапе revision/apply и
сохраняться как secret refs.

OpenVPN packs по умолчанию являются full-tunnel baseline: они отправляют
клиентам `redirect-gateway` и публичные DNS. Apply также материализует
managed network policy на node: IP forwarding и nftables `postrouting`
masquerade от OpenVPN client pool. Если нужен split-tunnel OpenVPN, удалите
redirect push lines и явно проверьте `nat_rules` перед применением revision.

Traffic camouflage packs создают два instances: Nginx public TLS edge и Xray
backend на `127.0.0.1`. Nginx проксирует только скрытый VLESS/gRPC path в Xray,
а обычный web-трафик на `/` reverse-proxy направляет на fallback website. Это
осознанная маскировка ingress-поведения, а не замена корректной TLS/SNI,
сертификатной и DNS-настройки endpoint.
Сгенерированные TLS-enabled Nginx edge configs могут слушать HTTP port `80` и
редиректить обычные HTTP requests на HTTPS до применения camouflage routing. В
форме instance это опция `Redirect HTTP to HTTPS`; redirect server name можно
оставить пустым, чтобы использовать основной `server_name`, или задать wildcard
например `*.example.com`, если один edge должен редиректить wildcard DNS.
Для repeatable smoke передавайте тот же fallback явно:
`MEGAVPN_FALLBACK_UPSTREAM_URL=https://target.example.com
scripts/service-pack-smoke.sh --matrix <node-id> <endpoint-domain>
[certificate-id]`. Matrix smoke пропускает camouflage packs, если значение не
задано: использовать сам ingress host как fallback нельзя, это может создать
proxy loop.
Чтобы тестировать протоколы партиями и не создавать лишние port conflicts на
одной node, ограничивайте matrix через `--packs` или `MEGAVPN_SMOKE_PACKS`:
`scripts/service-pack-smoke.sh --matrix <node-id> <endpoint-domain>
[certificate-id] --packs openvpn_tcp_11994,openvpn_udp_1194,wireguard_roadwarrior`.
Для временного исключения pack используйте `--exclude` или
`MEGAVPN_SMOKE_EXCLUDE_PACKS`. Перед реальным запуском используйте `--plan`
или `MEGAVPN_SMOKE_PLAN_ONLY=1`: smoke script покажет выбранные packs,
endpoint hosts, обязательные certificate/fallback условия и возможные
пересечения listen ports, но не создаст instances.
Для staged проверки всех основных протоколов используйте batch runner:
`scripts/service-pack-staged-smoke.sh --plan <node-id> <endpoint-domain> [certificate-id]`,
затем реальный запуск без `--plan`. Доступные партии: `remote_access_l3`
OpenVPN/WireGuard, `proxy_access` HTTP Proxy/MTProto/Shadowsocks,
`xray_reality`, `xray_nginx_http`, `xray_nginx_grpc` и `legacy_l2tp`
IPsec/L2TP. Обычный all-batches запуск на одной node требует `--cleanup`, так
как несколько партий используют public port 443; без cleanup runner остановится
до создания ресурсов. Ограничить запуск можно через
`--batches remote_access_l3,proxy_access`. Staged runner печатает путь
`staged_summary:` и пишет общий `_staged-summary.json` под evidence root; этот
файл показывает статус каждой партии и путь к ее `_matrix-summary.json`.
Переопределить путь можно через `MEGAVPN_SMOKE_STAGED_SUMMARY_FILE`.
Для повторных диагностических прогонов на одной disposable node можно включить
`MEGAVPN_SMOKE_CLEANUP=1`: после успешного smoke скрипт удалит созданного
smoke-клиента, его artifacts/share-links и дождется `instance.delete` jobs для
созданных instances. Если нужно автоматически убирать частично созданные
ресурсы после failed smoke, добавьте `MEGAVPN_SMOKE_CLEANUP_ON_FAILURE=1`.
Для release evidence cleanup лучше не включать, чтобы сохранить проверяемый
runtime след до ручного review.
Для machine-readable evidence задайте `MEGAVPN_SMOKE_EVIDENCE_DIR`, например
`MEGAVPN_SMOKE_EVIDENCE_DIR=tmp/service-pack-evidence`. Каждый успешный pack
запишет отдельный JSON с input-параметрами, created instances, runtime install
jobs, applied instance snapshots, runtime states, provision result и artifacts.
Matrix-run дополнительно пишет `_matrix-summary.json` с totals и строками
OK/FAILED/SKIPPED; путь можно переопределить через
`MEGAVPN_SMOKE_MATRIX_SUMMARY_FILE`. После matrix-run сформируйте offline
отчет по сохраненным файлам:
`scripts/service-pack-evidence-report.js tmp/service-pack-evidence/_matrix-summary.json`.
Для приемки конкретной партии добавьте
`--require-pack openvpn_tcp_11994,openvpn_udp_1194,wireguard_roadwarrior`;
скрипт завершится с ошибкой, если pack не дал OK evidence, runtime не ready
или у клиента нет active service access с ready artifact ожидаемого типа.
API, Web UI, Nginx renderer и smoke script отклоняют fallback URL/Host/SNI,
которые указывают на тот же публичный ingress host.
По умолчанию smoke script после apply каждого созданного instance также ждет,
что runtime projection станет `runtime_status=active`,
`health_status=healthy` и `drift_status=in_sync`. Используйте
`MEGAVPN_SMOKE_RUNTIME_CHECK=0` только для диагностики create/provision, где
runtime convergence намеренно не входит в проверку. Используйте
`MEGAVPN_SMOKE_REQUIRE_AGENT_REPORT=1`, если release evidence должен доказать,
что live systemd/listening-port state уже прислал агент, а не только
job-derived runtime projection.
Если на чистой node нет runtime capability, создание service pack может
поставить в очередь `runtime_install_jobs`. Smoke script ждет эти jobs перед
apply instance, поэтому matrix на чистой node проверяет convergence installer
flow, а не только уже предустановленные runtime. После client provisioning smoke
также ждет post-provision `instance.apply` jobs, которые доставляют клиентские
UUID/ключи в runtime, и проверяет, что каждый выбранный service access стал
`active` и получил свой ready artifact ожидаемого protocol type.

### 13.2 Manual instance

Используйте для точной настройки:

- дополнительный OpenVPN на той же node;
- отдельный VLESS endpoint;
- Nginx edge profile;
- кастомный Shadowsocks port;
- ручной route/network policy.

После изменения spec проверьте revision status. Draft revision нельзя применять,
пока validation не сделает ее apply-ready.

## 14. Apply и диагностика instance

Lifecycle:

1. `draft` - конфиг редактируется или не прошел validation.
2. `apply-ready` - revision можно применить.
3. `applying/provisioning` - job в очереди или выполняется.
4. `active` - desired и runtime state совпали.
5. `degraded` - сервис частично работает или есть runtime warning.
6. `failed` - apply/runtime validation завершились ошибкой.

Если сразу после создания instance виден `provisioning`, это не ошибка. Ошибка
должна показываться только после завершения job или явного failed runtime report.

В `Manage` должны быть видны:

- latest job;
- job timeline;
- service logs;
- runtime capability status;
- unit status;
- rendered config diagnostics без раскрытия secrets.

## 15. VLESS и egress

VLESS instance - это точка входа. Куда пойдет трафик дальше, должно решаться на
уровне instance/backhaul/route policy.

Правильная модель:

1. Клиент подключается к VLESS на ingress node.
2. Xray inbound принимает трафик.
3. Instance config выбирает default outbound:
   - local breakout, если выход с ingress допустим;
   - managed egress через backhaul, если трафик должен выйти с egress node.
4. Route policy и managed backhaul обеспечивают предсказуемый путь.

Где настраивать:

- `Backhaul`: создайте link `ingress -> egress`, нажмите `Apply`, затем `Test`.
- `Instances -> Create from pack`: если pack содержит VLESS, задайте
  `VLESS routing` до создания. Это записывает выбранный egress в каждый
  созданный Xray/VLESS instance и не меняет сам reusable service-pack template.
- `Instances -> Manage` для Xray/VLESS instance: выберите `Egress mode`.
  Используйте `egress node`, если весь VLESS instance должен выходить через
  удаленную egress node. Выберите конкретную `Egress node`, если link больше
  одного или нужен детерминированный выход.
- `Instances -> VLESS groups`: настройте reusable VLESS access groups один раз.
  Это политики доступа на этапе provisioning, а не просто названия:
  - `Use instance default route`: использовать egress-политику самого instance.
  - `Exit from current node`: принудительно выпускать трафик с текущей node.
  - `Exit from selected egress node`: при apply резолвить выбранную egress node
    через активный managed backhaul и создавать отдельный Xray outbound.
  - `Allow only selected instance`: разрешить пользователям группы доступ только
    к выбранному service instance endpoint, остальной трафик блокировать.
  - `Block all traffic`: полностью запретить трафик, например для quarantine
    или suspended access.
  - `Block ads`: добавить managed Xray rule `geosite:category-ads-all` для
    пользователей этой группы. В runtime Xray должны быть доступны geosite data.
  Сохранение, выключение или удаление группы автоматически синхронизирует
  catalog в существующие Xray/VLESS instances и ставит apply jobs для active
  instances. Если sync не прошел для конкретного instance, UI покажет stage и
  error.
- `Instances -> Manage` для Xray/VLESS instance: `Default VLESS group` задает
  группу, которая используется, если client binding не указал конкретную
  группу. Advanced JSON override специально свернут и нужен только для
  нестандартных transport experiments.
- `Clients -> Provision`: при выборе VLESS inbound можно выбрать access group.
  Эта группа сохраняется в client access binding и попадает в provisioning.
  Provisioning синхронизирует active catalog groups в выбранный Xray instance до
  validation, поэтому свежесозданную active group можно выбирать сразу. Если
  group disabled/deleted или selected egress не резолвится через active
  backhaul, API вернет available groups и blocking resolution error.
- `Clients -> Access -> Add route`: для отдельных route-policy правил можно
  указать `Egress mode = egress node` и выбрать egress node. После изменения
  route policy сначала выполните `Nodes -> Manage -> Inspect route policy`;
  если preview не показывает blocking warning для нужного пути, выполните
  `Sync route policy` на ingress node.
- `Nodes -> Manage -> Inspect route policy`: read-only projection для ingress
  node. Показывает enforceable routes, observe-only routes, VLESS/Xray system
  egress routes, blocked reasons, managed table/interface и nft/ip-rule
  primitives, которыми будет управлять agent. VLESS UUID-подобные source
  identities редактируются.
- `Nodes -> Manage -> Sync route policy`: ingress agent записывает signed
  snapshot и ставит managed kernel rules. Клиентский traffic маркируется в
  `inet megavpn route_policy_prerouting`; локальный Xray/VLESS traffic с
  `sendThrough` маркируется в `inet megavpn route_policy_output`; дальше
  `ip rule fwmark` отправляет marked flow в выбранную backhaul route table и
  интерфейс `mgbh*`. Job result содержит telemetry route-policy unit/timer,
  `ip rule show` и managed nftables chains.
- `Nodes -> Manage -> Clean route policy`: явный rollback managed
  route-policy runtime. Используйте его, если node убрали из ingress path,
  подозревается stale route-policy state после удаления instance/client, или
  предыдущий apply нужно очистить перед пересборкой. Операция удаляет только
  managed route-policy файлы MegaVPN, reserved `ip rule` priorities и managed
  nftables route-policy chains.

Для сценария “войти через VLESS и выйти с другой node” минимальный путь:

1. ingress node и egress node должны быть online.
2. Backhaul между ними должен быть `active` и успешно протестирован.
3. VLESS instance должен быть на ingress node.
4. Либо выберите `Use selected egress node` во время `Create from pack`, либо
   откройте `Instances -> Manage`, установите `Egress mode = egress node` и
   выберите нужную egress node.
5. Откройте `Instances -> VLESS groups`, если нужна client-specific группа.
   Например, создайте `Exit from selected egress node` для пользователей с
   конкретным egress или `Allow only selected instance` для ограниченного
   доступа.
6. Если меняли instance-level `Egress mode`, нажмите `Apply` у instance.
   Изменения самих VLESS groups распространяются автоматически через catalog
   sync.
7. Выполните `Inspect route policy` на ingress node и убедитесь, что
   VLESS/Xray system route активен и использует ожидаемую backhaul table и
   `mgbh*` interface.
8. Если используются client route rules, выполните `Sync route policy`.
9. Сначала проверьте telemetry в job result `node.route_policy.apply`. Если
   данных недостаточно, проверьте на ingress node:
   `nft list chain inet megavpn route_policy_output`,
   `nft list chain inet megavpn route_policy_prerouting`, `ip rule show` и
   `ip route show table <backhaul_table>`. Если job пишет `has no ready managed
   backhaul candidate`, выбранный transport не поднят или недоступен на ingress
   node; продвиньте healthy standby transport в active или повторно примените
   выбранный backhaul, затем re-apply Xray instance, чтобы `sendThrough`
   указывал на живой source address интерфейса `mgbh*`.

Подробная модель описана в [VLESS access groups](VLESS_GROUPS_RU.md):
режимы групп, runtime behavior и правила validation.

## 16. Клиенты и provisioning

1. Откройте `Clients`.
2. Создайте client account.
3. Нажмите `Provision`.
4. Выберите конкретные service instances, которые доступны клиенту.
5. Запустите provisioning job.
6. После постановки в очередь UI должен показать результат: job id, selected
   services, status и next action.
7. После успешного provisioning откройте client access.
8. Соберите artifacts: `.ovpn`, VLESS URL, WireGuard config, Shadowsocks URI или
   bundle.
9. Preview/download проверьте до отправки клиенту.
10. При необходимости создайте share link, выполните rotation VLESS
    subscription URL или отправьте email.
11. Для перевыпуска без удаления доступа используйте `Clients -> Access ->
    Client Configs -> Clear configs`, затем заново выполните `Build configs`.
12. Для полного удаления клиента используйте `Clients -> Delete`. Операция
    удаляет client account, service accesses, routes, generated configs,
    delivery links, VLESS subscriptions и service-access secret refs, после чего
    ставит apply jobs для затронутых service instances.

Provisioning не должен автоматически выдавать клиенту все совместимые services.
Оператор явно выбирает входные точки.

Для Xray/VLESS client UUID считается переиспользуемой service identity клиента.
Когда тот же client provision-ится на дополнительный VLESS ingress, Control
Plane переиспользует существующий UUID, записывает его в managed client list
нового instance и ставит instance apply. Доступ остается `pending`, пока agent
не подтвердит успешный apply; только после этого он становится `active`.

`Clear configs` не отзывает сам доступ: клиентские service bindings остаются, а
оператор может собрать новые artifacts. `Delete client` - необратимая операция
удаления клиента из runtime-модели; audit/job history при этом сохраняется для
traceability.

## 17. Share links, VLESS subscriptions и email

Share link - bearer URL. Его безопасность зависит от:

- высокой entropy token;
- expiry;
- revocation;
- `token_hash` в базе вместо plaintext token;
- audit events.

Plaintext token показывается только при создании ссылки. Если он потерян,
создайте новую ссылку.

VLESS subscription - это тоже bearer URL, но он не отдает статический artifact.
Endpoint собирает текущие активные VLESS service accesses клиента и возвращает
profile feed, разделенный переносами строк. Используйте его только после
успешного provisioning, потому что feed требует сгенерированный VLESS UUID из
service access metadata.

Операторский workflow:

1. Откройте `Clients -> Access`.
2. Убедитесь, что у клиента есть active VLESS inbound access.
3. В блоке `VLESS Subscription` нажмите `Rotate subscription`.
4. Сразу скопируйте сгенерированный URL. Plaintext token не хранится.
5. Отзовите subscription, если URL больше нельзя считать доверенным.

Подробности: [VLESS-подписки](VLESS_SUBSCRIPTIONS_RU.md) - lifecycle token,
типовые проблемы и public endpoint behavior.

## 18. Firewall policies

`Firewall` - это staged policy catalog, а не прямой редактор node-side правил.

Рекомендуемый workflow:

1. Откройте `Firewall -> Address lists` и создайте reusable source или
   destination lists.
2. Добавьте entries. Тип можно оставить auto-detect, если не нужно явно
   выбрать CIDR, single IP, range или DNS.
3. Откройте `Firewall -> Rules` и создайте ordered rules. Используйте presets
   для SSH, HTTPS, WireGuard, OpenVPN, IPsec/L2TP, Shadowsocks, HTTP proxy,
   MTProto, Nginx edge или invalid-packet случаев, затем уточните source lists
   и ports. Используйте filters правил, когда в catalog несколько policies или
   chains.
4. Откройте `Firewall -> Policies`, чтобы проверить defaults и количество rules.
5. Откройте `Firewall -> Node state` или apply action в policy и поставьте apply
   на выбранную node.

Изменения catalog вступают в силу только после завершения `node.firewall.apply`.
В apply dialog есть два режима:

- Default mode устанавливает explicit catalog rules и оставляет managed base
  chains в `accept`.
- Strict mode включает `enforce_default_policy` и применяет default
  input/forward/output actions из policy. Используйте его только после того,
  как добавлены management access rules и allow rules для нужных protocol
  listeners.

Если strict output default равен `drop` или `reject`, agent требует либо
IP-pinned control-plane URL, либо explicit active output accept rule для
TCP-порта control-plane. Если guard отсутствует, job падает на render stage до
изменения nftables.

## 19. Jobs, Audit и troubleshooting

`Jobs` показывает queue, status, result и failure reason.

Типовые сценарии:

| Симптом | Где смотреть | Что проверять |
| --- | --- | --- |
| Node offline | Nodes -> Manage | agent service, heartbeat, public URL, token |
| Runtime отсутствует | Services | результат capability verify/install |
| Apply завершился ошибкой | Instances -> Manage, Jobs | latest apply job, unit status, config validation |
| OpenVPN завис в activating | Instance logs, systemd state | config path, port, CA profile, unit status |
| Shadowsocks config не создан | Instance logs | generated config path, package install, password/spec |
| VLESS не использует egress | Instance config, Backhaul, route policy | default outbound, active backhaul, policy projection |
| Backhaul завершился ошибкой | Backhaul modal, Jobs | ingress/egress side, interface, route lookup, packet loss |
| Firewall apply завершился ошибкой | Firewall -> Node state, Jobs | rendered policy, agent logs, nftables support |
| Client config некорректен | Clients -> Access/Artifacts | selected services, revision applied, artifact build result |

Audit должен отвечать на вопросы:

- кто создал или изменил node;
- кто запустил bootstrap/update/cleanup;
- кто применил instance;
- кто установил runtime capability;
- кто создал/revoked share link;
- кто сделал rotate/revoke VLESS subscription;
- кто изменил или применил firewall policy;
- кто изменил settings/certificates.

## 20. Безопасное удаление

Удаление instance не должно быть только удалением строки из базы.

Правильный порядок:

1. Revoke client access, который использует instance.
2. Stop/disable instance, если требуется.
3. Delete instance через UI/API.
4. Дождаться `instance.delete` cleanup job на node.
5. Проверить, что systemd unit, config files и managed policy удалены.

Аварийная очистка node:

- требует явного подтверждения именем node;
- может удалить только managed state;
- опционально может удалить agent;
- не должен ломать unrelated backhaul/routes за пределами managed scopes.

## 21. Production checklist

Перед production rollout:

1. `scripts/release-gate.sh` без unexplained skips.
2. Disposable PostgreSQL migration test.
3. Backup/restore drill.
4. `nginx -t` на edge host.
5. `systemd-analyze verify` для systemd units.
6. Agent enrollment на тестовой node.
7. Service smoke matrix.
8. Backhaul apply/probe.
9. Client provisioning и artifact preview/download.
10. Audit review.
11. Rollback plan.

## 22. Роли

Кратко:

- `readonly`: чтение состояния и audit.
- `engineer`: clients/artifacts/share links без node/bootstrap/apply authority.
- `admin`: эксплуатация nodes/instances/jobs/settings без unrestricted secret reveal.
- `superadmin`: полный набор permissions.

Подробно: [RBAC matrix](RBAC_MATRIX.md).
