package http

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "syncvault",
	})
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "pong",
	})
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode([]map[string]interface{}{
		{
			"id":   "file1",
			"path": "/documents/report.pdf",
			"size": 1024,
		},
		{
			"id":   "file2",
			"path": "/images/photo.jpg",
			"size": 2048,
		},
	})
}

func (s *Server) handleCreateFile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":   "new-file-id",
		"path": req.Path,
		"size": req.Size,
	})
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     fileID,
		"path":   "/documents/report.pdf",
		"size":   1024,
		"status": "synced",
	})
}

func (s *Server) handleUpdateFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")

	var req struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":   fileID,
		"path": req.Path,
		"size": req.Size,
	})
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "fileID")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "File deleted successfully",
		"id":      fileID,
	})
}

func (s *Server) handleCreateSyncJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceNode string   `json:"sourceNode"`
		TargetNode string   `json:"targetNode"`
		FileIDs    []string `json:"fileIDs"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         "new-sync-job-id",
		"sourceNode": req.SourceNode,
		"targetNode": req.TargetNode,
		"fileIDs":    req.FileIDs,
		"status":     "pending",
		"createdAt":  "2026-03-15T14:57:00Z",
	})
}

func (s *Server) handleGetSyncJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         jobID,
		"sourceNode": "node1",
		"targetNode": "node2",
		"status":     "running",
		"progress":   50,
		"total":      100,
		"createdAt":  "2026-03-15T14:57:00Z",
		"startedAt":  "2026-03-15T14:58:00Z",
	})
}

func (s *Server) handleStartSyncJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Sync job started",
		"id":      jobID,
		"status":  "running",
	})
}

func (s *Server) handleCancelSyncJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Sync job cancelled",
		"id":      jobID,
		"status":  "cancelled",
	})
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode([]map[string]interface{}{
		{
			"id":     "node1",
			"name":   "Local Storage",
			"type":   "local",
			"status": "online",
		},
		{
			"id":     "node2",
			"name":   "Cloud Storage",
			"type":   "cloud",
			"status": "online",
		},
	})
}

func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		Endpoint string `json:"endpoint"`
		Capacity int64  `json:"capacity"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       "new-node-id",
		"name":     req.Name,
		"type":     req.Type,
		"endpoint": req.Endpoint,
		"capacity": req.Capacity,
		"status":   "offline",
	})
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "nodeID")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":        nodeID,
		"name":      "Local Storage",
		"type":      "local",
		"status":    "online",
		"capacity":  1000000000,
		"usedSpace": 500000000,
	})
}

func (s *Server) handleUpdateNodeStatus(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "nodeID")

	var req struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Node status updated",
		"id":      nodeID,
		"status":  req.Status,
	})
}

func (s *Server) handleListConflicts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode([]map[string]interface{}{
		{
			"id":           "conflict1",
			"fileID":       "file1",
			"sourceNode":   "node1",
			"targetNode":   "node2",
			"conflictType": "content",
			"description":  "Both files modified",
			"resolved":     false,
		},
	})
}

func (s *Server) handleResolveConflict(w http.ResponseWriter, r *http.Request) {
	conflictID := chi.URLParam(r, "conflictID")

	var req struct {
		Resolution string `json:"resolution"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "Conflict resolved",
		"id":         conflictID,
		"resolution": req.Resolution,
		"resolved":   true,
	})
}

func (s *Server) handleSyncCoordination(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Sync coordination completed",
		"status":  "success",
	})
}

func (s *Server) handleNodeSyncStatus(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "nodeID")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodeID":       nodeID,
		"syncStatus":   "in_progress",
		"lastSync":     "2026-03-15T14:50:00Z",
		"pendingFiles": 5,
		"failedFiles":  1,
	})
}

func (s *Server) handleNodeHeartbeat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Heartbeat received",
		"timestamp": "2026-03-15T14:57:00Z",
		"status":    "alive",
	})
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	event := "data: {\"type\":\"file_synced\",\"fileID\":\"file1\",\"timestamp\":\"2026-03-15T14:57:00Z\"}\n\n"
	w.Write([]byte(event))
	flusher.Flush()
}

func (s *Server) handlePublishEvent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type      string                 `json:"type"`
		Data      map[string]interface{} `json:"data"`
		Timestamp string                 `json:"timestamp"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Event published",
		"type":    req.Type,
		"status":  "success",
	})
}

func (s *Server) handleReadinessCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ready",
		"checks": map[string]string{
			"database": "ok",
			"storage":  "ok",
			"cache":    "ok",
		},
	})
}

func (s *Server) handleLivenessCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "alive",
		"timestamp": "2026-03-15T14:57:00Z",
		"uptime":    "2h30m15s",
	})
}
