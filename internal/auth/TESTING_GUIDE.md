# JWT Auth System - Testing Guide

## 🚀 Быстрый старт

### 1. Запуск тестового сервера
```bash
go run cmd/auth-test/main.go server
```
Сервер запустится на `http://localhost:8080`

### 2. Автоматические тесты
```bash
go run cmd/auth-test/main.go
# или
go test ./internal/auth/...
```

## 📋 API Endpoints

### Аутентификация
- `POST /api/v1/auth/login` - Вход в систему
- `POST /api/v1/auth/refresh` - Обновление токенов
- `POST /api/v1/auth/logout` - Выход из системы
- `GET /api/v1/auth/me` - Информация о текущем пользователе

### Тестовые роуты
- `GET /test/` - Публичный endpoint (без аутентификации)
- `GET /test/protected` - Защищенный endpoint (требуется auth)
- `GET /test/admin` - Admin endpoint (требуется роль admin)
- `GET /test/optional` - Опциональная аутентификация

### Системные
- `GET /health` - Проверка состояния системы

## 👤 Тестовые пользователи

### User
- **Email:** `user@example.com`
- **Password:** `password`
- **Role:** `user`

### Admin
- **Email:** `admin@example.com`
- **Password:** `password`
- **Role:** `admin`

## 🧪 Примеры использования

### 1. Login (получение токенов)
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}'
```

**Ответ:**
```json
{
  "tokens": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "token_type": "Bearer",
    "expires_in": 900
  },
  "user": {
    "id": "user123",
    "email": "user@example.com",
    "role": "user"
  }
}
```

### 2. Доступ к защищенному ресурсу
```bash
curl -X GET http://localhost:8080/test/protected \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

**Ответ:**
```json
{
  "message": "Protected endpoint - authenticated",
  "time": "2026-03-16T13:16:26+03:00",
  "user": {
    "email": "user@example.com",
    "id": "user123",
    "role": "user"
  }
}
```

### 3. Обновление токенов (Refresh)
```bash
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"YOUR_REFRESH_TOKEN"}'
```

**Ответ:**
```json
{
  "tokens": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```

### 4. Публичный endpoint (без auth)
```bash
curl -X GET http://localhost:8080/test/
```

**Ответ:**
```json
{
  "message": "Public endpoint - no auth required",
  "time": "2026-03-16T13:20:00+03:00"
}
```

### 5. Admin endpoint (требуется роль admin)
```bash
# Сначала логинимся как admin
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"password"}'

# Затем используем admin токен
curl -X GET http://localhost:8080/test/admin \
  -H "Authorization: Bearer ADMIN_ACCESS_TOKEN"
```

**Ответ:**
```json
{
  "message": "Admin endpoint - admin only",
  "admin_id": "admin123",
  "time": "2026-03-16T13:20:00+03:00"
}
```

### 6. Опциональная аутентификация
```bash
# Без токена
curl -X GET http://localhost:8080/test/optional

# С токеном
curl -X GET http://localhost:8080/test/optional \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

### 7. Logout
```bash
curl -X POST http://localhost:8080/api/v1/auth/logout \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

### 8. Получение информации о пользователе
```bash
curl -X GET http://localhost:8080/api/v1/auth/me \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

**Ответ:**
```json
{
  "id": "user123",
  "email": "user@example.com",
  "role": "user"
}
```

## 🔒 Тестирование безопасности

### Token Rotation (ротация токенов)
```bash
# 1. Получаем токены
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}'

# 2. Обновляем токены (старый refresh токен становится недействительным)
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"OLD_REFRESH_TOKEN"}'

# 3. Пытаемся использовать старый refresh токен снова (должно выдать ошибку)
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"OLD_REFRESH_TOKEN"}'
```

### Token Theft Detection (обнаружение кражи)
При повторном использовании refresh токена после ротации система обнаруживает кражу и блокирует все токены пользователя.

## 🔧 Диагностика

### Health Check
```bash
curl -X GET http://localhost:8080/health
```

**Ответ:**
```json
{
  "status": "ok",
  "timestamp": "2026-03-16T13:20:00+03:00",
  "version": "1.0.0",
  "redis": "ok"
}
```

### Логирование сервера
Сервер логирует все запросы с информацией о пользователе:
```
[GET] /test/protected 127.0.0.1:12345
User: user123
Request completed in 1.2ms
```

## 🧪 Unit тесты

### Запуск всех тестов
```bash
go test ./internal/auth/...
```

### Запуск с race detection
```bash
go test -race ./internal/auth/...
```

### Запуск конкретного теста
```bash
go test ./internal/auth/... -run TestJWTService
go test ./internal/auth/... -run TestAuthMiddleware
go test ./internal/auth/... -run TestAuthHandlers
```

### Покрытие кода
```bash
go test -cover ./internal/auth/...
```

## 📝 Важные моменты

### 🔑 Токены
- **Access Token:** 15 минут жизни
- **Refresh Token:** 30 дней жизни
- **Алгоритм:** HS256 (HMAC SHA-256)

### 🛡️ Безопасность
- Ротация refresh токенов при каждом использовании
- Обнаружение краж токенов
- Хранение токенов в Redis с TTL
- Автоматическая очистка истекших токенов

### 📦 Структура проекта
```
internal/auth/
├── jwt_models.go      # Модели данных
├── jwt_service.go     # Основной JWT сервис
├── middleware.go      # Auth middleware
├── handlers.go        # HTTP обработчики
├── redis_config.go    # Работа с Redis
├── jwt_test.go        # Unit тесты
├── test_server.go     # Тестовый сервер
├── example_usage.go   # Примеры использования
└── README.md          # Документация
```

## 🚨 Частые ошибки

### 1. `token is malformed`
**Причина:** Использование плейсхолдера `YOUR_ACCESS_TOKEN` вместо реального токена
**Решение:** Скопируйте реальный токен из ответа login endpoint

### 2. `Insufficient permissions`
**Причина:** Попытка доступа к admin endpoint с user ролью
**Решение:** Используйте `admin@example.com` для получения admin токена

### 3. `Invalid token: token is expired`
**Причина:** Истек срок действия access токена
**Решение:** Используйте refresh endpoint для получения новых токенов

### 4. `Invalid refresh token`
**Причина:** Refresh токен отозван или использован повторно
**Решение:** Выполните login снова для получения новой пары токенов

## 🎯 Полный workflow тестирования

```bash
# 1. Запуск сервера
go run cmd/auth-test/main.go server

# 2. В отдельном терминале - полный тестовый сценарий
# Login
TOKENS=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}' | jq -r '.tokens.access_token')

# Доступ к защищенному ресурсу
curl -X GET http://localhost:8080/test/protected \
  -H "Authorization: Bearer $TOKENS"

# Health check
curl -X GET http://localhost:8080/health
```

## 📊 Мониторинг

Сервер автоматически логирует:
- Все HTTP запросы
- Успешные и неуспешные попытки аутентификации
- Время выполнения запросов
- Информацию о пользователе в защищенных роутах

Готово к тестированию! 🎉
