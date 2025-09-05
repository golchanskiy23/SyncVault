package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	router chi.Router
	server *http.Server
	config ServerConfig
}

type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

func NewServer(config ServerConfig) *Server {
	r := chi.NewRouter()

	return &Server{
		router: r,
		config: config,
	}
}

func (s *Server) SetupMiddleware() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(60 * time.Second))
	s.router.Use(middleware.AllowContentType("application/json"))

	s.router.Use(func(next http.Handler) http.Handler {
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

func (s *Server) SetupRoutes() {
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.handleHealth)
		r.Get("/ping", s.handlePing)

		r.Route("/files", func(r chi.Router) {
			r.Get("/", s.handleListFiles)
			r.Post("/", s.handleCreateFile)
			r.Get("/{fileID}", s.handleGetFile)
			r.Put("/{fileID}", s.handleUpdateFile)
			r.Delete("/{fileID}", s.handleDeleteFile)
		})

		r.Route("/sync", func(r chi.Router) {
			r.Post("/jobs", s.handleCreateSyncJob)
			r.Get("/jobs/{jobID}", s.handleGetSyncJob)
			r.Post("/jobs/{jobID}/start", s.handleStartSyncJob)
			r.Post("/jobs/{jobID}/cancel", s.handleCancelSyncJob)
		})

		r.Route("/nodes", func(r chi.Router) {
			r.Get("/", s.handleListNodes)
			r.Post("/", s.handleCreateNode)
			r.Get("/{nodeID}", s.handleGetNode)
			r.Put("/{nodeID}/status", s.handleUpdateNodeStatus)
		})

		r.Route("/conflicts", func(r chi.Router) {
			r.Get("/", s.handleListConflicts)
			r.Post("/{conflictID}/resolve", s.handleResolveConflict)
		})
	})

	s.router.Route("/internal", func(r chi.Router) {
		r.Use(s.internalAuthMiddleware)
		r.Route("/sync", func(r chi.Router) {
			r.Post("/coordinate", s.handleSyncCoordination)
			r.Get("/status/{nodeID}", s.handleNodeSyncStatus)
			r.Post("/heartbeat", s.handleNodeHeartbeat)
		})

		r.Route("/events", func(r chi.Router) {
			r.Get("/stream", s.handleEventStream)
			r.Post("/publish", s.handlePublishEvent)
		})

		r.Route("/health", func(r chi.Router) {
			r.Get("/readiness", s.handleReadinessCheck)
			r.Get("/liveness", s.handleLivenessCheck)
		})
	})
}

func (s *Server) Start() error {
	s.SetupMiddleware()
	s.SetupRoutes()

	s.server = &http.Server{
		Addr:         s.getAddress(),
		Handler:      s.router,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, s.config.ShutdownTimeout)
	defer cancel()

	return s.server.Shutdown(shutdownCtx)
}

func (s *Server) getAddress() string {
	return fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
}

func (s *Server) keepAliveMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 1 && r.ProtoMinor >= 1 {
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("Keep-Alive", "timeout=60, max=1000")
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) internalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalToken := r.Header.Get("X-Internal-Token")
		if internalToken != s.getInternalToken() {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) getInternalToken() string {
	return "syncvault-internal-token"
}
