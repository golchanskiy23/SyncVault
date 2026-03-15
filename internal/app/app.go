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
	httpSwagger "github.com/swaggo/http-swagger"

	"syncvault/docs"
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
	log.Printf("   🌐 Public API: http://localhost:8080/api/v1/")
	log.Printf("   🔧 Internal API: http://localhost:8080/internal/")
	log.Printf("   ❤️  Health Check: http://localhost:8080/api/v1/health")
	log.Printf("   🏓 Ping: http://localhost:8080/api/v1/ping")
	log.Printf("   📖 Swagger UI: http://localhost:8080/swagger/index.html")
	log.Printf("   � Swagger UI: http://localhost:8080/swagger-ui")
	log.Printf("   �📚 Custom Docs: http://localhost:8080/docs")
	log.Printf("   📄 Swagger JSON: http://localhost:8080/swagger.json")

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

	// Отмена контекста для всех горутин
	a.cancel()

	// Graceful shutdown HTTP сервера
	if a.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}

	a.wg.Wait()
	log.Println("Application shutdown completed")
	return nil
}

// Public exported methods for testing
func (a *App) HandleCreateFile(w http.ResponseWriter, r *http.Request) {
	a.handleCreateFile(w, r)
}

func (a *App) HandleGetFile(w http.ResponseWriter, r *http.Request) {
	a.handleGetFile(w, r)
}

func (a *App) HandleUpdateFile(w http.ResponseWriter, r *http.Request) {
	a.handleUpdateFile(w, r)
}

func (a *App) HandleDeleteFile(w http.ResponseWriter, r *http.Request) {
	a.handleDeleteFile(w, r)
}

func (a *App) HandleStartSync(w http.ResponseWriter, r *http.Request) {
	a.handleStartSync(w, r)
}

func (a *App) HandleGetSyncStatus(w http.ResponseWriter, r *http.Request) {
	a.handleGetSyncStatus(w, r)
}

func (a *App) HandleStopSync(w http.ResponseWriter, r *http.Request) {
	a.handleStopSync(w, r)
}

func (a *App) HandleCreateStorage(w http.ResponseWriter, r *http.Request) {
	a.handleCreateStorage(w, r)
}

func (a *App) HandleGetStorageUsage(w http.ResponseWriter, r *http.Request) {
	a.handleGetStorageUsage(w, r)
}

func (a *App) HandleGetStorage(w http.ResponseWriter, r *http.Request) {
	a.handleGetStorage(w, r)
}

func (a *App) InternalAuthMiddleware(next http.Handler) http.Handler {
	return a.internalAuthMiddleware(next)
}

func (a *App) HandleHealth(w http.ResponseWriter, r *http.Request) {
	a.handleHealth(w, r)
}

func (a *App) HandlePing(w http.ResponseWriter, r *http.Request) {
	a.handlePing(w, r)
}

// GetRouter - возвращает chi роутер для тестирования
func (a *App) GetRouter() http.Handler {
	return a.router
}

// GetHTTPServer - возвращает HTTP сервер для тестирования
func (a *App) GetHTTPServer() *http.Server {
	return a.httpServer
}

// SetupTestRoutes - настройка роутов для тестов
func (a *App) SetupTestRoutes() {
	a.setupMiddleware()
	a.setupRoutes()
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
	// Swagger Documentation Routes
	a.setupSwaggerRoutes()

	// Публичные API endpoints
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

		r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
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

func (a *App) setupSwaggerRoutes() {
	a.router.Get("/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		swaggerJSON, err := docs.SwaggerJSON()
		if err != nil {
			http.Error(w, "Swagger JSON not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(swaggerJSON)
	})

	a.router.Get("/swagger.yaml", func(w http.ResponseWriter, r *http.Request) {
		swaggerYAML, err := docs.SwaggerYAML()
		if err != nil {
			http.Error(w, "Swagger YAML not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/x-yaml")
		w.Write(swaggerYAML)
	})

	a.router.Get("/swagger", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/index.html", http.StatusMovedPermanently)
	})

	a.router.Get("/swagger-ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/index.html", http.StatusMovedPermanently)
	})

	a.router.Handle("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger.json"),
	))

	a.router.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		html := `<!DOCTYPE html>
<html>
<head>
    <title>SyncVault API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui.css" />
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin:0; background: #fafafa; }
        .swagger-ui .topbar { display: none; }
        .custom-header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
            text-align: center;
            margin-bottom: 20px;
        }
        .custom-header h1 {
            margin: 0;
            font-size: 2.5em;
            font-weight: 300;
        }
        .custom-header p {
            margin: 10px 0 0 0;
            opacity: 0.8;
        }
    </style>
</head>
<body>
    <div class="custom-header">
        <h1>🚀 SyncVault API</h1>
        <p>Distributed File Synchronization System</p>
    </div>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@4.15.5/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: '/swagger.json',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                defaultModelsExpandDepth: 2,
                displayRequestDuration: true,
                docExpansion: "none",
                operationsSorter: "alpha",
                tagsSorter: "alpha",
                tryItOutEnabled: true,
                filter: true,
                supportedSubmitMethods: ['get', 'post', 'put', 'delete', 'patch']
            });
        };
    </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	})
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
