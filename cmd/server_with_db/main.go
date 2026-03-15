package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server представляет HTTP сервер с интеграцией базы данных
type Server struct {
	httpServer *http.Server
	router     chi.Router
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	shutdown   bool
	mu         sync.RWMutex
	db         *pgxpool.Pool
}

// NewServer создает новый сервер
func NewServer() *Server {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Server{
		ctx:    ctx,
		cancel: cancel,
		router: chi.NewRouter(),
	}
}

// ConnectDB подключается к базе данных
func (s *Server) ConnectDB() error {
	connString := "postgres://postgres:postgres@localhost:5432/syncvault?sslmode=disable"
	
	pool, err := pgxpool.New(s.ctx, connString)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	
	s.db = pool
	log.Println("✓ Connected to database")
	return nil
}

// setupMiddleware настраивает middleware
func (s *Server) setupMiddleware() {
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.URLFormat)
	s.router.Use(middleware.Timeout(60 * time.Second))
	s.router.Use(s.corsMiddleware)
}

// corsMiddleware - простой CORS middleware
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// setupRoutes настраивает маршруты
func (s *Server) setupRoutes() {
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/ping", s.handlePing)
		r.Get("/files", s.handleListFiles)
		r.Post("/files", s.handleCreateFile)
		r.Get("/files/{id}", s.handleGetFile)
		r.Put("/files/{id}", s.handleUpdateFile)
		r.Delete("/files/{id}", s.handleDeleteFile)
		r.Get("/files/{id}/versions", s.handleGetFileVersions)
		r.Get("/stats", s.handleStats)
	})
	
	// Маршрут для демонстрации интеграции с БД
	s.router.Get("/db-demo", s.handleDBDemo)
}

// Run запускает сервер
func (s *Server) Run(ctx context.Context) error {
	log.Println("Starting SyncVault server with database integration...")
	
	// Подключаемся к базе данных
	if err := s.ConnectDB(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	
	s.setupMiddleware()
	s.setupRoutes()
	
	if s.httpServer == nil {
		s.httpServer = &http.Server{
			Addr:         ":8080",
			Handler:      s.router,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		}
	}
	
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		log.Printf("HTTP server starting on %s", s.httpServer.Addr)
		
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
	
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-s.ctx.Done()
		log.Println("Server context cancelled, shutting down...")
		
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()
	
	log.Println("✓ Server started successfully!")
	log.Printf("📚 API: http://localhost:8080/api/v1/")
	log.Printf("❤️  Health: http://localhost:8080/api/v1/health")
	log.Printf("🏓 Ping: http://localhost:8080/api/v1/ping")
	log.Printf("🗄️ DB Demo: http://localhost:8080/db-demo")
	
	<-s.ctx.Done()
	log.Println("Server shutdown completed")
	return nil
}

// Shutdown останавливает сервер
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return nil
	}
	s.shutdown = true
	s.mu.Unlock()
	
	log.Println("Starting server shutdown...")
	
	// Закрываем подключение к базе данных
	if s.db != nil {
		s.db.Close()
		log.Println("✓ Database connection closed")
	}
	
	// Отменяем контекст
	s.cancel()
	
	s.wg.Wait()
	return nil
}

// Обработчики маршрутов

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Проверяем подключение к базе данных
	if s.db != nil {
		var result int
		err := s.db.QueryRow(r.Context(), "SELECT 1").Scan(&result)
		if err != nil {
			http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","database":"connected","result":%d}`, result)
	} else {
		http.Error(w, "Database not connected", http.StatusServiceUnavailable)
	}
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"message":"pong","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	// Получаем файлы из базы данных
	rows, err := s.db.Query(r.Context(), `
		SELECT id, user_id, file_name, file_size_bytes, created_at, updated_at
		FROM files 
		WHERE is_deleted = false 
		ORDER BY updated_at DESC 
		LIMIT 10
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	
	var files []map[string]interface{}
	for rows.Next() {
		var id, userID int64
		var fileName string
		var fileSize int64
		var createdAt, updatedAt time.Time
		
		if err := rows.Scan(&id, &userID, &fileName, &fileSize, &createdAt, &updatedAt); err != nil {
			log.Printf("Failed to scan file: %v", err)
			continue
		}
		
		files = append(files, map[string]interface{}{
			"id":           id,
			"user_id":      userID,
			"file_name":    fileName,
			"file_size":    fileSize,
			"created_at":   createdAt,
			"updated_at":   updatedAt,
		})
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"files":%d,"data":%v}`, len(files), files)
}

func (s *Server) handleCreateFile(w http.ResponseWriter, r *http.Request) {
	// Создаем файл в базе данных
	var fileID int64
	err := s.db.QueryRow(r.Context(), `
		INSERT INTO files (user_id, file_path, file_name, file_size_bytes, mime_type, 
		                 checksum_md5, checksum_sha256, is_deleted, created_at, updated_at)
		VALUES (1, '/api/test.txt', 'test.txt', 1024, 'text/plain', 
		        'api_md5', 'api_sha256', false, NOW(), NOW())
		RETURNING id
	`).Scan(&fileID)
	
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create file: %v", err), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"message":"File created","file_id":%d}`, fileID)
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	
	var fileName string
	var fileSize int64
	err := s.db.QueryRow(r.Context(), `
		SELECT file_name, file_size_bytes 
		FROM files 
		WHERE id = $1 AND is_deleted = false
	`, fileID).Scan(&fileName, &fileSize)
	
	if err != nil {
		http.Error(w, fmt.Sprintf("File not found: %v", err), http.StatusNotFound)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"file_id":"%s","file_name":"%s","file_size":%d}`, fileID, fileName, fileSize)
}

func (s *Server) handleUpdateFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	
	// Обновляем файл в базе данных
	_, err := s.db.Exec(r.Context(), `
		UPDATE files 
		SET updated_at = NOW(), file_size_bytes = file_size_bytes + 100
		WHERE id = $1 AND is_deleted = false
	`, fileID)
	
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update file: %v", err), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"message":"File updated","file_id":"%s"}`, fileID)
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	
	// Удаляем файл в базе данных
	_, err := s.db.Exec(r.Context(), `
		UPDATE files 
		SET is_deleted = true, updated_at = NOW()
		WHERE id = $1
	`, fileID)
	
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete file: %v", err), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"message":"File deleted","file_id":"%s"}`, fileID)
}

func (s *Server) handleGetFileVersions(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	
	// Получаем версии файла с window функцией
	rows, err := s.db.Query(r.Context(), `
		WITH versioned_files AS (
			SELECT 
				fv.id,
				fv.file_id,
				fv.version_number,
				fv.file_size_bytes,
				fv.created_at,
				ROW_NUMBER() OVER (PARTITION BY fv.file_id ORDER BY fv.version_number DESC) as rn
			FROM file_versions fv
			WHERE fv.file_id = $1
		)
		SELECT id, file_id, version_number, file_size_bytes, created_at
		FROM versioned_files
		ORDER BY version_number DESC
	`, fileID)
	
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get file versions: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	
	var versions []map[string]interface{}
	for rows.Next() {
		var id, fileID int64
		var versionNumber int
		var fileSize int64
		var createdAt time.Time
		
		if err := rows.Scan(&id, &fileID, &versionNumber, &fileSize, &createdAt); err != nil {
			log.Printf("Failed to scan file version: %v", err)
			continue
		}
		
		versions = append(versions, map[string]interface{}{
			"id":            id,
			"file_id":       fileID,
			"version_number": versionNumber,
			"file_size":    fileSize,
			"created_at":    createdAt,
		})
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"file_id":"%s","versions":%d,"data":%v}`, fileID, len(versions), versions)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	// Получаем статистику базы данных
	stats := make(map[string]interface{})
	
	// Количество файлов
	var fileCount int64
	s.db.QueryRow(r.Context(), "SELECT COUNT(*) FROM files WHERE is_deleted = false").Scan(&fileCount)
	stats["files_count"] = fileCount
	
	// Количество версий
	var versionCount int64
	s.db.QueryRow(r.Context(), "SELECT COUNT(*) FROM file_versions").Scan(&versionCount)
	stats["versions_count"] = versionCount
	
	// Размер базы данных
	var dbSize string
	s.db.QueryRow(r.Context(), "SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&dbSize)
	stats["database_size"] = dbSize
	
	// Статистика пула
	if s.db != nil {
		poolStats := s.db.Stat()
		stats["pool_connections"] = map[string]interface{}{
			"acquired": poolStats.AcquiredConns(),
			"total":    poolStats.TotalConns(),
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"stats":%v}`, stats)
}

func (s *Server) handleDBDemo(w http.ResponseWriter, r *http.Request) {
	// Демонстрация интеграции с базой данных
	demo := make(map[string]interface{})
	
	// 1. Проверяем подключение
	var result int
	err := s.db.QueryRow(r.Context(), "SELECT 1").Scan(&result)
	if err != nil {
		demo["connection"] = map[string]interface{}{
			"status": "error",
			"error":  err.Error(),
		}
	} else {
		demo["connection"] = map[string]interface{}{
			"status": "ok",
			"result": result,
		}
	}
	
	// 2. Создаем тестовую запись
	var userID int64
	err = s.db.QueryRow(r.Context(), `
		INSERT INTO users (username, email, password_hash, storage_quota_bytes, used_storage_bytes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (username) DO NOTHING
		RETURNING id
	`, "server_demo", "server@example.com", "hashed_password", int64(10)*1024*1024*1024, int64(0)).Scan(&userID)
	
	if err != nil {
		demo["user_creation"] = map[string]interface{}{
			"status": "error",
			"error":  err.Error(),
		}
	} else {
		demo["user_creation"] = map[string]interface{}{
			"status": "ok",
			"user_id": userID,
		}
	}
	
	// 3. Получаем статистику
	var userCount, fileCount int64
	s.db.QueryRow(r.Context(), "SELECT COUNT(*) FROM users").Scan(&userCount)
	s.db.QueryRow(r.Context(), "SELECT COUNT(*) FROM files WHERE is_deleted = false").Scan(&fileCount)
	
	demo["statistics"] = map[string]interface{}{
		"users": userCount,
		"files": fileCount,
		"timestamp": time.Now(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"database_integration_demo":%v}`, demo)
}

func main() {
	var (
		port = flag.String("port", "8080", "Server port")
	)
	flag.Parse()
	
	server := NewServer()
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Run(ctx); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()
	
	sig := <-sigChan
	log.Printf("Received signal: %v, starting graceful shutdown", sig)
	
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shutdownCancel()
	
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during shutdown: %v", err)
		os.Exit(1)
	}
}
