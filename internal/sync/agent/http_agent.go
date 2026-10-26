package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	internalsync "syncvault/internal/sync"
)

const (
	registerRetryInterval = 30 * time.Second
	heartbeatInterval     = 30 * time.Second
)

// HTTPAgent — HTTP сервер на машине пользователя.
// Предоставляет файловую систему через REST API.
// Middleware сервер использует этот API для синхронизации.
type HTTPAgent struct {
	nodeID    string
	rootPath  string // базовая папка: сервер работает только с подпапками внутри неё
	port      string
	serverURL string // адрес middleware сервера для регистрации (http://host:port)
	advertise string // адрес агента для сервера; "" — сервер определит IP сам
	token     string // общий секрет агента и сервера
	client    *http.Client
}

// NewHTTPAgent создаёт агент. serverAddr — host:port или URL сервера,
// advertise — URL агента, по которому к нему ходит сервер (например, адрес в Tailscale).
func NewHTTPAgent(nodeID, rootPath, port, serverAddr, advertise, token string) *HTTPAgent {
	serverURL := strings.TrimRight(serverAddr, "/")
	if !strings.Contains(serverURL, "://") {
		serverURL = "http://" + serverURL
	}
	return &HTTPAgent{
		nodeID:    nodeID,
		rootPath:  rootPath,
		port:      port,
		serverURL: serverURL,
		advertise: advertise,
		token:     token,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Run запускает агент: слушает HTTP запросы и регистрируется на сервере
func (a *HTTPAgent) Run() error {
	if fi, err := os.Stat(a.rootPath); err != nil || !fi.IsDir() {
		return fmt.Errorf("root %s is not an existing directory", a.rootPath)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := &http.Server{Addr: ":" + a.port, Handler: a.Handler()}

	go a.serverLoop(ctx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		cancel()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		srv.Shutdown(shutCtx)
	}()

	log.Printf("Agent %s listening on :%s, root=%s, server=%s", a.nodeID, a.port, a.rootPath, a.serverURL)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Handler возвращает HTTP API агента (с проверкой токена)
func (a *HTTPAgent) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /agent/files", a.handleListFiles)
	mux.HandleFunc("GET /agent/files/download", a.handleDownload)
	mux.HandleFunc("POST /agent/files/upload", a.handleUpload)
	mux.HandleFunc("DELETE /agent/files/delete", a.handleDelete)
	mux.HandleFunc("POST /agent/mkdir", a.handleMkDir)
	mux.HandleFunc("GET /agent/dirs", a.handleDirs)
	mux.HandleFunc("GET /agent/health", a.handleHealth)
	return a.authMiddleware(mux)
}

// authMiddleware пропускает только запросы с верным токеном агента
func (a *HTTPAgent) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !internalsync.ValidAgentToken(a.token, r.Header.Get(internalsync.AgentTokenHeader)) {
			http.Error(w, "invalid agent token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// storage возвращает хранилище для подпапки ?root= внутри базовой папки агента
func (a *HTTPAgent) storage(r *http.Request) (*internalsync.SimpleStorage, error) {
	root, err := internalsync.ResolveInside(a.rootPath, r.URL.Query().Get("root"))
	if err != nil {
		return nil, err
	}
	return internalsync.NewSimpleStorage(a.nodeID, root), nil
}

func (a *HTTPAgent) handleListFiles(w http.ResponseWriter, r *http.Request) {
	st, err := a.storage(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entries, err := st.ListFiles(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"files": entries})
}

func (a *HTTPAgent) handleDownload(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	st, err := a.storage(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f, err := st.ReadFile(r.Context(), path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, f)
}

func (a *HTTPAgent) handleUpload(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	st, err := a.storage(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := st.WriteFile(r.Context(), path, r.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "path": path})
}

func (a *HTTPAgent) handleDelete(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	st, err := a.storage(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := st.DeleteFile(r.Context(), path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (a *HTTPAgent) handleMkDir(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}
	st, err := a.storage(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := st.MkDir(r.Context(), path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleDirs возвращает подпапки папки ?path= (относительно базовой); 404 — такой папки нет
func (a *HTTPAgent) handleDirs(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	dirs, err := internalsync.ListSubdirs(a.rootPath, rel)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"root": a.rootPath, "path": rel, "dirs": dirs})
}

func (a *HTTPAgent) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"node_id":   a.nodeID,
		"root_path": a.rootPath,
		"status":    "healthy",
		"timestamp": time.Now(),
	})
}

// serverLoop регистрирует агент на сервере (с повторами, пока сервер недоступен)
// и затем шлёт heartbeat; если сервер забыл узел — регистрируется заново
func (a *HTTPAgent) serverLoop(ctx context.Context) {
	registered := false
	for {
		if !registered {
			if err := a.register(ctx); err != nil {
				log.Printf("Agent %s: registration on %s failed: %v (retry in %s)", a.nodeID, a.serverURL, err, registerRetryInterval)
			} else {
				log.Printf("Agent %s registered on %s", a.nodeID, a.serverURL)
				registered = true
			}
		} else {
			status, err := a.post(ctx, "/sync/agents/heartbeat", map[string]string{"node_id": a.nodeID})
			switch {
			case err != nil:
				log.Printf("Agent %s: heartbeat failed: %v", a.nodeID, err)
			case status == http.StatusNotFound:
				log.Printf("Agent %s: server does not know this node, registering again", a.nodeID)
				registered = false
				continue
			}
		}

		wait := heartbeatInterval
		if !registered {
			wait = registerRetryInterval
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (a *HTTPAgent) register(ctx context.Context) error {
	body := map[string]string{"id": a.nodeID, "port": a.port, "root": a.rootPath}
	if a.advertise != "" {
		body["endpoint"] = a.advertise
	}
	status, err := a.post(ctx, "/sync/agents/register", body)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("server responded %d", status)
	}
	return nil
}

// post отправляет JSON на сервер с токеном агента и возвращает HTTP статус
func (a *HTTPAgent) post(ctx context.Context, path string, body any) (int, error) {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.serverURL+path, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(internalsync.AgentTokenHeader, a.token)
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
