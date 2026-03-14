package main

import (
	"context"
	"encoding/json"
	"fmt"
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

	// Sync engine
	store := internalsync.NewInMemoryStateStore()
	engine := internalsync.NewSyncEngine(store, internalsync.KeepNewest)
	registry := internalsync.NewNodeRegistry()
	syncHandlers := internalsync.NewSyncHTTPHandlers(registry, engine)

	// DriveAdapter для sync-узлов
	syncHandlers.SetDriveFactory(&driveNodeFactory{
		drive:  oauthHandlers.DriveAdapter(),
		tmpDir: os.TempDir(),
	})

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

	// Sync API
	router.Post("/sync/nodes/local", syncHandlers.RegisterLocalNode)
	router.Post("/sync/nodes/remote", syncHandlers.RegisterRemoteNode)
	router.Post("/sync/nodes/drive", syncHandlers.RegisterDriveNode)
	router.Post("/sync/nodes/heartbeat", syncHandlers.Heartbeat)
	router.Get("/sync/nodes", syncHandlers.ListNodes)
	router.Post("/sync/run", syncHandlers.SyncPair)
	router.Post("/sync/run/all", syncHandlers.SyncAll)
	router.Delete("/sync/nodes/{id}", syncHandlers.UnregisterNode)

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

// driveNodeFactory реализует internalsync.DriveNodeFactory
type driveNodeFactory struct {
	drive  *google.DriveAdapter
	tmpDir string
}

func (f *driveNodeFactory) NewDriveNode(userID, accountID, nodeID string) (internalsync.Node, error) {
	api := syncadapters.NewGoogleDriveAdapter(f.drive, userID, accountID)
	return internalsync.NewComplexStorage(nodeID, "google_drive", f.tmpDir, api), nil
}