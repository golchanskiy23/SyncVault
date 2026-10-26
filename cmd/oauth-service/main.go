package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"syncvault/internal/auth"
	"syncvault/internal/config"
	"syncvault/internal/oauth/google"
	syncadapters "syncvault/internal/sync/adapters"
	internalsync "syncvault/internal/sync"
)

func main() {
	log.Println("🚀 Starting OAuth Service with Google Drive integration...")

	cfg := config.LoadConfig()

	// PostgreSQL connection string
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)
	log.Printf("Connecting to DB: host=%s port=%d dbname=%s user=%s",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName, cfg.Database.User)
	log.Printf("OAuth redirect URL: %s", cfg.OAuth.GoogleDrive.OAuth.RedirectURL)

	db, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Address,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	jwtConfig := auth.DefaultJWTConfig()
	jwtConfig.AccessSecret = cfg.JWT.AccessSecret
	jwtConfig.RefreshSecret = cfg.JWT.RefreshSecret
	jwtConfig.AccessTTL = cfg.JWT.AccessTTL
	jwtConfig.RefreshTTL = cfg.JWT.RefreshTTL
	jwtConfig.Issuer = cfg.JWT.Issuer

	jwtService := auth.NewJWTService(jwtConfig, rdb)

	// Build config.GoogleDriveConfig for NewOAuthHandlers
	var googleDriveCfg *config.GoogleDriveConfig
	if cfg.OAuth.GoogleDrive != nil {
		googleDriveCfg = cfg.OAuth.GoogleDrive
	} else {
		log.Fatal("Google Drive config is missing in config.yml")
	}

	if err := (&google.GoogleDriveConfig{
		OAuth: &google.GoogleOAuthConfig{
			ClientID:     googleDriveCfg.OAuth.ClientID,
			ClientSecret: googleDriveCfg.OAuth.ClientSecret,
			Scopes:       googleDriveCfg.OAuth.Scopes,
		},
	}).Validate(); err != nil {
		log.Fatalf("Invalid Google Drive config: %v", err)
	}

	oauthHandlers := google.NewOAuthHandlers(db, googleDriveCfg, jwtService)

	// Синхронизация связок: устройства, связки и состояние файлов хранятся в PostgreSQL
	store := internalsync.NewPgStateStore(db)
	if err := store.Check(context.Background()); err != nil {
		log.Fatalf("Sync storage is not ready: %v", err)
	}
	engine := internalsync.NewSyncEngine(store, internalsync.KeepNewest)
	devices := internalsync.NewNodeRegistry()

	localRoot := cfg.Sync.LocalRoot
	if localRoot != "" {
		if localRoot, err = filepath.Abs(localRoot); err != nil {
			log.Fatalf("Invalid sync.localRoot: %v", err)
		}
		if fi, err := os.Stat(localRoot); err != nil || !fi.IsDir() {
			log.Fatalf("sync.localRoot %s is not an existing directory", localRoot)
		}
		log.Printf("Local device: folders inside %s", localRoot)
	} else {
		log.Println("⚠️ sync.localRoot is not set — folders of this computer cannot be used in pairs")
	}

	pairs := internalsync.NewPairManager(engine, devices, internalsync.NewPgPairStore(db), localRoot, cfg.Sync.AgentToken)
	pairs.SetDriveFactory(&driveNodeFactory{
		drive:  oauthHandlers.DriveAdapter(),
		tmpDir: os.TempDir(),
	})

	syncHandlers := internalsync.NewSyncHTTPHandlers(devices, pairs, cfg.Sync.AgentToken)
	syncHandlers.SetNodeStore(internalsync.NewPgNodeStore(db))
	if cfg.Sync.AgentToken == "" {
		log.Println("⚠️ sync.agentToken is not set — agents on other devices cannot connect")
	}

	syncInterval, err := cfg.SyncInterval()
	if err != nil {
		log.Fatalf("Invalid sync config: %v", err)
	}

	if err := syncHandlers.RestoreDevices(context.Background()); err != nil {
		log.Fatalf("Failed to restore devices: %v", err)
	}

	router := chi.NewRouter()

	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			log.Printf("[%s] %s %s", r.Method, r.URL.Path, r.RemoteAddr)
			next.ServeHTTP(w, r)
		})
	})

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now().UTC(),
			"service":   "oauth-service",
			"version":   "1.0.0",
		}
		json.NewEncoder(w).Encode(response)
	})

	oauthHandlers.RegisterRoutes(router)

	// Sync API (требует JWT: связки дают доступ к файлам устройств)
	router.Group(func(r chi.Router) {
		r.Use(jwtService.AuthMiddleware)

		r.Get("/sync/devices", syncHandlers.ListDevices)
		r.Get("/sync/devices/{id}/dirs", syncHandlers.ListDeviceDirs)
		r.Delete("/sync/devices/{id}", syncHandlers.DeleteDevice)

		r.Get("/sync/pairs", syncHandlers.ListPairs)
		r.Post("/sync/pairs", syncHandlers.CreatePair)
		r.Post("/sync/pairs/run", syncHandlers.RunAllPairs)
		r.Delete("/sync/pairs/{id}", syncHandlers.DeletePair)
		r.Post("/sync/pairs/{id}/run", syncHandlers.RunPair)
	})

	// API агентов на других устройствах (авторизация общим токеном sync.agentToken)
	router.Group(func(r chi.Router) {
		r.Use(syncHandlers.AgentAuth)

		r.Post("/sync/agents/register", syncHandlers.RegisterAgent)
		r.Post("/sync/agents/heartbeat", syncHandlers.AgentHeartbeat)
	})

	port := "8081"
	if envPort := os.Getenv("OAUTH_SERVICE_PORT"); envPort != "" {
		port = envPort
	}

	log.Printf("🌐 OAuth Service starting on port %s", port)
	log.Printf("   GET  http://localhost:%s/auth/google", port)
	log.Printf("   GET  http://localhost:%s/auth/google/callback", port)
	log.Printf("   GET  http://localhost:%s/drive/files", port)
	log.Printf("   POST http://localhost:%s/drive/sync", port)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Фоновая синхронизация всех узлов
	schedulerCtx, stopScheduler := context.WithCancel(context.Background())
	defer stopScheduler()
	if syncInterval > 0 {
		go internalsync.RunScheduler(schedulerCtx, pairs, syncInterval)
	} else {
		log.Println("Background sync is disabled (sync.interval: off)")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down OAuth Service...")
	stopScheduler()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("✅ OAuth Service stopped gracefully")
}

// driveNodeFactory реализует internalsync.DriveNodeFactory
type driveNodeFactory struct {
	drive  *google.DriveAdapter
	tmpDir string
}

func (f *driveNodeFactory) NewDriveNode(info internalsync.NodeInfo) (internalsync.Node, error) {
	api := syncadapters.NewGoogleDriveAdapter(f.drive, info.UserID, info.AccountID, info.FolderID)
	return internalsync.NewComplexStorage(info.ID, "google_drive", f.tmpDir, api), nil
}

func (f *driveNodeFactory) CheckDriveNode(ctx context.Context, info internalsync.NodeInfo) error {
	return f.drive.CheckAccount(ctx, info.UserID, info.AccountID, info.FolderID)
}