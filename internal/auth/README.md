# JWT Authentication System

Полная JWT система аутентификации с поддержкой refresh токенов и хранением в Redis.

## Структура

- `jwt_models.go` - Модели данных и константы
- `jwt_service.go` - Основной JWT сервис
- `middleware.go` - Middleware для аутентификации
- `handlers.go` - HTTP обработчики
- `redis_config.go` - Конфигурация и работа с Redis
- `example_usage.go` - Примеры использования

## Основные возможности

### ✅ JWT токены
- Access токены (15 минут)
- Refresh токены (30 дней)
- Поддержка алгоритма HS256
- Валидация всех стандартных claims

### ✅ Безопасность
- Ротация refresh токенов
- Обнаружение краж токенов
- Хранение хешей в Redis с TTL
- Защита от CSRF через httpOnly cookies

### ✅ Middleware
- Обязательная аутентификация
- Опциональная аутентификация
- Проверка ролей
- Извлечение данных из context

### ✅ API Endpoints
- `POST /api/v1/auth/login` - Вход
- `POST /api/v1/auth/refresh` - Обновление токенов
- `POST /api/v1/auth/logout` - Выход
- `GET /api/v1/auth/me` - Информация о пользователе

## Быстрый старт

### 1. Конфигурация

```go
jwtConfig := &auth.JWTConfig{
    AccessSecret:  "your-access-secret-key",
    RefreshSecret: "your-refresh-secret-key", 
    AccessTTL:     15 * time.Minute,
    RefreshTTL:    30 * 24 * time.Hour,
    Issuer:        "syncvault",
}

redisConfig := &auth.RedisConfig{
    Host:     "localhost",
    Port:     6379,
    Password: "",
    DB:       0,
}
```

### 2. Инициализация

```go
// Redis клиент
redisClient, err := auth.NewRedisClient(redisConfig)
if err != nil {
    log.Fatal(err)
}

// JWT сервис
jwtService := auth.NewJWTService(jwtConfig, redisClient)

// UserService (ваша реализация)
userService := &YourUserService{}

// Auth обработчики
authHandler := auth.NewAuthHandler(jwtService, userService)
```

### 3. Настройка роутов

```go
router := chi.NewRouter()

// Роуты аутентификации
authHandler.RegisterRoutes(router)

// Защищенные роуты
router.Route("/api/v1", func(r chi.Router) {
    r.Use(jwtService.AuthMiddleware)
    r.Get("/protected", protectedHandler)
})
```

## Использование middleware

### Обязательная аутентификация
```go
r.Use(jwtService.AuthMiddleware)
```

### Опциональная аутентификация
```go
r.Use(jwtService.OptionalAuthMiddleware)
```

### Проверка роли
```go
r.Use(auth.RequireRoleMiddleware("admin"))
// или
r.Use(auth.RequireAdminMiddleware)
```

### Получение данных из context
```go
func handler(w http.ResponseWriter, r *http.Request) {
    userID, _ := auth.GetUserIDFromContext(r.Context())
    email, _ := auth.GetEmailFromContext(r.Context())
    role, _ := auth.GetRoleFromContext(r.Context())
    claims, _ := auth.GetClaimsFromContext(r.Context())
}
```

## Поток аутентификации

### 1. Login
```bash
POST /api/v1/auth/login
{
    "email": "user@example.com",
    "password": "password123"
}
```

Ответ:
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

### 2. Использование Access Token
```bash
GET /api/v1/protected
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

### 3. Refresh Token
```bash
POST /api/v1/auth/refresh
{
    "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

### 4. Logout
```bash
POST /api/v1/auth/logout
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

## UserService Interface

Вам нужно реализовать интерфейс `UserService`:

```go
type UserService interface {
    GetUserByEmail(ctx context.Context, email string) (*UserInfo, error)
    VerifyPassword(hashedPassword, password string) bool
}
```

Пример реализации:
```go
type YourUserService struct{}

func (s *YourUserService) GetUserByEmail(ctx context.Context, email string) (*auth.UserInfo, error) {
    // Получение пользователя из базы данных
    user, err := yourDB.GetUserByEmail(email)
    if err != nil {
        return nil, err
    }
    
    return &auth.UserInfo{
        ID:           user.ID,
        Email:        user.Email,
        Role:         user.Role,
        PasswordHash: user.PasswordHash,
    }, nil
}

func (s *YourUserService) VerifyPassword(hashedPassword, password string) bool {
    return auth.CheckPassword(hashedPassword, password)
}
```

## Безопасность

### Refresh Token Rotation
При каждом использовании refresh токена:
1. Создается новая пара токенов
2. Старый refresh токен отзывается
3. Если старый токен используется повторно - кража обнаружена

### Хранение в Redis
- Refresh токены хранятся как хеши с TTL
- Автоматическая очистка истекших токенов
- Возможность отзыва всех токенов пользователя

### Рекомендации
1. Используйте httpOnly Secure cookies для хранения токенов
2. Настройте CORS для вашего домена
3. Используйте HTTPS в production
4. Регулярно ротируйте секретные ключи
5. Логируйте события безопасности

## Тестирование

Запустите пример использования:
```go
func main() {
    auth.ExampleUsage()
}
```

Тестовый поток:
```bash
# 1. Login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}'

# 2. Доступ к защищенному ресурсу
curl -X GET http://localhost:8080/api/v1/protected \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"

# 3. Обновление токена
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"YOUR_REFRESH_TOKEN"}'
```

## Конфигурация для Production

```go
jwtConfig := &auth.JWTConfig{
    AccessSecret:  os.Getenv("JWT_ACCESS_SECRET"),  // Минимум 32 символа
    RefreshSecret: os.Getenv("JWT_REFRESH_SECRET"), // Минимум 32 символа
    AccessTTL:     15 * time.Minute,
    RefreshTTL:    30 * 24 * time.Hour,
    Issuer:        "your-app-name",
}

redisConfig := &auth.RedisConfig{
    Host:     os.Getenv("REDIS_HOST"),
    Port:     6379,
    Password: os.Getenv("REDIS_PASSWORD"),
    DB:       0,
}
```

## Мониторинг

### Health Check
```go
// Проверка доступности Redis
err := redisRepo.HealthCheck(ctx)
```

### Метрики
- Количество активных токенов пользователя
- Статистика успешных/неудачных попыток входа
- Время жизни токенов

## Логирование

Добавьте логирование для событий безопасности:
- Успешные входы
- Неудачные попытки входа
- Обнаружение краж токенов
- Logout события

## Интеграция с существующим проектом

1. Добавьте пакет auth в ваш проект
2. Реализуйте UserService интерфейс
3. Настройте middleware в вашем роутере
4. Обновите фронтенд для работы с JWT токенами

Готово! 🎉
