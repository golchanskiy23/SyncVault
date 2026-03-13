# SyncVault

Система синхронизации файлов между разнородными хранилищами через центральный сервер (middleware).

Поддерживаемые хранилища:
- **SimpleStorage** — любая машина с ОС (Linux, macOS, Windows) с доступом к файловой системе
- **ComplexStorage** — специализированные API-хранилища (Google Drive, S3, Dropbox и т.д.)
- **RemoteNode** — удалённая машина в сети, на которой запущен агент SyncVault

Синхронизация работает через алгоритм:
1. **MerkleTree** — быстро находит расходящиеся файлы между двумя узлами
2. **VectorClock** — определяет кто новее при конфликте
3. **SyncEngine** — применяет стратегию разрешения конфликтов (KeepNewest / KeepSource / KeepBoth)

---

## Архитектура

```
┌─────────────────────────────────────────────────────────┐
│                   SyncVault Server                       │
│                  (oauth-service :8081)                   │
│                                                          │
│  NodeRegistry ──► SyncEngine ──► MerkleTree + VectorClock│
│       │                                                  │
│  ┌────┴──────────────────────────────────┐               │
│  │  Node (интерфейс)                     │               │
│  │  ├── SimpleStorage  (локальная ОС)    │               │
│  │  ├── ComplexStorage (Google Drive...) │               │
│  │  └── HTTPAgentNode  (удалённая машина)│               │
│  └───────────────────────────────────────┘               │
└─────────────────────────────────────────────────────────┘
```

Все хранилища регистрируются в `NodeRegistry`. `SyncEngine` работает только с интерфейсом `Node` — он не знает что за ним стоит.

---

## Быстрый старт

### 1. Зависимости

```bash
# PostgreSQL + Redis
docker-compose up -d postgres redis
```

### 2. Конфигурация

```bash
cp internal/config/config.example.yml internal/config/config.yml
```

Заполнить в `config.yml`:
- `database.password` — пароль PostgreSQL
- `jwt.accessSecret` / `jwt.refreshSecret` — любые случайные строки
- `oauth.google_drive.oauth.client_id` / `client_secret` — из Google Cloud Console

### 3. Миграции

```bash
go run cmd/migrate/main.go
```

### 4. Запуск сервера

```bash
go run cmd/oauth-service/main.go
```

Сервер стартует на `http://localhost:8081`.

### 5. Запуск агента (для удалённых машин)

На каждой удалённой машине в сети:

```bash
go run cmd/agent/main.go --port 9100 --root /path/to/sync/folder
```

---

## Получение JWT токена

Все `/sync/*` и `/drive/*` эндпоинты требуют JWT. Для тестирования используются хардкод-учётные данные:

```bash
TOKEN=$(curl -s -X POST http://localhost:8081/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}' \
  | jq -r '.access_token')
```

---

## Способы синхронизации

### Случай 1: Локальная папка ↔ Локальная папка

Синхронизация двух папок на одной машине (или двух машин с общим NFS).

```bash
# Регистрируем первую папку
curl -X POST http://localhost:8081/sync/nodes/local \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "folder-a", "root_path": "/home/user/docs"}'

# Регистрируем вторую папку
curl -X POST http://localhost:8081/sync/nodes/local \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "folder-b", "root_path": "/home/user/backup"}'

# Синхронизируем
curl -X POST http://localhost:8081/sync/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"source_id": "folder-a", "target_id": "folder-b"}'
```

---

### Случай 2: Локальная папка ↔ Google Drive

Файлы с локальной машины синхронизируются с Google Drive аккаунтом.

```bash
# Шаг 1: подключить Google аккаунт через OAuth
# Открыть в браузере:
# http://localhost:8081/auth/google?user_id=user_123

# Шаг 2: зарегистрировать локальную папку
curl -X POST http://localhost:8081/sync/nodes/local \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "laptop-home", "root_path": "/home/user/documents"}'

# Шаг 3: зарегистрировать Google Drive как узел
curl -X POST http://localhost:8081/sync/nodes/drive \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "drive-main", "account_id": "user@gmail.com", "user_id": "user_123"}'

# Шаг 4: синхронизировать
curl -X POST http://localhost:8081/sync/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"source_id": "laptop-home", "target_id": "drive-main"}'
```

Поток данных:
```
/home/user/documents  ──►  SyncVault Server  ──►  Google Drive
     (SimpleStorage)           (tmpDir)          (ComplexStorage)
```

---

### Случай 3: Google Drive ↔ Google Drive (два аккаунта)

Синхронизация между двумя Google аккаунтами. Данные проходят через сервер как middleware.

```bash
# Подключить оба аккаунта через OAuth (в браузере):
# http://localhost:8081/auth/google?user_id=user_123
# (войти под первым аккаунтом, затем повторить под вторым)

# Зарегистрировать первый аккаунт
curl -X POST http://localhost:8081/sync/nodes/drive \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "drive-account1", "account_id": "first@gmail.com", "user_id": "user_123"}'

# Зарегистрировать второй аккаунт
curl -X POST http://localhost:8081/sync/nodes/drive \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "drive-account2", "account_id": "second@gmail.com", "user_id": "user_123"}'

# Синхронизировать
curl -X POST http://localhost:8081/sync/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"source_id": "drive-account1", "target_id": "drive-account2"}'
```

Поток данных:
```
Google Drive (account1)  ──►  SyncVault Server  ──►  Google Drive (account2)
    (ComplexStorage)              (tmpDir)               (ComplexStorage)
```

---

### Случай 4: Удалённая машина ↔ Любое хранилище

Машина B в локальной сети регистрирует себя через агент. Сервер синхронизирует её с любым другим узлом.

**На машине B:**
```bash
go run cmd/agent/main.go --port 9100 --root /home/user/sync
```

**На сервере:**
```bash
# Зарегистрировать удалённую машину
curl -X POST http://localhost:8081/sync/nodes/remote \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "laptop-b", "endpoint": "http://192.168.1.10:9100"}'

# Зарегистрировать локальную папку на сервере
curl -X POST http://localhost:8081/sync/nodes/local \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "server-storage", "root_path": "/var/syncvault/data"}'

# Синхронизировать
curl -X POST http://localhost:8081/sync/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"source_id": "server-storage", "target_id": "laptop-b"}'
```

---

### Случай 5: Полная синхронизация всех узлов в сети

Все зарегистрированные узлы синхронизируются между собой (полный граф).

```bash
# Предварительно зарегистрировать все нужные узлы (случаи 1-4 выше)

# Запустить полную синхронизацию
curl -X POST http://localhost:8081/sync/run/all \
  -H "Authorization: Bearer $TOKEN"
```

Пример сети из 4 узлов:
```
laptop-home ──────────────── drive-main
     │    ╲                ╱    │
     │      ╲            ╱      │
     │        server-nas        │
     │      ╱            ╲      │
     │    ╱                ╲    │
laptop-b ──────────────── drive-backup
```

`SyncAll` синхронизирует все пары: `N*(N-1)/2` операций. При 4 узлах — 6 пар.

---

## Управление узлами

### Список зарегистрированных узлов

```bash
curl http://localhost:8081/sync/nodes \
  -H "Authorization: Bearer $TOKEN"
```

Ответ:
```json
{
  "count": 3,
  "nodes": [
    {"ID": "laptop-home", "Type": "simple",       "Endpoint": "/home/user/docs", "Online": true},
    {"ID": "drive-main",  "Type": "google_drive", "AccountID": "user@gmail.com", "Online": true},
    {"ID": "laptop-b",    "Type": "remote_simple", "Endpoint": "http://192.168.1.10:9100", "Online": true}
  ]
}
```

### Удалить узел

```bash
curl -X DELETE http://localhost:8081/sync/nodes/laptop-b \
  -H "Authorization: Bearer $TOKEN"
```

### Heartbeat (агент сообщает что жив)

```bash
curl -X POST http://localhost:8081/sync/nodes/heartbeat \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"node_id": "laptop-b"}'
```

---

## Google Drive: прямые операции

Помимо синхронизации через SyncEngine, доступны прямые операции с Drive.

### Список подключённых аккаунтов

```bash
curl http://localhost:8081/drive/accounts \
  -H "Authorization: Bearer $TOKEN"
```

### Скачать папку с Drive на локальный путь

```bash
curl -X POST http://localhost:8081/drive/sync/download \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"folder_id": "1akCM2ejRZ7zP8IjIYDLIE_OjMZkiqOQ3", "local_path": "/home/user/downloads"}'
```

### Загрузить локальную папку на Drive

```bash
curl -X POST http://localhost:8081/drive/sync/upload \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"local_path": "/home/user/documents", "folder_id": "1akCM2ejRZ7zP8IjIYDLIE_OjMZkiqOQ3"}'
```

### Синхронизировать Drive → Drive (прямой метод без MerkleTree)

```bash
curl -X POST http://localhost:8081/drive/sync/drive-to-drive \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "src_account":   "first@gmail.com",
    "src_folder_id": "FOLDER_ID_SOURCE",
    "dst_account":   "second@gmail.com",
    "dst_folder_id": "FOLDER_ID_DEST"
  }'
```

> Отличие от `/sync/run`: этот метод копирует всё подряд без сравнения. `/sync/run` использует MerkleTree и копирует только изменения.

---

## Разрешение конфликтов

При одновременном изменении файла на двух узлах VectorClock определяет конфликт. Стратегия задаётся при старте сервера в `cmd/oauth-service/main.go`:

| Стратегия | Поведение |
|-----------|-----------|
| `KeepNewest` | Побеждает файл с более поздним временем изменения (по умолчанию) |
| `KeepSource` | Всегда побеждает источник синхронизации |
| `KeepBoth` | Оба файла сохраняются: оригинал + `filename.conflict.nodeID` |

---

## Полный пример: синхронизация всей сети

Сценарий: ноутбук дома, ноутбук в офисе, Google Drive как резервная копия.

```bash
# 1. Запустить агент на ноутбуке в офисе (192.168.1.20)
#    (выполнить на той машине)
go run cmd/agent/main.go --port 9100 --root /home/user/work

# 2. На сервере — зарегистрировать все узлы
TOKEN="..."  # получить через /auth/login

# Локальная папка на сервере
curl -X POST http://localhost:8081/sync/nodes/local \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "server-home", "root_path": "/var/syncvault/home"}'

# Ноутбук в офисе (агент)
curl -X POST http://localhost:8081/sync/nodes/remote \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "office-laptop", "endpoint": "http://192.168.1.20:9100"}'

# Google Drive (предварительно авторизоваться через браузер)
curl -X POST http://localhost:8081/sync/nodes/drive \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": "drive-backup", "account_id": "user@gmail.com", "user_id": "user_123"}'

# 3. Проверить что все узлы онлайн
curl http://localhost:8081/sync/nodes \
  -H "Authorization: Bearer $TOKEN"

# 4. Запустить полную синхронизацию всех узлов
curl -X POST http://localhost:8081/sync/run/all \
  -H "Authorization: Bearer $TOKEN"
```

Результат: все три узла (`server-home`, `office-laptop`, `drive-backup`) будут содержать одинаковый набор файлов. Синхронизируются только изменившиеся файлы (MerkleTree diff).

---

## Справочник API

| Метод | Путь | Описание |
|-------|------|----------|
| `POST` | `/sync/nodes/local` | Зарегистрировать локальную папку |
| `POST` | `/sync/nodes/remote` | Зарегистрировать удалённую машину (агент) |
| `POST` | `/sync/nodes/drive` | Зарегистрировать Google Drive аккаунт |
| `POST` | `/sync/nodes/heartbeat` | Обновить статус узла |
| `GET` | `/sync/nodes` | Список всех узлов |
| `DELETE` | `/sync/nodes/{id}` | Удалить узел |
| `POST` | `/sync/run` | Синхронизировать два узла |
| `POST` | `/sync/run/all` | Синхронизировать все узлы |
| `GET` | `/auth/google` | Начать OAuth flow для Google Drive |
| `GET` | `/auth/google/callback` | OAuth callback |
| `GET` | `/drive/accounts` | Список подключённых Google аккаунтов |
| `GET` | `/drive/files` | Список файлов на Drive |
| `POST` | `/drive/sync/download` | Скачать папку с Drive |
| `POST` | `/drive/sync/upload` | Загрузить папку на Drive |
| `POST` | `/drive/sync/drive-to-drive` | Синхронизировать два Drive аккаунта напрямую |
| `POST` | `/drive/sync` | Синхронизировать метаданные Drive в БД |
| `GET` | `/drive/sync/status` | Статус последней синхронизации |
| `GET` | `/health` | Health check |

---

## Структура проекта

```
cmd/
  oauth-service/   — основной сервер (порт 8081)
  agent/           — агент для удалённых машин (порт 9100)
  auth-service/    — сервис аутентификации (порт 8080)

internal/
  sync/
    engine.go      — SyncEngine: MerkleTree + VectorClock
    node.go        — Node интерфейс, SimpleStorage, ComplexStorage
    handlers.go    — HTTP API для управления узлами
    registry.go    — NodeRegistry: реестр всех узлов
    merkletree.go  — построение дерева и поиск diff
    vectorclock.go — векторные часы для разрешения конфликтов
    pgstore.go     — PostgreSQL StateStore
    adapters/
      googledrive.go — ComplexStorageAPI для Google Drive
    agent/
      http_agent.go  — HTTPAgentNode: удалённый узел через HTTP
      agent.go       — агент-сервер для удалённых машин

  oauth/google/
    handlers.go    — OAuth flow + прямые Drive операции
    drive.go       — DriveAdapter (Google Drive API)
    service.go     — OAuthService (токены, PKCE)

  config/
    config.example.yml — шаблон конфигурации
```
