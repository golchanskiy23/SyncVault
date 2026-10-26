# SyncVault

Фоновая синхронизация папок между вашим компьютером, другими Linux-устройствами/серверами и Google Drive.

Сервер запускается на вашем компьютере в Docker. Вы создаёте **связки** — «папка здесь ↔ папка там», —
и сервер синхронизирует каждую связку раз в `sync.interval` (по умолчанию раз в час) в обе стороны.

Стороны связки:
- **local** — папка этого компьютера (внутри `SYNC_LOCAL_ROOT`)
- **агент** — папка на другом Linux-устройстве или сервере, где запущен агент SyncVault
- **Google Drive** — папка на Drive подключённого аккаунта

Синхронизация работает через алгоритм:
1. **MerkleTree** — быстро находит расходящиеся файлы между двумя сторонами
2. **VectorClock** — определяет, где файл изменился после прошлой синхронизации
3. **SyncEngine** — копирует изменения, распространяет удаления, разрешает конфликты (KeepNewest / KeepSource / KeepBoth)

---

## Архитектура

```
 ваш компьютер (Docker)                              другие устройства
┌───────────────────────────────────────────┐
│ auth-service :50056  — вход, JWT          │
│ oauth-service :8081                       │        ┌──────────────────────┐
│   Scheduler (раз в sync.interval)         │  HTTP  │ agent :9100          │
│     └─► PairManager ─► SyncEngine         │◄──────►│  базовая папка, напр.│
│           связка = Node A ↔ Node B        │ токен  │  /home/user          │
│           ├── SimpleStorage (local)       │        └──────────────────────┘
│           ├── HTTPRemoteNode (агент)      │
│           └── ComplexStorage (Drive) ─────┼──────► Google Drive API
│ postgres — устройства, связки, состояние  │
└───────────────────────────────────────────┘
```

Сервер сам ходит к агентам, поэтому порт агента должен быть доступен с компьютера сервера:
в локальной сети — напрямую, вне её — через Tailscale (см. ниже). Хостинг не нужен.

---

## Быстрый старт

### 1. Конфигурация

```bash
cp internal/config/config.example.yml internal/config/config.yml
cp .env.example .env
```

В `internal/config/config.yml`:
- `jwt.accessSecret` / `jwt.refreshSecret` — любые случайные строки
- `sync.agentToken` — общий секрет для агентов: `openssl rand -hex 32`
- `sync.interval` — период фоновой синхронизации (`1h`, `30m`, `off`)
- для Google Drive: `oauth.google_drive.oauth.client_id` / `client_secret` из Google Cloud Console,
  scope `https://www.googleapis.com/auth/drive`

В `.env`:
- `SYNC_LOCAL_ROOT` — папка этого компьютера, внутри которой выбираются папки для связок,
  например `/home/bloom/test-sync`. Монтируется в контейнер по тому же пути.
- `UID` / `GID` — ваш пользователь (`id -u`, `id -g`), чтобы файлы создавались не от root

### 2. Запуск

```bash
docker compose up -d --build
docker compose logs -f oauth-service   # логи синхронизации
```

Поднимаются `postgres`, `redis`, `migrate` (применяет миграции и завершается), `auth-service`
и `oauth-service`. Сервисы перезапускаются сами (`restart: unless-stopped`), в том числе после
перезагрузки. Порты PostgreSQL/Redis наружу не публикуются — не конфликтуют с локально установленными.
Консоль БД: `docker compose exec postgres psql -U postgres -d syncvault`.

`docker compose --profile extra up -d` дополнительно поднимает `api-gateway`, `sync-service` и
`mongodb` — для синхронизации они не нужны.

Если том `postgres_data` остался от старой версии и `migrate` падает на существующих таблицах,
пересоздайте его: `docker compose down -v` (удаляет данные БД в контейнере).

### 3. Аккаунт

```bash
# Регистрация (один раз). Пароль: от 8 символов, заглавная и строчная буква, цифра и спецсимвол
curl -s -X POST http://localhost:50056/register -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"<пароль>"}'

# Вход — токен действует 15 минут
TOKEN=$(curl -s -X POST http://localhost:50056/login -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"<пароль>"}' | jq -r '.access_token')
H=(-H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json")
```

### 4. Устройства и связки

```bash
# Устройства: local (этот компьютер) и подключённые агенты
curl -s "${H[@]}" localhost:8081/sync/devices | jq

# Выбрать папку на устройстве — просмотр подпапок
curl -s "${H[@]}" "localhost:8081/sync/devices/home-server/dirs?path=projects" | jq

# Связка: папка этого компьютера ↔ папка на устройстве
curl -s "${H[@]}" -X POST localhost:8081/sync/pairs -d '{
  "name": "docs",
  "a": {"device": "local",       "path": "/home/bloom/test-sync/docs"},
  "b": {"device": "home-server", "path": "projects/docs"}
}' | jq

# Связки и их статус (время, сколько передано/удалено, последняя ошибка)
curl -s "${H[@]}" localhost:8081/sync/pairs | jq
```

Дальше связки синхронизируются в фоне. Синхронизировать сейчас: `POST /sync/pairs/{id}/run`
(одну) или `POST /sync/pairs/run` (все).

---

## Подключение других устройств

На каждое Linux-устройство или сервер ставится агент — один на устройство. Он открывает серверу
**базовую папку** (по умолчанию домашнюю), а конкретные папки для связок выбираются с сервера.
Доступ к агенту — только с `sync.agentToken`; выйти за пределы базовой папки нельзя.
Агент сам регистрируется на сервере, шлёт heartbeat и переподключается после перезапусков.

**Вариант A — Docker** (на устройстве нужна копия репозитория):
```bash
cp deploy/agent/.env.example deploy/agent/.env   # AGENT_ID, AGENT_DATA_DIR, AGENT_SERVER, AGENT_TOKEN
docker compose -f deploy/agent/docker-compose.yml up -d --build
```

**Вариант B — бинарник + systemd** (без Go и Docker на устройстве):
```bash
make agent                                     # на вашем компьютере: bin/syncvault-agent-{amd64,arm64}
scp bin/syncvault-agent-amd64 server:/tmp/syncvault-agent
# на устройстве — см. deploy/agent/syncvault-agent.service
```

Или вручную:
```bash
./syncvault-agent --id home-server --server 192.168.1.10:8081 --token <sync.agentToken>
# --root /srv  — другая базовая папка (по умолчанию домашняя)
```

`--server` — адрес компьютера с сервером в локальной сети (`ip -br addr`, интерфейс Wi-Fi/Ethernet).
На устройстве должен быть открыт порт агента (9100), на компьютере с сервером — 8081.

### Устройства вне домашней сети

Устройства за другим роутером напрямую недоступны. Поставьте [Tailscale](https://tailscale.com)
(или ZeroTier) на компьютер с сервером и на эти устройства — они получат адреса `100.x.y.z`
и будут видны друг другу как в одной сети. Агенту указываются адреса из Tailscale:
```bash
./syncvault-agent --id vps --token <...> --server 100.64.0.1:8081 --advertise http://100.64.0.2:9100
```

### Если устройство выключено

Связки с недоступным устройством получают `last_status: "error"`, остальные синхронизируются.
Когда устройство снова в сети, следующий проход догонит изменения.

---

## Связки

### Стороны

| Сторона | Пример | Путь |
|---|---|---|
| Этот компьютер | `{"device": "local", "path": "/home/bloom/test-sync/docs"}` | абсолютный внутри `SYNC_LOCAL_ROOT` или относительно него |
| Агент | `{"device": "home-server", "path": "projects/docs"}` | относительно базовой папки агента или абсолютный внутри неё |
| Google Drive | `{"drive": "user@gmail.com", "folder_id": "<ID папки>"}` | `folder_id` — из URL `drive.google.com/drive/folders/<ID>`; без него — корень Drive |

При создании связки проверяется, что папки существуют, лежат внутри разрешённых корней и не
совпадают/не вложены друг в друга. Папки на устройствах должны существовать заранее.
Каждая связка синхронизируется независимо: файлы разных связок не смешиваются.
Одну папку можно связать с несколькими устройствами — это разные связки.

### Google Drive

```bash
# 1. Ваш user_id
curl -s localhost:50056/profile -H "Authorization: Bearer $TOKEN" | jq -r .id
# 2. В браузере: http://localhost:8081/auth/google?user_id=<ваш user_id>
# 3. Связка
curl -s "${H[@]}" -X POST localhost:8081/sync/pairs -d '{
  "name": "backup",
  "a": {"device": "local", "path": "/home/bloom/test-sync/docs"},
  "b": {"drive": "user@gmail.com", "folder_id": "<ID папки>"}
}'
```

- Токен Drive берётся у текущего пользователя — чужой Drive подключить нельзя.
- Google Docs/Sheets/Slides не синхронизируются (их нельзя скачать как файл).
- Удалённые файлы попадают в корзину Drive (восстанавливаются 30 дней).
- Со scope `drive.file` изменять и удалять можно только файлы, созданные SyncVault —
  для полной синхронизации нужен scope `https://www.googleapis.com/auth/drive`.

### Изменить связку

Связку нельзя изменить — удалите (`DELETE /sync/pairs/{id}`, файлы в папках остаются) и создайте
заново. Состояние синхронизации при этом начинается с нуля.

---

## Как синхронизируются изменения

- Файл изменился только на одной стороне — копируется на другую.
- Файл изменился на обеих сторонах — конфликт, решается стратегией (задаётся в `cmd/oauth-service/main.go`):

| Стратегия | Поведение |
|-----------|-----------|
| `KeepNewest` | Побеждает файл с более поздним временем изменения (по умолчанию) |
| `KeepSource` | Всегда побеждает сторона `a` |
| `KeepBoth` | На обеих сторонах — версия `b`, версия `a` сохраняется как `filename.conflict.<id стороны>` |

- Файл удалён на одной стороне, а на другой не менялся — удаляется и там. Если его изменили после
  удаления на другой стороне — изменённая версия восстанавливается.
- Защита от массового удаления: если сторона вдруг оказалась пустой, хотя раньше на ней было больше
  10 синхронизированных файлов (например, не примонтирован диск), синхронизация связки отменяется.
  Если файлы удалены намеренно — пересоздайте связку.

Состояние синхронизации хранится в PostgreSQL (`sync_file_states`) и переживает перезапуски.

---

## Справочник API

`oauth-service` (`http://localhost:8081`), все `/sync/*` и `/drive/*` — с JWT:

| Метод | Путь | Описание |
|-------|------|----------|
| `GET` | `/sync/devices` | Устройства: `local` и агенты (online, last_seen) |
| `GET` | `/sync/devices/{id}/dirs?path=` | Подпапки папки на устройстве |
| `DELETE` | `/sync/devices/{id}` | Удалить агента (если он не используется в связках) |
| `GET` | `/sync/pairs` | Связки и статус последней синхронизации |
| `POST` | `/sync/pairs` | Создать связку |
| `DELETE` | `/sync/pairs/{id}` | Удалить связку |
| `POST` | `/sync/pairs/{id}/run` | Синхронизировать связку сейчас |
| `POST` | `/sync/pairs/run` | Синхронизировать все связки сейчас |
| `POST` | `/sync/agents/register` | Агент регистрирует себя (`X-Agent-Token`) |
| `POST` | `/sync/agents/heartbeat` | Heartbeat агента (`X-Agent-Token`) |
| `GET` | `/auth/google?user_id=` | Подключить Google аккаунт |
| `GET` | `/drive/accounts` | Подключённые Google аккаунты |
| `GET` | `/drive/files` | Файлы на Drive |
| `POST` | `/drive/sync/download` | Скачать папку с Drive |
| `POST` | `/drive/sync/upload` | Загрузить папку на Drive |
| `POST` | `/drive/sync/drive-to-drive` | Скопировать папку между двумя Drive аккаунтами |
| `GET` | `/health` | Health check |

`auth-service` (`http://localhost:50056`): `POST /register`, `POST /login`, `POST /refresh`, `GET /profile`.

---

## Разработка без Docker

```bash
DATABASE_URL="postgres://postgres:<пароль>@localhost:5432/syncvault?sslmode=disable" \
  go run ./tools/migrate up
SYNC_LOCAL_ROOT=/home/bloom/test-sync go run ./cmd/oauth-service
go run ./cmd/auth-service
go test ./internal/sync/...
```

`go run ./tools/migrate version` — версия БД. Внимание: `down` без аргументов откатывает **все** миграции.

---

## Структура проекта

```
cmd/
  oauth-service/   — сервер синхронизации (порт 8081)
  auth-service/    — вход и регистрация (порт 50056)
  agent/           — агент для других устройств (порт 9100)
tools/migrate/     — миграции БД
deploy/agent/      — запуск агента: docker-compose и systemd-юнит

internal/
  sync/
    pairs.go       — связки: проверка сторон, запуск синхронизации, статус
    scheduler.go   — фоновая синхронизация связок раз в sync.interval
    engine.go      — SyncEngine: MerkleTree + VectorClock, удаления, конфликты
    node.go        — Node интерфейс, SimpleStorage, ComplexStorage
    remote_node.go — HTTPRemoteNode: папка на устройстве через HTTP API агента
    handlers.go    — HTTP API устройств, связок и агентов
    registry.go    — реестр подключённых агентов
    pgstore.go     — PostgreSQL: состояние файлов, устройства, связки
    adapters/googledrive.go — ComplexStorageAPI для Google Drive
    agent/http_agent.go     — агент: HTTP API базовой папки устройства

  auth/            — JWT, пользователи (user_store.go)
  oauth/google/    — OAuth flow, DriveAdapter, прямые операции с Drive
  config/          — конфигурация (config.example.yml — шаблон)
```
