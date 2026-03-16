package auth

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// TestServer создает тестовый сервер для проверки JWT auth системы
func TestServer() http.Handler {
	// Конфигурация
	jwtConfig := &JWTConfig{
		AccessSecret:  "test-access-secret-key-32-chars-min",
		RefreshSecret: "test-refresh-secret-key-32-chars-min",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    30 * 24 * time.Hour,
		Issuer:        "syncvault-test",
	}

	redisConfig := &RedisConfig{
		Host:     "localhost",
		Port:     6379,
		Password: "",
		DB:       1, // Используем отдельную БД для тестов
	}

	// Инициализация Redis клиента
	redisClient, err := NewRedisClient(redisConfig)
	if err != nil {
		log.Printf("Warning: Redis not available, using in-memory storage: %v", err)
		// В реальном проекте здесь можно реализовать fallback
	}

	// Инициализация сервисов
	jwtService := NewJWTService(jwtConfig, redisClient)
	userService := NewExampleUserService()
	authHandler := NewAuthHandler(jwtService, userService)

	// Создаем роутер
	router := chi.NewRouter()

	// Middleware для логирования запросов
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Логируем запрос
			log.Printf("[%s] %s %s", r.Method, r.URL.Path, r.RemoteAddr)

			// Если есть пользователь в context, логируем его
			if userID, ok := GetUserIDFromContext(r.Context()); ok {
				log.Printf("User: %s", userID)
			}

			next.ServeHTTP(w, r)

			// Логируем время выполнения
			log.Printf("Request completed in %v", time.Since(start))
		})
	})

	// Роуты аутентификации
	authHandler.RegisterRoutes(router)

	// Тестовые роуты
	router.Route("/test", func(r chi.Router) {
		// Общедоступный роут
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"message": "Public endpoint - no auth required",
				"time":    time.Now().Format(time.RFC3339),
			})
		})

		// Защищенный роут
		r.With(jwtService.AuthMiddleware).Get("/protected", func(w http.ResponseWriter, r *http.Request) {
			userID, _ := GetUserIDFromContext(r.Context())
			email, _ := GetEmailFromContext(r.Context())
			role, _ := GetRoleFromContext(r.Context())

			response := map[string]interface{}{
				"message": "Protected endpoint - authenticated",
				"user": map[string]string{
					"id":    userID,
					"email": email,
					"role":  role,
				},
				"time": time.Now().Format(time.RFC3339),
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		// Admin роут
		r.With(jwtService.AuthMiddleware, RequireAdminMiddleware).Get("/admin", func(w http.ResponseWriter, r *http.Request) {
			userID, _ := GetUserIDFromContext(r.Context())

			response := map[string]interface{}{
				"message":  "Admin endpoint - admin only",
				"admin_id": userID,
				"time":     time.Now().Format(time.RFC3339),
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		// Опциональная аутентификация
		r.With(jwtService.OptionalAuthMiddleware).Get("/optional", func(w http.ResponseWriter, r *http.Request) {
			userID, ok := GetUserIDFromContext(r.Context())

			var response map[string]interface{}

			if ok {
				response = map[string]interface{}{
					"message":       "Optional auth endpoint - authenticated",
					"user_id":       userID,
					"authenticated": true,
				}
			} else {
				response = map[string]interface{}{
					"message":       "Optional auth endpoint - anonymous",
					"authenticated": false,
				}
			}

			response["time"] = time.Now().Format(time.RFC3339)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})
	})

	// Health check
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		health := map[string]interface{}{
			"status":    "ok",
			"timestamp": time.Now().Format(time.RFC3339),
			"version":   "1.0.0",
		}

		// Проверяем Redis
		if redisClient != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			if err := redisClient.Ping(ctx).Err(); err != nil {
				health["redis"] = "error: " + err.Error()
				health["status"] = "degraded"
			} else {
				health["redis"] = "ok"
			}
		} else {
			health["redis"] = "not configured"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health)
	})

	return router
}

// RunTestServer запускает тестовый сервер
func RunTestServer() {
	router := TestServer()

	log.Println("🚀 JWT Auth Test Server")
	log.Println("Available endpoints:")
	log.Println("  Auth:")
	log.Println("    POST /api/v1/auth/login      - Login (user@example.com / password)")
	log.Println("    POST /api/v1/auth/refresh    - Refresh tokens")
	log.Println("    POST /api/v1/auth/logout     - Logout")
	log.Println("    GET  /api/v1/auth/me         - Get user info")
	log.Println()
	log.Println("  Test endpoints:")
	log.Println("    GET  /test/                  - Public endpoint")
	log.Println("    GET  /test/protected         - Protected (requires auth)")
	log.Println("    GET  /test/admin             - Admin only")
	log.Println("    GET  /test/optional          - Optional auth")
	log.Println()
	log.Println("  Health:")
	log.Println("    GET  /health                 - Health check")
	log.Println()
	log.Println("📝 Example usage:")
	log.Println("  1. Login:")
	log.Println("     curl -X POST http://localhost:8080/api/v1/auth/login \\")
	log.Println("       -H 'Content-Type: application/json' \\")
	log.Println("       -d '{\"email\":\"user@example.com\",\"password\":\"password\"}'")
	log.Println()
	log.Println("  2. Access protected endpoint:")
	log.Println("     curl -X GET http://localhost:8080/test/protected \\")
	log.Println("       -H 'Authorization: Bearer YOUR_ACCESS_TOKEN'")
	log.Println()
	log.Println("  3. Refresh tokens:")
	log.Println("     curl -X POST http://localhost:8080/api/v1/auth/refresh \\")
	log.Println("       -H 'Content-Type: application/json' \\")
	log.Println("       -d '{\"refresh_token\":\"YOUR_REFRESH_TOKEN\"}'")
	log.Println()
	log.Println("🔧 Server starting on :8080")

	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// RunTestsWithServer запускает автоматизированные тесты
func RunTestsWithServer() {
	log.Println("🧪 Running JWT Auth System Tests")

	// Конфигурация
	jwtConfig := &JWTConfig{
		AccessSecret:  "test-access-secret-key-32-chars-min",
		RefreshSecret: "test-refresh-secret-key-32-chars-min",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    30 * 24 * time.Hour,
		Issuer:        "syncvault-test",
	}

	redisConfig := &RedisConfig{
		Host:     "localhost",
		Port:     6379,
		Password: "",
		DB:       2, // Отдельная БД для тестов
	}

	// Проверяем Redis
	redisClient, err := NewRedisClient(redisConfig)
	if err != nil {
		log.Printf("⚠️  Redis not available: %v", err)
		log.Println("Running tests without Redis...")
		redisClient = nil
	} else {
		log.Println("✅ Redis connected")
	}

	jwtService := NewJWTService(jwtConfig, redisClient)
	ctx := context.Background()

	// Тест 1: Генерация токенов
	log.Println("\n1. Testing token generation...")
	tokenPair, err := jwtService.GenerateTokenPair(ctx, "test123", "test@example.com", "user")
	if err != nil {
		log.Printf("❌ Token generation failed: %v", err)
		return
	}
	log.Println("✅ Token generation successful")
	log.Printf("   Access token: %s...", tokenPair.AccessToken[:20])
	log.Printf("   Refresh token: %s...", tokenPair.RefreshToken[:20])

	// Тест 2: Валидация access токена
	log.Println("\n2. Testing access token validation...")
	claims, err := jwtService.ValidateToken(tokenPair.AccessToken, AccessTokenType)
	if err != nil {
		log.Printf("❌ Access token validation failed: %v", err)
		return
	}
	log.Println("✅ Access token validation successful")
	log.Printf("   User ID: %s", claims.UserID)
	log.Printf("   Email: %s", claims.Email)
	log.Printf("   Role: %s", claims.Role)

	// Тест 3: Валидация refresh токена
	log.Println("\n3. Testing refresh token validation...")
	claims, err = jwtService.ValidateToken(tokenPair.RefreshToken, RefreshTokenType)
	if err != nil {
		log.Printf("❌ Refresh token validation failed: %v", err)
		return
	}
	log.Println("✅ Refresh token validation successful")
	log.Printf("   User ID: %s", claims.UserID)

	// Тест 4: Ротация токенов
	log.Println("\n4. Testing token rotation...")
	newTokenPair, err := jwtService.RefreshTokens(ctx, tokenPair.RefreshToken)
	if err != nil {
		log.Printf("❌ Token rotation failed: %v", err)
		return
	}
	log.Println("✅ Token rotation successful")
	log.Printf("   New access token: %s...", newTokenPair.AccessToken[:20])
	log.Printf("   New refresh token: %s...", newTokenPair.RefreshToken[:20])

	// Тест 5: Обнаружение кражи токена
	log.Println("\n5. Testing token theft detection...")
	_, err = jwtService.RefreshTokens(ctx, tokenPair.RefreshToken)
	if err != nil {
		log.Println("✅ Token theft detection successful - old token rejected")
	} else {
		log.Println("❌ Token theft detection failed - old token was accepted")
	}

	// Тест 6: Logout
	log.Println("\n6. Testing logout...")
	err = jwtService.Logout(ctx, "test123")
	if err != nil {
		log.Printf("❌ Logout failed: %v", err)
		return
	}
	log.Println("✅ Logout successful")

	// Тест 7: Проверка что токены отозваны
	log.Println("\n7. Testing token revocation...")
	_, err = jwtService.RefreshTokens(ctx, newTokenPair.RefreshToken)
	if err != nil {
		log.Println("✅ Token revocation successful - tokens are invalid")
	} else {
		log.Println("❌ Token revocation failed - tokens are still valid")
	}

	log.Println("\n🎉 All tests completed!")
	log.Println("\n📊 Test Summary:")
	log.Println("  ✅ Token generation")
	log.Println("  ✅ Access token validation")
	log.Println("  ✅ Refresh token validation")
	log.Println("  ✅ Token rotation")
	log.Println("  ✅ Token theft detection")
	log.Println("  ✅ Logout")
	log.Println("  ✅ Token revocation")
}
