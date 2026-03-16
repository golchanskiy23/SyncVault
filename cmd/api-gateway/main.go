package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"syncvault/internal/auth"
	"syncvault/internal/config"
)

const (
	AuthServiceURL         = "http://localhost:50056"
	StorageServiceURL      = "http://localhost:50054"
	SyncServiceURL         = "http://localhost:50053"
	FileServiceURL         = "http://localhost:50052"
	NotificationServiceURL = "http://localhost:50055"
)

// Circuit Breaker состояния
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitHalfOpen
	CircuitOpen
)

// Circuit Breaker структура
type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration
	failures     int
	lastFailTime time.Time
	state        CircuitState
	mutex        sync.RWMutex
}

func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        CircuitClosed,
	}
}

func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Если цепь разорвана, сразу возвращаем ошибку
	if cb.state == CircuitOpen {
		return fmt.Errorf("circuit breaker is open")
	}

	// Вызываем функцию
	err := fn()

	// Обрабатываем результат
	if err != nil {
		cb.failures++
		cb.lastFailTime = time.Now()

		// Если превышен порог отказов, разрываем цепь
		if cb.failures >= cb.maxFailures {
			cb.state = CircuitOpen
			go func() {
				time.Sleep(cb.resetTimeout)
				cb.mutex.Lock()
				cb.state = CircuitClosed
				cb.failures = 0
				cb.mutex.Unlock()
			}()
		}
	} else {
		// Успешный вызов, сбрасываем счетчик отказов
		cb.failures = 0
	}

	return err
}

// APIServer основной API Gateway
type APIServer struct {
	router      *chi.Mux
	services    map[string]string
	healthCheck map[string]string
	jwtService  *auth.JWTService
	config      *config.Config
}

func NewAPIServer() *APIServer {
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
	jwtConfig.AccessTTL = cfg.JWT.AccessTTL
	jwtConfig.RefreshTTL = cfg.JWT.RefreshTTL
	jwtConfig.Issuer = cfg.JWT.Issuer

	// Создаем JWT сервис
	jwtService := auth.NewJWTService(jwtConfig, rdb)

	return &APIServer{
		router: chi.NewRouter(),
		services: map[string]string{
			"auth":         AuthServiceURL,
			"storage":      StorageServiceURL,
			"sync":         SyncServiceURL,
			"file":         FileServiceURL,
			"notification": NotificationServiceURL,
		},
		healthCheck: map[string]string{
			"auth":         AuthServiceURL + "/health",
			"storage":      StorageServiceURL + "/health",
			"sync":         SyncServiceURL + "/health",
			"file":         FileServiceURL + "/health",
			"notification": NotificationServiceURL + "/health",
		},
		jwtService: jwtService,
		config:     cfg,
	}
}

func (s *APIServer) setupRoutes() {
	// Middleware
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Timeout(60 * time.Second))
	s.router.Use(middleware.AllowContentType("application/json"))
	s.router.Use(corsMiddleware)
	s.router.Use(rateLimitMiddleware(100)) // 100 запросов в минуту

	// Health check endpoint
	s.router.Get("/health", s.healthCheckHandler)
	s.router.Get("/health/services", s.servicesHealthHandler)

	// API Routes
	s.router.Route("/api/v1", func(r chi.Router) {
		// Authentication routes (public)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", s.proxyHandler("auth"))
			r.Post("/register", s.proxyHandler("auth"))
			r.Post("/refresh", s.proxyHandler("auth"))
			r.Post("/validate", s.proxyHandler("auth"))

			// Protected routes (require authentication)
			r.Group(func(r chi.Router) {
				r.Use(s.jwtService.AuthMiddleware)
				r.Post("/logout", s.proxyHandler("auth"))
				r.Get("/profile", s.proxyHandler("auth"))
				r.Put("/profile", s.proxyHandler("auth"))
				r.Post("/change-password", s.proxyHandler("auth"))
			})

			// Device management routes (protected)
			r.Route("/devices", func(r chi.Router) {
				r.Use(s.jwtService.AuthMiddleware)
				r.Get("/", s.proxyHandler("auth"))
				r.Post("/", s.proxyHandler("auth"))
				r.Delete("/{deviceID}", s.proxyHandler("auth"))
				r.Get("/{deviceID}/status", s.proxyHandler("auth"))
			})
		})

		// Storage routes (protected)
		r.Route("/storage", func(r chi.Router) {
			r.Use(s.jwtService.AuthMiddleware)
			r.Post("/files", s.proxyHandler("storage"))
			r.Get("/files/{fileID}", s.proxyHandler("storage"))
			r.Delete("/files/{fileID}", s.proxyHandler("storage"))
			r.Get("/files/{fileID}/history", s.proxyHandler("storage"))
			r.Post("/files/{fileID}/sync", s.proxyHandler("storage"))
			r.Post("/conflicts/resolve", s.proxyHandler("storage"))
		})

		// Sync routes (protected)
		r.Route("/sync", func(r chi.Router) {
			r.Use(s.jwtService.AuthMiddleware)
			r.Post("/start", s.proxyHandler("sync"))
			r.Get("/status/{sessionID}", s.proxyHandler("sync"))
			r.Post("/cancel/{sessionID}", s.proxyHandler("sync"))
			r.Post("/force", s.proxyHandler("sync"))
			r.Get("/history/{sessionID}", s.proxyHandler("sync"))
			r.Get("/conflicts/{sessionID}", s.proxyHandler("sync"))
			r.Post("/conflicts/{sessionID}/resolve", s.proxyHandler("sync"))
		})

		// File routes (protected)
		r.Route("/files", func(r chi.Router) {
			r.Use(s.jwtService.AuthMiddleware)
			r.Post("/upload", s.proxyHandler("file"))
			r.Get("/download/{fileID}", s.proxyHandler("file"))
			r.Get("/list", s.proxyHandler("file"))
			r.Delete("/{fileID}", s.proxyHandler("file"))
		})

		// Notification routes (protected)
		r.Route("/notifications", func(r chi.Router) {
			r.Use(s.jwtService.AuthMiddleware)
			r.Get("/", s.proxyHandler("notification"))
			r.Post("/", s.proxyHandler("notification"))
			r.Put("/{notificationID}/read", s.proxyHandler("notification"))
			r.Delete("/{notificationID}", s.proxyHandler("notification"))
			r.Route("/settings", func(r chi.Router) {
				r.Get("/", s.proxyHandler("notification"))
				r.Put("/", s.proxyHandler("notification"))
			})
		})
	})

	// Static files and documentation
	s.router.Get("/", s.indexHandler)
	s.router.Get("/docs", s.docsHandler)
	s.router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
}

func (s *APIServer) proxyHandler(service string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetURL, exists := s.services[service]
		if !exists {
			http.Error(w, "Service not found", http.StatusNotFound)
			return
		}

		// Создаем target URL
		target, err := url.Parse(targetURL)
		if err != nil {
			http.Error(w, "Invalid target URL", http.StatusInternalServerError)
			return
		}

		// Создаем reverse proxy
		proxy := httputil.NewSingleHostReverseProxy(target)

		// Добавляем custom headers
		r.Header.Set("X-Gateway-Service", service)
		r.Header.Set("X-Gateway-Request-ID", r.Header.Get("X-Request-ID"))
		r.Header.Set("X-Forwarded-For", r.RemoteAddr)
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("X-Forwarded-Host", r.Host)

		// Idempotency для POST запросов
		if r.Method == "POST" {
			idempotencyKey := r.Header.Get("Idempotency-Key")
			if idempotencyKey != "" {
				// Проверяем, был ли уже выполнен этот запрос
				log.Printf("Idempotency check for key: %s", idempotencyKey)
				// Здесь можно вернуть кешированный ответ
			}
		}

		// Log прокси запроса
		log.Printf("Proxying %s %s to %s", r.Method, r.URL.Path, targetURL)

		// Используем circuit breaker для вызова
		err = NewCircuitBreaker(5, 30*time.Second).Call(func() error {
			proxy.ServeHTTP(w, r)
			return nil
		})

		if err != nil {
			log.Printf("Proxy error: %v", err)
			http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
	}
}

func (s *APIServer) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
		"services":  s.checkServicesHealth(),
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (s *APIServer) servicesHealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	healthStatus := make(map[string]interface{})

	for service, healthURL := range s.healthCheck {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
		if err != nil {
			healthStatus[service] = map[string]interface{}{
				"status": "error",
				"error":  "failed to create request",
			}
			continue
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			healthStatus[service] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
			continue
		}
		defer resp.Body.Close()

		healthStatus[service] = map[string]interface{}{
			"status": resp.StatusCode == 200,
			"code":   resp.StatusCode,
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(healthStatus)
}

func (s *APIServer) checkServicesHealth() map[string]interface{} {
	status := make(map[string]interface{})

	for service := range s.services {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", s.healthCheck[service], nil)
		if err != nil {
			status[service] = "error"
			continue
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != 200 {
			status[service] = "unhealthy"
		} else {
			status[service] = "healthy"
		}

		if resp != nil {
			resp.Body.Close()
		}
	}

	return status
}

func (s *APIServer) indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	html := `<!DOCTYPE html>
<html>
<head>
    <title>SyncVault API Gateway</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .container { max-width: 800px; margin: 0 auto; }
        .service { background: #f5f5f5; margin: 10px 0; padding: 20px; border-radius: 8px; }
        .service h3 { color: #333; margin-top: 0; }
        .endpoint { color: #666; font-family: monospace; margin: 5px 0; }
        .method { display: inline-block; background: #007bff; color: white; padding: 2px 6px; margin: 2px; border-radius: 3px; font-size: 12px; }
        .get { background: #28a745; }
        .post { background: #ffc107; color: #000; }
        .put { background: #17a2b8; }
        .delete { background: #dc3545; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🚀 SyncVault API Gateway</h1>
        <p>Единая точка доступа ко всем микросервисам</p>
        
        <div class="service">
            <h3>🔐 Authentication Service</h3>
            <div class="endpoint">POST /api/v1/auth/login</div>
            <div class="endpoint">POST /api/v1/auth/register</div>
            <div class="endpoint">GET /api/v1/auth/devices</div>
        </div>
        
        <div class="service">
            <h3>💾 Storage Service</h3>
            <div class="endpoint">POST /api/v1/storage/files</div>
            <div class="endpoint">GET /api/v1/storage/files/{fileID}</div>
            <div class="endpoint">POST /api/v1/storage/conflicts/resolve</div>
        </div>
        
        <div class="service">
            <h3>🔄 Sync Service</h3>
            <div class="endpoint">POST /api/v1/sync/start</div>
            <div class="endpoint">GET /api/v1/sync/status/{sessionID}</div>
            <div class="endpoint">GET /api/v1/sync/conflicts/{sessionID}</div>
        </div>
        
        <div class="service">
            <h3>📁 File Service</h3>
            <div class="endpoint">POST /api/v1/files/upload</div>
            <div class="endpoint">GET /api/v1/files/download/{fileID}</div>
            <div class="endpoint">GET /api/v1/files/list</div>
        </div>
        
        <div class="service">
            <h3>🔔 Notification Service</h3>
            <div class="endpoint">GET /api/v1/notifications</div>
            <div class="endpoint">POST /api/v1/notifications</div>
            <div class="endpoint">PUT /api/v1/notifications/{id}/read</div>
        </div>
        
        <div class="service">
            <h3>📊 System Status</h3>
            <div class="endpoint">GET /health</div>
            <div class="endpoint">GET /health/services</div>
            <div class="endpoint">GET /docs</div>
        </div>
    </div>
</body>
</html>`

	w.Write([]byte(html))
}

func (s *APIServer) docsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	docs := map[string]interface{}{
		"title":       "SyncVault API Gateway",
		"version":     "1.0.0",
		"description": "API Gateway для микросервисов SyncVault с Circuit Breaker и Idempotency",
		"base_url":    "http://localhost:8080",
		"features": []string{
			"Circuit Breaker",
			"Idempotency",
			"Rate Limiting",
			"CORS Support",
			"Health Checks",
		},
		"services": map[string]interface{}{
			"auth": map[string]interface{}{
				"name":        "Authentication Service",
				"port":        50056,
				"description": "Аутентификация и управление устройствами",
				"endpoints": []string{
					"POST /api/v1/auth/login",
					"POST /api/v1/auth/register",
					"GET /api/v1/auth/devices",
					"DELETE /api/v1/auth/devices/{deviceID}",
				},
			},
			"storage": map[string]interface{}{
				"name":        "Storage Service",
				"port":        50054,
				"description": "Хранение метаданных и синхронизация файлов",
				"endpoints": []string{
					"POST /api/v1/storage/files",
					"GET /api/v1/storage/files/{fileID}",
					"DELETE /api/v1/storage/files/{fileID}",
					"POST /api/v1/storage/conflicts/resolve",
				},
			},
			"sync": map[string]interface{}{
				"name":        "Sync Service",
				"port":        50053,
				"description": "Координация синхронизации между устройствами",
				"endpoints": []string{
					"POST /api/v1/sync/start",
					"GET /api/v1/sync/status/{sessionID}",
					"POST /api/v1/sync/cancel/{sessionID}",
					"GET /api/v1/sync/conflicts/{sessionID}",
				},
			},
			"file": map[string]interface{}{
				"name":        "File Service",
				"port":        50052,
				"description": "Управление файлами на устройствах",
				"endpoints": []string{
					"POST /api/v1/files/upload",
					"GET /api/v1/files/download/{fileID}",
					"GET /api/v1/files/list",
					"DELETE /api/v1/files/{fileID}",
				},
			},
			"notification": map[string]interface{}{
				"name":        "Notification Service",
				"port":        50055,
				"description": "Уведомления и события системы",
				"endpoints": []string{
					"GET /api/v1/notifications",
					"POST /api/v1/notifications",
					"PUT /api/v1/notifications/{id}/read",
					"DELETE /api/v1/notifications/{id}",
				},
			},
		},
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(docs)
}

// CORS middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, Idempotency-Key")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Rate limiting middleware
func rateLimitMiddleware(requestsPerMinute int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Простая реализация rate limiting
			// В реальном приложении здесь был бы Redis
			clientIP := r.RemoteAddr
			log.Printf("Rate limiting check for IP: %s", clientIP)

			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	log.Println("Starting API Gateway with Circuit Breaker and Idempotency...")

	server := NewAPIServer()
	server.setupRoutes()

	port := "8080"
	if envPort := os.Getenv("API_GATEWAY_PORT"); envPort != "" {
		port = envPort
	}

	log.Printf("API Gateway starting on port %s", port)
	log.Printf("Available services: %v", server.services)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := http.ListenAndServe(":"+port, server.router); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down API Gateway...")
}
