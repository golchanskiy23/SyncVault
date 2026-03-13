package agent

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	internalsync "syncvault/internal/sync"
)

// HTTPAgent — HTTP сервер на машине пользователя.
// Предоставляет файловую систему через REST API.
// Middleware сервер использует этот API для синхронизации.
type HTTPAgent struct {
	nodeID     string
	rootPath   string
	port       string
	serverAddr string // адрес middleware сервера для регистрации
	node       *internalsync.SimpleStorage
}

func NewHTTPAgent(nodeID, rootPath, port, serverAddr string) *HTTPAgent {
	return &HTTPAgent{
		nodeID:     nodeID,
		rootPath:   rootPath,
		port:       port,
		serverAddr: serverAddr,
		node:       internalsync.NewSimpleStorage(nodeID, rootPath),
	}
}

// Run запускает агент: регистрируется на сервере и слушает HTTP запросы
func (a *HTTPAgent) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Регистрируемся на middleware сервере
	if err := a.register(ctx); err != nil {
		log.Printf("Warning: failed to register with server %s: %v", a.serverAddr, err)
		log.Printf("Agent will run in standalone mode")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /agent/files", a.handleListFiles)
	mux.HandleFunc("GET /agent/files/download", a.handleDownload)
	mux.HandleFunc("POST /agent/files/upload", a.handleUpload)
	mux.HandleFunc("DELETE /agent/files/delete", a.handleDelete)
	mux.HandleFunc("POST /agent/mkdir", a.handleMkDir)
	mux.HandleFunc("GET /agent/health", a.handleHealth)

	srv := &http.Server{Addr: ":" + a.port, Handler: mux}

	go a.heartbeatLoop(ctx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		cancel()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		srv.Shutdown(shutCtx)
	}()

	log.Printf("Agent %s listening on :%s, root=%s", a.nodeID, a.port, a.rootPath)
	return srv.ListenAndServe()
}

func (a *HTTPAgent) handleListFiles(w http.ResponseWriter, r *http.Request) {
	entries, err := a.node.ListFiles(r.Context())
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

	f, err := a.node.ReadFile(r.Context(), path)
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

	if err := a.node.WriteFile(r.Context(), path, r.Body); err != nil {
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

	if err := a.node.DeleteFile(r.Context(), path); err != nil {
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

	if err := a.node.MkDir(r.Context(), path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

func (a *HTTPAgent) register(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{
		"id":        a.nodeID,
		"root_path": a.rootPath,
		"endpoint":  "http://AUTO_DETECT:" + a.port, // middleware заменит на реальный IP
		"type":      "remote_simple",
	})
	return doPost(ctx, "http://"+a.serverAddr+"/sync/nodes/remote", bytesReader(body))
}

func (a *HTTPAgent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			body, _ := json.Marshal(map[string]string{"node_id": a.nodeID})
			doPost(ctx, "http://"+a.serverAddr+"/sync/nodes/heartbeat", bytesReader(body))
		}
	}
}

// --- HTTP helpers ---

func doGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func doPost(ctx context.Context, rawURL string, body io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, "POST", rawURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func doDelete(ctx context.Context, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func doUpload(ctx context.Context, rawURL string, r io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, "POST", rawURL, r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func decodeJSON(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

func urlEncode(s string) string {
	return url.QueryEscape(s)
}
