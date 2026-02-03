package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// ExampleUserService пример реализации UserService
type ExampleUserService struct {
	users map[string]*UserInfo
}

func NewExampleUserService() *ExampleUserService {
	// В реальном приложении здесь будет подключение к базе данных
	users := make(map[string]*UserInfo)

	// Добавляем тестового пользователя
	users["user@example.com"] = &UserInfo{
		ID:           "user123",
		Email:        "user@example.com",
		Role:         "user",
		PasswordHash: "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi", // "password"
	}

	users["admin@example.com"] = &UserInfo{
		ID:           "admin123",
		Email:        "admin@example.com",
		Role:         "admin",
		PasswordHash: "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi", // "password"
	}

	return &ExampleUserService{users: users}
}

func (s *ExampleUserService) GetUserByEmail(ctx context.Context, email string) (*UserInfo, error) {
	user, exists := s.users[email]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

func (s *ExampleUserService) VerifyPassword(hashedPassword, password string) bool {
	return CheckPassword(hashedPassword, password)
}

// SetupAuthRoutes пример настройки роутов аутентификации
func SetupAuthRoutes() http.Handler {
	// Конфигурация
	jwtConfig := DefaultJWTConfig()
	jwtConfig.AccessSecret = "your-access-secret-key-change-in-production"
	jwtConfig.RefreshSecret = "your-refresh-secret-key-change-in-production"

	redisConfig := DefaultRedisConfig()

	// Инициализация Redis клиента
	redisClient, err := NewRedisClient(redisConfig)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Инициализация сервисов
	jwtService := NewJWTService(jwtConfig, redisClient)
	userService := NewExampleUserService()
	authHandler := NewAuthHandler(jwtService, userService)

	// Создаем роутер
	router := chi.NewRouter()

	// Роуты аутентификации
	authHandler.RegisterRoutes(router)

	// Пример защищенных роутов
	router.Route("/api/v1", func(r chi.Router) {
		// Общедоступные роуты
		r.Get("/public", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Public endpoint"))
		})

		// Требуется аутентификация
		r.Group(func(r chi.Router) {
			r.Use(jwtService.AuthMiddleware)
			r.Get("/protected", func(w http.ResponseWriter, r *http.Request) {
				userID, _ := GetUserIDFromContext(r.Context())
				email, _ := GetEmailFromContext(r.Context())
				role, _ := GetRoleFromContext(r.Context())

				response := map[string]string{
					"message": "Protected endpoint",
					"user_id": userID,
					"email":   email,
					"role":    role,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			})
		})

		// Требуется роль admin
		r.Group(func(r chi.Router) {
			r.Use(jwtService.AuthMiddleware)
			r.Use(RequireAdminMiddleware)
			r.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("Admin only endpoint"))
			})
		})

		// Опциональная аутентификация
		r.Group(func(r chi.Router) {
			r.Use(jwtService.OptionalAuthMiddleware)
			r.Get("/optional", func(w http.ResponseWriter, r *http.Request) {
				userID, ok := GetUserIDFromContext(r.Context())
				if ok {
					response := map[string]string{
						"message": "Optional auth endpoint - authenticated",
						"user_id": userID,
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(response)
				} else {
					w.Write([]byte("Optional auth endpoint - anonymous"))
				}
			})
		})
	})

	return router
}

// ExampleUsage пример использования JWT системы
func ExampleUsage() {
	// Запуск сервера
	router := SetupAuthRoutes()

	log.Println("Server starting on :8080")
	log.Println("Available endpoints:")
	log.Println("  POST /api/v1/auth/login - Login")
	log.Println("  POST /api/v1/auth/refresh - Refresh token")
	log.Println("  POST /api/v1/auth/logout - Logout")
	log.Println("  GET  /api/v1/auth/me - Get user info")
	log.Println("  GET  /api/v1/public - Public endpoint")
	log.Println("  GET  /api/v1/protected - Protected endpoint (requires auth)")
	log.Println("  GET  /api/v1/admin - Admin endpoint (requires admin role)")
	log.Println("  GET  /api/v1/optional - Optional auth endpoint")

	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// TestAuthFlow пример тестового потока аутентификации
func TestAuthFlow() {
	ctx := context.Background()

	// Конфигурация
	jwtConfig := DefaultJWTConfig()
	jwtConfig.AccessSecret = "test-access-secret"
	jwtConfig.RefreshSecret = "test-refresh-secret"

	redisConfig := DefaultRedisConfig()
	redisClient, _ := NewRedisClient(redisConfig)

	// Сервисы
	jwtService := NewJWTService(jwtConfig, redisClient)

	// 1. Генерация токенов
	tokenPair, err := jwtService.GenerateTokenPair(ctx, "user123", "user@example.com", "user")
	if err != nil {
		log.Printf("Failed to generate tokens: %v", err)
		return
	}

	log.Printf("Access token: %s", tokenPair.AccessToken)
	log.Printf("Refresh token: %s", tokenPair.RefreshToken)

	// 2. Валидация access токена
	claims, err := jwtService.ValidateToken(tokenPair.AccessToken, AccessTokenType)
	if err != nil {
		log.Printf("Failed to validate access token: %v", err)
		return
	}

	log.Printf("Access token valid - User ID: %s, Email: %s, Role: %s", claims.UserID, claims.Email, claims.Role)

	// 3. Ротация refresh токена
	newTokenPair, err := jwtService.RefreshTokens(ctx, tokenPair.RefreshToken)
	if err != nil {
		log.Printf("Failed to refresh tokens: %v", err)
		return
	}

	log.Printf("New access token: %s", newTokenPair.AccessToken)
	log.Printf("New refresh token: %s", newTokenPair.RefreshToken)

	// 4. Logout
	err = jwtService.Logout(ctx, "user123")
	if err != nil {
		log.Printf("Failed to logout: %v", err)
		return
	}

	log.Println("Logout successful - all tokens revoked")
}
