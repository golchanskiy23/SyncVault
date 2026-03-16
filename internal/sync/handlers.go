package sync

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"syncvault/internal/sync/agent"
)

// DriveNodeFactory создаёт Node для Google Drive аккаунта.
// Реализуется в cmd/oauth-service/main.go чтобы избежать циклических зависимостей.
type DriveNodeFactory interface {
	NewDriveNode(userID, accountID, nodeID string) (Node, error)
}

// SyncHTTPHandlers — HTTP API для управления синхронизацией
type SyncHTTPHandlers struct {
	registry     *NodeRegistry
	engine       *SyncEngine
	driveFactory DriveNodeFactory // может быть nil если drive не нужен
}

func NewSyncHTTPHandlers(registry *NodeRegistry, engine *SyncEngine) *SyncHTTPHandlers {
	return &SyncHTTPHandlers{registry: registry, engine: engine}
}

// SetDriveFactory устанавливает фабрику для создания drive-узлов
func (h *SyncHTTPHandlers) SetDriveFactory(f DriveNodeFactory) {
	h.driveFactory = f
}

// RegisterLocalNode регистрирует локальную папку как узел
// POST /sync/nodes/local
// {"id": "laptop-home", "root_path": "/home/bloom/all/all_files/Резюме"}
func (h *SyncHTTPHandlers) RegisterLocalNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		RootPath string `json:"root_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" || req.RootPath == "" {
		http.Error(w, "id and root_path are required", http.StatusBadRequest)
		return
	}

	if _, err := os.Stat(req.RootPath); err != nil {
		http.Error(w, "root_path does not exist: "+err.Error(), http.StatusBadRequest)
		return
	}

	node := NewSimpleStorage(req.ID, req.RootPath)
	h.registry.Register(NodeInfo{
		ID:       req.ID,
		Type:     "simple",
		Endpoint: req.RootPath,
	}, node)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "simple storage node registered",
		"id":      req.ID,
		"path":    req.RootPath,
	})
}

// ListNodes возвращает все зарегистрированные узлы
// GET /sync/nodes
func (h *SyncHTTPHandlers) ListNodes(w http.ResponseWriter, r *http.Request) {
	nodes := h.registry.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	})
}

// SyncPair синхронизирует два конкретных узла
// POST /sync/run
// {"source_id": "laptop-home", "target_id": "drive-account1"}
func (h *SyncHTTPHandlers) SyncPair(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceID string `json:"source_id"`
		TargetID string `json:"target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SourceID == "" || req.TargetID == "" {
		http.Error(w, "source_id and target_id are required", http.StatusBadRequest)
		return
	}

	source, err := h.registry.Get(req.SourceID)
	if err != nil {
		http.Error(w, "source node not found: "+err.Error(), http.StatusNotFound)
		return
	}
	target, err := h.registry.Get(req.TargetID)
	if err != nil {
		http.Error(w, "target node not found: "+err.Error(), http.StatusNotFound)
		return
	}

	go func() {
		ctx := r.Context()
		result, err := h.engine.Sync(ctx, source, target)
		if err != nil {
			log.Printf("SyncPair error %s↔%s: %v", req.SourceID, req.TargetID, err)
			return
		}
		log.Printf("SyncPair done %s↔%s: transferred=%d conflicts=%d",
			req.SourceID, req.TargetID, result.Transferred, len(result.Conflicts))
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "sync started",
		"source_id": req.SourceID,
		"target_id": req.TargetID,
		"status":    "in_progress",
	})
}

// SyncAll синхронизирует все зарегистрированные узлы между собой
// POST /sync/run/all
func (h *SyncHTTPHandlers) SyncAll(w http.ResponseWriter, r *http.Request) {
	nodes := h.registry.AllNodes()
	if len(nodes) < 2 {
		http.Error(w, "need at least 2 registered nodes", http.StatusBadRequest)
		return
	}

	go func() {
		results, err := h.engine.SyncAll(r.Context(), nodes)
		if err != nil {
			log.Printf("SyncAll error: %v", err)
			return
		}
		total := 0
		for _, r := range results {
			total += r.Transferred
		}
		log.Printf("SyncAll done: %d pairs, %d files transferred", len(results), total)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "full sync started",
		"node_count": len(nodes),
		"pairs":      len(nodes) * (len(nodes) - 1) / 2,
		"status":     "in_progress",
		"started_at": time.Now(),
	})
}

// UnregisterNode удаляет узел из реестра
// DELETE /sync/nodes/{id}
func (h *SyncHTTPHandlers) UnregisterNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "node id required", http.StatusBadRequest)
		return
	}
	h.registry.Unregister(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "node unregistered",
		"id":      id,
	})
}

// RegisterRemoteNode регистрирует удалённую машину (агент сообщает о себе)
// POST /sync/nodes/remote
// {"id": "laptop-b", "endpoint": "http://192.168.1.10:9100"}
func (h *SyncHTTPHandlers) RegisterRemoteNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" || req.Endpoint == "" {
		http.Error(w, "id and endpoint are required", http.StatusBadRequest)
		return
	}

	node := agent.NewHTTPAgentNode(req.ID, req.Endpoint)
	h.registry.Register(NodeInfo{
		ID:       req.ID,
		Type:     "remote_simple",
		Endpoint: req.Endpoint,
	}, node)

	log.Printf("Remote node registered: %s @ %s", req.ID, req.Endpoint)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":  "remote node registered",
		"id":       req.ID,
		"endpoint": req.Endpoint,
	})
}

// RegisterDriveNode регистрирует Google Drive аккаунт как узел синхронизации
// POST /sync/nodes/drive
// {"id": "drive-account1", "account_id": "user@gmail.com", "user_id": "user_123"}
func (h *SyncHTTPHandlers) RegisterDriveNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        string `json:"id"`
		AccountID string `json:"account_id"`
		UserID    string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" || req.AccountID == "" || req.UserID == "" {
		http.Error(w, "id, account_id and user_id are required", http.StatusBadRequest)
		return
	}

	if h.driveFactory == nil {
		http.Error(w, "drive factory not configured", http.StatusInternalServerError)
		return
	}

	node, err := h.driveFactory.NewDriveNode(req.UserID, req.AccountID, req.ID)
	if err != nil {
		http.Error(w, "failed to create drive node: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.registry.Register(NodeInfo{
		ID:        req.ID,
		Type:      "google_drive",
		AccountID: req.AccountID,
	}, node)

	log.Printf("Drive node registered: %s (account=%s)", req.ID, req.AccountID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "drive node registered",
		"id":         req.ID,
		"account_id": req.AccountID,
	})
}

// Heartbeat обновляет статус узла
// POST /sync/nodes/heartbeat
// {"node_id": "laptop-b"}
func (h *SyncHTTPHandlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeID == "" {
		http.Error(w, "node_id required", http.StatusBadRequest)
		return
	}
	h.registry.Heartbeat(req.NodeID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"node_id": req.NodeID,
		"ts":      time.Now(),
	})
}
