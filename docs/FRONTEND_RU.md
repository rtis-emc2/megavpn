# Frontend Control Plane

**Релиз:** `8.0.0-pre.2`

Поддерживаемый интерфейс Control Plane - React/TypeScript-приложение в
`frontend/`. Второй параллельной реализации UI в проекте больше нет.

## Исходники И Сборка

| Путь | Назначение |
| --- | --- |
| `frontend/src` | Поддерживаемый исходный код приложения |
| `frontend/dist` | Воспроизводимая production-сборка в репозитории |
| `/opt/megavpn/web` | Runtime-каталог выкладки на Control Plane |

`frontend/go.mod` намеренно задает границу модулей: корневые Go-команды сборки
и тестирования не должны обходить npm-зависимости.

API отдает `index.html` только для известных frontend routes. API, agent,
public download и health endpoints никогда не попадают в SPA fallback.

```bash
cd frontend
npm ci
npm run typecheck
npm run lint
npm test
npm run i18n:check
npm run build
```

CI пересобирает `frontend/dist` и запрещает незакоммиченный diff. Изменение
исходников без соответствующего production bundle не может попасть в релиз.

## Deployment

`deploy-local.sh` запускает проверенный frontend installer. Он синхронизирует
`frontend/dist` в `/opt/megavpn/web` и удаляет устаревшие hashed assets.
Произвольный target по умолчанию запрещен, symbolic-link target отклоняется.
Если Git-синхронизация изменила revision, `deploy-local.sh` перезапускает себя
до сборки и установки. Старое тело уже запущенного скрипта поэтому не сможет
вызвать удаленный deployment entrypoint из новой revision.

Frontend и API работают на одном origin. Оператор не может указать браузеру
другой API origin, а authentication material не сохраняется в browser storage.

## Security Boundary

- Авторизация использует HttpOnly cookie Control Plane.
- Изменяющие состояние запросы требуют CSRF header, проверяемый API.
- Content Security Policy разрешает script, style и connection только с origin
  Control Plane.
- WebSocket ticket терминала принимается только для выбранной ноды и текущего
  origin Control Plane.
- CI запрещает HTML injection, browser secret storage, прямой `fetch` вне общего
  API client и ссылки на удаленный legacy UI.

## Rollback И Отказы

Откатывается релиз целиком: API binary и `frontend/dist`. Нельзя смешивать UI и
API разных версий. После прерванного deployment безопасно повторно запустить
`deploy-local.sh`: установка идемпотентна и удаляет устаревшие bundle-файлы.
Отсутствие `index.html` или asset directory останавливает deployment.
