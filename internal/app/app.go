package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"syncvault/internal/config"
)

type App struct {
	httpServer *http.Server
	router     chi.Router
	server     *http.Server
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	shutdown   bool
	mu         sync.RWMutex
	config     *config.Config
}

func New(opts ...Option) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		ctx:    ctx,
		cancel: cancel,
		config: &config.Config{},
		router: chi.NewRouter(),
	}

	for _, opt := range opts {
		if err := opt(app); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	log.Println("Starting application...")

	a.setupMiddleware()
	a.setupRoutes()

	if a.httpServer == nil {
		a.httpServer = &http.Server{
			Addr:         a.config.Address(),
			Handler:      a.router,
			ReadTimeout:  a.config.HTTP.ReadTimeout,
			WriteTimeout: a.config.HTTP.WriteTimeout,
			IdleTimeout:  a.config.HTTP.IdleTimeout,
		}
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		log.Printf("HTTP server starting on %s", a.httpServer.Addr)

		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		<-a.ctx.Done()
		log.Println("Application context cancelled, shutting down HTTP server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	// a.startBackgroundServices()

	log.Println("Application started successfully")
	log.Printf("📚 API Documentation:")
	log.Printf(" 🌐 Public API: http://localhost:8080/api/v1/")
	log.Printf(" 🔧 Internal API: http://localhost:8080/internal/")
	log.Printf(" ❤️ Health Check: http://localhost:8080/api/v1/health")
	log.Printf(" 🏓 Ping: http://localhost:8080/api/v1/ping")

	<-a.ctx.Done()
	log.Println("Application context cancelled, exiting Run()")

	return nil
}

func (a *App) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	if a.shutdown {
		a.mu.Unlock()
		return nil
	}
	a.shutdown = true
	a.mu.Unlock()

	log.Println("Starting application shutdown...")

	a.cancel()

	if a.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, a.config.Shutdown.Timeout)
		defer cancel()

		log.Println("Shutting down HTTP server...")
		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}

	//a.shutdownBackgroundServices(ctx)

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All background services shutdown completed")
	case <-ctx.Done():
		log.Println("Shutdown timeout reached, forcing exit")
		return ctx.Err()
	}

	log.Println("Application shutdown completed")
	return nil
}

func (a *App) setupMiddleware() {
	a.router.Use(middleware.RequestID)
	a.router.Use(middleware.RealIP)
	a.router.Use(middleware.Logger)
	a.router.Use(middleware.Recoverer)
	a.router.Use(middleware.Timeout(60 * time.Second))
	a.router.Use(middleware.AllowContentType("application/json"))

	a.router.Use(func(next http.Handler) http.Handler {
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
	})
}

func (a *App) setupRoutes() {
	a.router.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", a.handleHealth)
		r.Get("/ping", a.handlePing)

		r.Route("/files", func(r chi.Router) {
			r.Get("/", a.handleListFiles)
			r.Post("/", a.handleCreateFile)
			r.Get("/{fileID}", a.handleGetFile)
			r.Put("/{fileID}", a.handleUpdateFile)
			r.Delete("/{fileID}", a.handleDeleteFile)
		})

		r.Route("/sync", func(r chi.Router) {
			r.Post("/start", a.handleStartSync)
			r.Post("/stop", a.handleStopSync)
			r.Get("/status", a.handleGetSyncStatus)
			r.Get("/jobs", a.handleListSyncJobs)
			r.Get("/jobs/{jobID}", a.handleGetSyncJob)
			r.Post("/jobs/{jobID}/cancel", a.handleCancelSyncJob)
		})

		r.Route("/storages", func(r chi.Router) {
			r.Get("/", a.handleListStorages)
			r.Post("/", a.handleCreateStorage)
			r.Get("/{storageID}", a.handleGetStorage)
			r.Put("/{storageID}", a.handleUpdateStorage)
			r.Delete("/{storageID}", a.handleDeleteStorage)
			r.Post("/{storageID}/connect", a.handleConnectStorage)
			r.Post("/{storageID}/disconnect", a.handleDisconnectStorage)
			r.Get("/{storageID}/status", a.handleGetStorageStatus)
			r.Get("/{storageID}/usage", a.handleGetStorageUsage)
		})

		r.Route("/conflicts", func(r chi.Router) {
			r.Get("/", a.handleListConflicts)
			r.Post("/{conflictID}/resolve", a.handleResolveConflict)
		})
	})

	a.router.Route("/internal", func(r chi.Router) {
		r.Use(a.internalAuthMiddleware)

		r.Route("/sync", func(r chi.Router) {
			r.Post("/coordinate", a.handleSyncCoordination)
			r.Get("/status/{nodeID}", a.handleNodeSyncStatus)
			r.Post("/heartbeat", a.handleNodeHeartbeat)
		})

		r.Route("/events", func(r chi.Router) {
			r.Get("/stream", a.handleEventStream)
			r.Post("/publish", a.handlePublishEvent)
		})

		r.Route("/health", func(r chi.Router) {
			r.Get("/readiness", a.handleReadinessCheck)
			r.Get("/liveness", a.handleLivenessCheck)
		})
	})
}

func (a *App) internalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalToken := r.Header.Get("X-Internal-Token")
		if internalToken != a.getInternalToken() {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) getInternalToken() string {
	return "syncvault-internal-token"
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy","service":"syncvault"}`))
}

func (a *App) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"pong"}`))
}

func (a *App) handleListFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`[{"id":"file1","path":"/documents/report.pdf","size":1024}]`))
}

func (a *App) handleCreateFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"id":"new-file-id","path":"/documents/new.pdf","size":2048}`))
}

func (a *App) handleGetFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"id":"` + fileID + `","path":"/documents/report.pdf","size":1024,"status":"synced"}`))
}

func (a *App) handleUpdateFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"File updated successfully"}`))
}

func (a *App) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"File deleted successfully"}`))
}

func (a *App) handleCreateSyncJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"id":"new-sync-job-id","status":"pending","createdAt":"2026-03-15T14:57:00Z"}`))
}

func (a *App) handleGetSyncJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"id":"job123","status":"running","progress":50,"total":100}`))
}

func (a *App) handleStartSyncJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Sync job started","status":"running"}`))
}

func (a *App) handleCancelSyncJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Sync job cancelled","status":"cancelled"}`))
}

func (a *App) handleListNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`[{"id":"node1","name":"Local Storage","type":"local","status":"online"}]`))
}

func (a *App) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"id":"new-node-id","name":"New Storage","type":"cloud","status":"offline"}`))
}

func (a *App) handleGetNode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"id":"node123","name":"Local Storage","type":"local","status":"online","capacity":1000000000,"usedSpace":500000000}`))
}

func (a *App) handleUpdateNodeStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Node status updated"}`))
}

func (a *App) handleListConflicts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`[{"id":"conflict1","fileID":"file1","conflictType":"content","resolved":false}]`))
}

func (a *App) handleResolveConflict(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Conflict resolved","resolved":true}`))
}

func (a *App) handleSyncCoordination(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Sync coordination completed","status":"success"}`))
}

func (a *App) handleNodeSyncStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"syncStatus":"in_progress","pendingFiles":5,"failedFiles":1}`))
}

func (a *App) handleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Heartbeat received","status":"alive"}`))
}

func (a *App) handleEventStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	event := "data: {\"type\":\"file_synced\",\"fileID\":\"file1\",\"timestamp\":\"2026-03-15T14:57:00Z\"}\n\n"
	w.Write([]byte(event))
}

func (a *App) handlePublishEvent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Event published","status":"success"}`))
}

func (a *App) handleReadinessCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ready","checks":{"database":"ok","storage":"ok","cache":"ok"}}`))
}

func (a *App) handleLivenessCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"alive","timestamp":"2026-03-15T14:57:00Z","uptime":"2h30m15s"}`))
}

// Sync Management Handlers
func (a *App) handleStartSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"job_id":"sync-job-123","node_id":"local-storage","status":"started","created_at":"2026-03-15T14:57:00Z"}`))
}

func (a *App) handleStopSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Sync stopped","status":"stopped"}`))
}

func (a *App) handleGetSyncStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"running","active_jobs":3,"completed_jobs":125,"failed_jobs":2,"files_processed":1500,"files_total":2000,"last_sync_at":"2026-03-15T14:55:00Z"}`))
}

func (a *App) handleListSyncJobs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`[{"id":"job-1","node_id":"local","status":"running","progress":75},{"id":"job-2","node_id":"cloud","status":"completed","progress":100}]`))
}

// Storage Management Handlers
func (a *App) handleListStorages(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`[{"id":"storage-1","name":"Local Storage","type":"local","status":"connected","capacity":1000000000000,"used":500000000000},{"id":"storage-2","name":"Cloud Storage","type":"s3","status":"connected","capacity":10000000000000,"used":2000000000000}]`))
}

func (a *App) handleCreateStorage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"id":"storage-3","name":"New Storage","type":"ftp","status":"disconnected","capacity":0,"used":0,"created_at":"2026-03-15T14:57:00Z"}`))
}

func (a *App) handleGetStorage(w http.ResponseWriter, r *http.Request) {
	storageID := chi.URLParam(r, "storageID")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"id":"` + storageID + `","name":"Local Storage","type":"local","status":"connected","capacity":1000000000000,"used":500000000000,"config":{"path":"/syncvault/data"}}`))
}

func (a *App) handleUpdateStorage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Storage updated successfully"}`))
}

func (a *App) handleDeleteStorage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleConnectStorage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Storage connected successfully","status":"connected"}`))
}

func (a *App) handleDisconnectStorage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Storage disconnected successfully","status":"disconnected"}`))
}

func (a *App) handleGetStorageStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"storage_id":"storage-1","status":"connected","last_check":"2026-03-15T14:57:00Z","latency_ms":15,"error_count":0,"uptime":"99.9%"}`))
}

func (a *App) handleGetStorageUsage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"storage_id":"storage-1","total_bytes":1000000000000,"used_bytes":500000000000,"available_bytes":500000000000,"file_count":15000,"usage_percent":50.0}`))
}

/*
func (a *App) startBackgroundServices() {
a.wg.Add(1)
go func() {
defer a.wg.Done()

ticker := time.NewTicker(30 * time.Second)
defer ticker.Stop()

for {
select {
case <-a.ctx.Done():
log.Println("Background service: context cancelled, stopping...")
return
case <-ticker.C:
log.Println("Background service: performing periodic task")
}
}
}()

log.Println("Background services started")
}

func (a *App) shutdownBackgroundServices(ctx context.Context) {
log.Println("Shutting down background services...")

shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
defer cancel()

done := make(chan struct{})
go func() {
time.Sleep(2 * time.Second)
close(done)
}()

select {
case <-done:
log.Println("Background services shutdown completed")
case <-shutdownCtx.Done():
log.Println("Background services shutdown timeout")
}
}*/
