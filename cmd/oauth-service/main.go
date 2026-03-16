package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"syncvault/internal/auth"
	"syncvault/internal/config"
	"syncvault/internal/oauth/google"
)

func main() {
	log.Println("🚀 Starting OAuth Service with Google Drive integration...")

	// Загружаем конфигурацию
	cfg := config.LoadConfig()

	// Создаем подключение к PostgreSQL
	db, err := pgxpool.New(context.Background(), cfg.Database.GetConnectionString())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Создаем Redis клиент
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Создаем JWT сервис
	jwtConfig := auth.DefaultJWTConfig()
	jwtConfig.AccessSecret = cfg.JWT.AccessSecret
	jwtConfig.RefreshSecret = cfg.JWT.RefreshSecret
	jwtConfig.AccessTTL = cfg.JWT.AccessTTL
	jwtConfig.RefreshTTL = cfg.JWT.RefreshTTL
	jwtConfig.Issuer = cfg.JWT.Issuer

	jwtService := auth.NewJWTService(jwtConfig, rdb)

	// Конвертируем конфигурацию для OAuth
	googleDriveConfig := &google.GoogleDriveConfig{
		OAuth: &google.GoogleOAuthConfig{
			ClientID:     cfg.OAuth.GoogleDrive.OAuth.ClientID,
			ClientSecret: cfg.OAuth.GoogleDrive.OAuth.ClientSecret,
			RedirectURL:  cfg.OAuth.GoogleDrive.OAuth.RedirectURL,
			Scopes:       cfg.OAuth.GoogleDrive.OAuth.Scopes,
		},
		APIBaseURL:  cfg.OAuth.GoogleDrive.APIBaseURL,
		UploadURL:   cfg.OAuth.GoogleDrive.UploadURL,
		MaxFileSize: cfg.OAuth.GoogleDrive.MaxFileSize,
		ChunkSize:   cfg.OAuth.GoogleDrive.ChunkSize,
		RetryCount:  cfg.OAuth.GoogleDrive.RetryCount,
		RetryDelay:  cfg.OAuth.GoogleDrive.RetryDelay,
	}

	// Валидируем конфигурацию
	if err := googleDriveConfig.Validate(); err != nil {
		log.Fatalf("Invalid Google Drive config: %v", err)
	}

	// Создаем OAuth handlers
	oauthHandlers := google.NewOAuthHandlers(db, googleDriveConfig, jwtService)

	// Создаем роутер
	router := chi.NewRouter()

	// Middleware
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			log.Printf("[%s] %s %s", r.Method, r.URL.Path, r.RemoteAddr)
			next.ServeHTTP(w, r)
		})
	})

	// Health check
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now().UTC(),
			"service":   "oauth-service",
			"version":   "1.0.0",
		}
		json.NewEncoder(w).Encode(response)
	})

	// Регистрируем OAuth роуты
	oauthHandlers.RegisterRoutes(router)

	// Дополнительные роуты для интеграции с основным auth сервисом
	router.Group(func(r chi.Router) {
		r.Use(jwtService.AuthMiddleware)

		// Получение OAuth статуса пользователя
		r.Get("/oauth/status", func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.GetUserIDFromContext(r.Context())
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Проверяем есть ли у пользователя Google OAuth токены
			token, err := oauthHandlers.OAuthService.GetToken(r.Context(), userID)
			if err != nil {
				response := map[string]interface{}{
					"connected": false,
					"provider":  "google",
					"user_id":   userID,
				}
				json.NewEncoder(w).Encode(response)
				return
			}

			response := map[string]interface{}{
				"connected":   true,
				"provider":    "google",
				"user_id":     userID,
				"token_expiry": token.Expiry,
				"scope":       token.Scope,
			}
			json.NewEncoder(w).Encode(response)
		})

		// Интеграция с существующими endpoint'ами
		r.Get("/profile/enhanced", func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.GetUserIDFromContext(r.Context())
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Базовая информация о пользователе
			profile := map[string]interface{}{
				"id":    userID,
				"email": "user@example.com", // из вашего UserService
				"role":  "user",
			}

			// Добавляем OAuth информацию если доступно
			if token, err := oauthHandlers.OAuthService.GetToken(r.Context(), userID); err == nil {
				profile["google_drive"] = map[string]interface{}{
					"connected":   true,
					"token_expiry": token.Expiry,
					"scope":       token.Scope,
				}
			} else {
				profile["google_drive"] = map[string]interface{}{
					"connected": false,
				}
			}

			json.NewEncoder(w).Encode(profile)
		})
	})

	// Запускаем HTTP сервер
	port := "8080"
	if envPort := os.Getenv("OAUTH_SERVICE_PORT"); envPort != "" {
		port = envPort
	}

	log.Printf("🌐 OAuth Service starting on port %s", port)
	log.Printf("🔗 Google OAuth endpoints:")
	log.Printf("   GET  http://localhost:%s/auth/google", port)
	log.Printf("   GET  http://localhost:%s/auth/google/callback", port)
	log.Printf("📁 Google Drive endpoints (require JWT):")
	log.Printf("   GET  http://localhost:%s/drive/files", port)
	log.Printf("   GET  http://localhost:%s/drive/search", port)
	log.Printf("   POST http://localhost:%s/drive/sync", port)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Ожидаем сигнала завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down OAuth Service...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("✅ OAuth Service stopped gracefully")
}
