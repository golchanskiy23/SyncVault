package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/timestamppb"

	"syncvault/internal/auth"
	"syncvault/internal/config"
	authv1 "syncvault/internal/grpc/proto/auth"
	"syncvault/internal/oauth/google"
	"syncvault/internal/storage"
)

// AuthService микросервис для аутентификации и авторизации
type AuthService struct {
	authv1.UnimplementedAuthServiceServer
	deviceManager *storage.DeviceManager
	jwtService    *auth.JWTService
	config        *config.Config
}

func NewAuthService() *AuthService {
	// Загружаем конфигурацию
	cfg := config.LoadConfig()

	// Создаем Redis клиент
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Создаем JWT конфигурацию
	jwtConfig := auth.DefaultJWTConfig()
	jwtConfig.AccessSecret = cfg.JWT.AccessSecret
	jwtConfig.RefreshSecret = cfg.JWT.RefreshSecret

	// Создаем JWT сервис
	jwtService := auth.NewJWTService(jwtConfig, rdb)

	return &AuthService{
		deviceManager: storage.NewDeviceManager(nil),
		jwtService:    jwtService,
		config:        cfg,
	}
}

func (s *AuthService) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	log.Printf("Login attempt for user: %s", req.Email)

	if req.Email == "test@example.com" && req.Password == "password123" {
		userID := "user_123"
		role := "user"

		tokenPair, err := s.jwtService.GenerateTokenPair(ctx, userID, req.Email, role)
		if err != nil {
			log.Printf("Failed to generate tokens: %v", err)
			return nil, fmt.Errorf("authentication failed")
		}

		return &authv1.LoginResponse{
			AccessToken:  tokenPair.AccessToken,
			RefreshToken: tokenPair.RefreshToken,
			TokenType:    tokenPair.TokenType,
			ExpiresIn:    int32(tokenPair.ExpiresIn),
			User: &authv1.User{
				UserId:    userID,
				Email:     req.Email,
				Username:  "testuser",
				FirstName: "Test",
				LastName:  "User",
				IsActive:  true,
				CreatedAt: timestamppb.Now(),
				UpdatedAt: timestamppb.Now(),
			},
		}, nil
	}

	return nil, fmt.Errorf("invalid credentials")
}

func main() {
	log.Println("Starting Auth Service microservice...")

	// Загружаем конфигурацию
	cfg := config.LoadConfig()

	// Создаем подключение к PostgreSQL
	db, err := pgxpool.New(context.Background(), fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.User, cfg.Database.Password, cfg.Database.DBName, cfg.Database.SSLMode))
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

	// Создаем JWT конфигурацию
	jwtConfig := auth.DefaultJWTConfig()
	jwtConfig.AccessSecret = cfg.JWT.AccessSecret
	jwtConfig.RefreshSecret = cfg.JWT.RefreshSecret
	jwtConfig.AccessTTL = cfg.JWT.AccessTTL
	jwtConfig.RefreshTTL = cfg.JWT.RefreshTTL
	jwtConfig.Issuer = cfg.JWT.Issuer

	// Создаем JWT сервис
	jwtService := auth.NewJWTService(jwtConfig, rdb)

	// Создаем OAuth handlers если настроен
	var oauthHandlers *google.OAuthHandlers
	if cfg.OAuth.GoogleDrive != nil {
		oauthHandlers = google.NewOAuthHandlers(db, cfg.OAuth.GoogleDrive, jwtService)
		log.Println("OAuth handlers initialized")
	}

	// Запускаем HTTP сервер в отдельной горутине
	go func() {
		log.Println("Starting HTTP Auth Service...")

		// Создаем HTTP сервер
		router := chi.NewRouter()

		// Middleware
		router.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				next.ServeHTTP(w, r)
			})
		})

		// Health check
		router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"status":    "healthy",
				"timestamp": time.Now().UTC(),
				"service":   "auth-service",
				"version":   "1.0.0",
			}
			json.NewEncoder(w).Encode(response)
		})

		// Login endpoint
		router.Post("/login", func(w http.ResponseWriter, r *http.Request) {
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			if req["email"] == "test@example.com" && req["password"] == "password123" {
				tokenPair, err := jwtService.GenerateTokenPair(context.Background(), "user_123", req["email"], "user")
				if err != nil {
					http.Error(w, "Authentication failed", http.StatusInternalServerError)
					return
				}

				response := map[string]interface{}{
					"access_token":  tokenPair.AccessToken,
					"refresh_token": tokenPair.RefreshToken,
					"token_type":    tokenPair.TokenType,
					"expires_in":    tokenPair.ExpiresIn,
					"user": map[string]interface{}{
						"id":        "user_123",
						"email":     req["email"],
						"username":  "testuser",
						"is_active": true,
					},
				}
				json.NewEncoder(w).Encode(response)
			} else {
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			}
		})

		// Token validation endpoint
		router.Post("/validate", func(w http.ResponseWriter, r *http.Request) {
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			claims, err := jwtService.ValidateToken(req["access_token"], auth.AccessTokenType)
			if err != nil {
				response := map[string]interface{}{
					"valid":   false,
					"message": "Invalid token: " + err.Error(),
				}
				json.NewEncoder(w).Encode(response)
				return
			}

			response := map[string]interface{}{
				"valid": true,
				"user": map[string]interface{}{
					"id":        claims.UserID,
					"email":     claims.Email,
					"username":  "testuser",
					"is_active": true,
				},
			}
			json.NewEncoder(w).Encode(response)
		})

		// Token refresh endpoint
		router.Post("/refresh", func(w http.ResponseWriter, r *http.Request) {
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			tokenPair, err := jwtService.RefreshTokens(r.Context(), req["refresh_token"])
			if err != nil {
				http.Error(w, "Token refresh failed", http.StatusUnauthorized)
				return
			}

			response := map[string]interface{}{
				"access_token": tokenPair.AccessToken,
				"token_type":   tokenPair.TokenType,
				"expires_in":   tokenPair.ExpiresIn,
			}
			json.NewEncoder(w).Encode(response)
		})

		// Protected endpoint
		router.Group(func(r chi.Router) {
			r.Use(jwtService.AuthMiddleware)
			r.Get("/profile", func(w http.ResponseWriter, r *http.Request) {
				userID, ok := auth.GetUserIDFromContext(r.Context())
				if !ok {
					http.Error(w, "User not found", http.StatusUnauthorized)
					return
				}

				response := map[string]interface{}{
					"id":        userID,
					"email":     "test@example.com",
					"username":  "testuser",
					"is_active": true,
				}
				json.NewEncoder(w).Encode(response)
			})

			r.Post("/logout", func(w http.ResponseWriter, r *http.Request) {
				userID, ok := auth.GetUserIDFromContext(r.Context())
				if !ok {
					http.Error(w, "User not found", http.StatusUnauthorized)
					return
				}

				err := jwtService.Logout(r.Context(), userID)
				if err != nil {
					http.Error(w, "Logout failed", http.StatusInternalServerError)
					return
				}

				response := map[string]interface{}{
					"success": true,
					"message": "Logged out successfully",
				}
				json.NewEncoder(w).Encode(response)
			})
		})

		// Добавляем OAuth роуты если настроены
		if oauthHandlers != nil {
			oauthHandlers.RegisterRoutes(router)
			log.Println("OAuth routes registered")
		}

		port := "50056"
		if envPort := os.Getenv("AUTH_SERVICE_PORT"); envPort != "" {
			port = envPort
		}

		if err := http.ListenAndServe(":"+port, router); err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	// Создаем gRPC сервер
	grpcServer := grpc.NewServer()

	// Регистрируем сервисы
	authService := NewAuthService()
	authv1.RegisterAuthServiceServer(grpcServer, authService)

	// Включаем reflection для разработки
	reflection.Register(grpcServer)

	// Настраиваем порт
	port := "50057"
	if envPort := os.Getenv("AUTH_GRPC_SERVICE_PORT"); envPort != "" {
		port = envPort
	}

	// Создаем listener
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	log.Printf("Auth gRPC Service listening on port %s", port)

	// Запускаем сервер в горутине
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Auth Service...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutdown timeout, forcing stop...")
		grpcServer.Stop()
	case <-stopped:
		log.Println("Auth Service stopped gracefully")
	}
}
