package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"syncvault/internal/auth"
)

// agentOfflineAfter — через сколько без heartbeat агент считается офлайн
const agentOfflineAfter = 2 * heartbeatTimeout

// heartbeatTimeout совпадает с интервалом heartbeat агента (cmd/agent)
const heartbeatTimeout = 30 * time.Second

// DriveNodeFactory создаёт Node для Google Drive аккаунта.
// Реализуется в cmd/oauth-service/main.go чтобы избежать циклических зависимостей.
type DriveNodeFactory interface {
	NewDriveNode(info NodeInfo) (Node, error)
	// CheckDriveNode проверяет, что для аккаунта есть рабочий токен и папка существует
	CheckDriveNode(ctx context.Context, info NodeInfo) error
}

// NodeStore сохраняет зарегистрированные устройства (агенты) между перезапусками
type NodeStore interface {
	SaveNode(ctx context.Context, info NodeInfo) error
	DeleteNode(ctx context.Context, id string) error
	LoadNodes(ctx context.Context) ([]NodeInfo, error)
}

// SyncHTTPHandlers — HTTP API устройств и связок
type SyncHTTPHandlers struct {
	devices    *NodeRegistry
	pairs      *PairManager
	nodeStore  NodeStore // может быть nil — тогда устройства живут только в памяти
	agentToken string    // общий секрет агентов; "" — агенты не могут подключаться
}

func NewSyncHTTPHandlers(devices *NodeRegistry, pairs *PairManager, agentToken string) *SyncHTTPHandlers {
	return &SyncHTTPHandlers{devices: devices, pairs: pairs, agentToken: agentToken}
}

// SetNodeStore включает сохранение устройств
func (h *SyncHTTPHandlers) SetNodeStore(s NodeStore) {
	h.nodeStore = s
}

// RestoreDevices регистрирует агентов, сохранённых в NodeStore
func (h *SyncHTTPHandlers) RestoreDevices(ctx context.Context) error {
	if h.nodeStore == nil {
		return nil
	}
	infos, err := h.nodeStore.LoadNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to load devices: %w", err)
	}
	for _, info := range infos {
		if info.Type != "remote_simple" {
			continue
		}
		h.devices.Register(info, NewHTTPAgentNode(info.ID, info.Endpoint, h.agentToken))
	}
	log.Printf("Restored %d devices", len(infos))
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// =============================================================================
// Устройства
// =============================================================================

type deviceView struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"` // local | agent
	Root     string     `json:"root"`
	Endpoint string     `json:"endpoint,omitempty"`
	Online   bool       `json:"online"`
	LastSeen *time.Time `json:"last_seen,omitempty"`
}

// ListDevices возвращает этот компьютер (local) и подключённых агентов
// GET /sync/devices
func (h *SyncHTTPHandlers) ListDevices(w http.ResponseWriter, r *http.Request) {
	devices := []deviceView{}
	if root := h.pairs.LocalRoot(); root != "" {
		devices = append(devices, deviceView{ID: LocalDevice, Type: "local", Root: root, Online: true})
	}
	for _, info := range h.devices.List() {
		lastSeen := info.LastSeen
		devices = append(devices, deviceView{
			ID:       info.ID,
			Type:     "agent",
			Root:     info.Root,
			Endpoint: info.Endpoint,
			Online:   time.Since(lastSeen) < agentOfflineAfter,
			LastSeen: &lastSeen,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

// ListDeviceDirs возвращает подпапки папки на устройстве — чтобы выбрать папку для связки
// GET /sync/devices/{id}/dirs?path=projects
func (h *SyncHTTPHandlers) ListDeviceDirs(w http.ResponseWriter, r *http.Request) {
	device := r.PathValue("id")
	path := r.URL.Query().Get("path")

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	dirs, err := h.pairs.ListDirs(ctx, device, path)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device": device, "path": path, "dirs": dirs})
}

// DeleteDevice забывает агента; устройство, используемое в связках, удалить нельзя
// DELETE /sync/devices/{id}
func (h *SyncHTTPHandlers) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := h.devices.Info(id); !ok {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	used, err := h.pairs.PairsUsingDevice(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(used) > 0 {
		http.Error(w, fmt.Sprintf("device is used by pairs %v — delete them first", used), http.StatusConflict)
		return
	}
	if h.nodeStore != nil {
		if err := h.nodeStore.DeleteNode(r.Context(), id); err != nil {
			http.Error(w, "failed to delete device: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	h.devices.Unregister(id)
	writeJSON(w, http.StatusOK, map[string]any{"message": "device deleted", "id": id})
}

// =============================================================================
// Связки
// =============================================================================

// CreatePair создаёт связку двух папок
// POST /sync/pairs
//
//	{"name": "docs",
//	 "a": {"device": "local", "path": "/home/bloom/test-sync/docs"},
//	 "b": {"device": "home-server", "path": "projects/docs"}}
//
// Сторона на Google Drive: {"drive": "user@gmail.com", "folder_id": "<ID папки>"}
func (h *SyncHTTPHandlers) CreatePair(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string   `json:"name"`
		A    PairSide `json:"a"`
		B    PairSide `json:"b"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	for _, side := range []*PairSide{&req.A, &req.B} {
		if side.Drive == "" {
			side.UserID = ""
			continue
		}
		// Токен Drive берётся только у текущего пользователя — чужой Drive подключить нельзя
		if side.UserID != "" && side.UserID != userID {
			http.Error(w, "user_id does not match authenticated user", http.StatusForbidden)
			return
		}
		side.UserID = userID
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	pair := &Pair{Name: req.Name, A: req.A, B: req.B}
	if err := h.pairs.Create(ctx, pair); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("Pair created: %s", pair.label())
	writeJSON(w, http.StatusCreated, pair)
}

// ListPairs возвращает связки со статусом последней синхронизации
// GET /sync/pairs
func (h *SyncHTTPHandlers) ListPairs(w http.ResponseWriter, r *http.Request) {
	pairs, err := h.pairs.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if pairs == nil {
		pairs = []Pair{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pairs": pairs})
}

func pairID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// DeletePair удаляет связку (файлы в папках остаются)
// DELETE /sync/pairs/{id}
func (h *SyncHTTPHandlers) DeletePair(w http.ResponseWriter, r *http.Request) {
	id, err := pairID(r)
	if err != nil {
		http.Error(w, "invalid pair id", http.StatusBadRequest)
		return
	}
	if err := h.pairs.Delete(r.Context(), id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrPairNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "pair deleted", "id": id})
}

// RunPair запускает синхронизацию связки сейчас (в фоне)
// POST /sync/pairs/{id}/run
func (h *SyncHTTPHandlers) RunPair(w http.ResponseWriter, r *http.Request) {
	id, err := pairID(r)
	if err != nil {
		http.Error(w, "invalid pair id", http.StatusBadRequest)
		return
	}
	pair, err := h.pairs.Get(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrPairNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	// r.Context() отменяется сразу после ответа клиенту — синхронизация идёт в фоне
	go h.pairs.Run(context.Background(), *pair)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "sync started, see GET /sync/pairs for the result",
		"id":      id,
	})
}

// RunAllPairs запускает синхронизацию всех связок сейчас (в фоне)
// POST /sync/pairs/run
func (h *SyncHTTPHandlers) RunAllPairs(w http.ResponseWriter, r *http.Request) {
	go h.pairs.RunAll(context.Background())
	writeJSON(w, http.StatusAccepted, map[string]any{"message": "sync of all pairs started"})
}

// =============================================================================
// Агенты (авторизация общим токеном sync.agentToken)
// =============================================================================

// AgentAuth пропускает только запросы агентов с верным токеном
func (h *SyncHTTPHandlers) AgentAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.agentToken == "" {
			http.Error(w, "agents are disabled: sync.agentToken is not configured", http.StatusServiceUnavailable)
			return
		}
		if !ValidAgentToken(h.agentToken, r.Header.Get(AgentTokenHeader)) {
			http.Error(w, "invalid agent token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RegisterAgent — агент регистрирует себя сам
// POST /sync/agents/register
// {"id": "home-server", "port": "9100", "root": "/home/user"} или с "endpoint": "http://100.64.0.2:9100"
func (h *SyncHTTPHandlers) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		Port     string `json:"port"`
		Endpoint string `json:"endpoint"`
		Root     string `json:"root"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" || (req.Port == "" && req.Endpoint == "") {
		http.Error(w, "id and port (or endpoint) are required", http.StatusBadRequest)
		return
	}
	if req.ID == LocalDevice {
		http.Error(w, `device id "local" is reserved for this computer`, http.StatusConflict)
		return
	}

	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint = "http://" + agentAutoDetectHost + ":" + req.Port
	}
	endpoint = resolveAgentEndpoint(endpoint, r.RemoteAddr)

	info := NodeInfo{ID: req.ID, Type: "remote_simple", Endpoint: endpoint, Root: req.Root}
	if h.nodeStore != nil {
		if err := h.nodeStore.SaveNode(r.Context(), info); err != nil {
			http.Error(w, "failed to save device: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	h.devices.Register(info, NewHTTPAgentNode(req.ID, endpoint, h.agentToken))
	log.Printf("Agent registered: %s @ %s (root %s)", req.ID, endpoint, req.Root)

	writeJSON(w, http.StatusOK, map[string]any{"message": "agent registered", "id": req.ID, "endpoint": endpoint})
}

// AgentHeartbeat — агент сообщает, что он онлайн; 404 — сервер не знает устройство,
// агенту нужно зарегистрироваться заново
// POST /sync/agents/heartbeat
// {"node_id": "home-server"}
func (h *SyncHTTPHandlers) AgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeID == "" {
		http.Error(w, "node_id required", http.StatusBadRequest)
		return
	}
	if _, ok := h.devices.Info(req.NodeID); !ok {
		http.Error(w, "unknown device", http.StatusNotFound)
		return
	}
	h.devices.Heartbeat(req.NodeID)
	writeJSON(w, http.StatusOK, map[string]any{"node_id": req.NodeID, "ts": time.Now()})
}

// agentAutoDetectHost — хост, который агент подставляет, когда не знает свой адрес
const agentAutoDetectHost = "AUTO_DETECT"

// resolveAgentEndpoint заменяет AUTO_DETECT на IP, с которого пришёл запрос агента
func resolveAgentEndpoint(endpoint, remoteAddr string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() != agentAutoDetectHost {
		return endpoint
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return endpoint
	}
	u.Host = net.JoinHostPort(host, u.Port())
	return u.String()
}
