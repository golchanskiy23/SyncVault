package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"syncvault/internal/domain/services"
)

// FileHandler handles HTTP requests for file operations
type FileHandler struct {
	fileService *services.FileService
}

// NewFileHandler creates a new FileHandler
func NewFileHandler(fileService *services.FileService) *FileHandler {
	return &FileHandler{
		fileService: fileService,
	}
}

// CreateFile handles POST /api/v1/files
func (h *FileHandler) CreateFile(w http.ResponseWriter, r *http.Request) {
	log.Printf("FileHandler: Processing create file request")
	
	// Parse request body
	var req services.CreateFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("FileHandler: JSON decode error: %v", err)
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	
	// Set default user ID (in real app, this would come from auth middleware)
	req.UserID = 1
	
	// Call service layer
	response, err := h.fileService.CreateFile(r.Context(), req)
	if err != nil {
		log.Printf("FileHandler: Service error: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create file: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// ListFiles handles GET /api/v1/files
func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	log.Printf("FileHandler: Processing list files request")
	
	// Parse query parameters
	userID := int64(1) // Default user ID
	limit := 10
	offset := 0
	
	// Call service layer
	response, err := h.fileService.ListFiles(r.Context(), services.ListFilesRequest{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		log.Printf("FileHandler: Service error: %v", err)
		http.Error(w, fmt.Sprintf("Failed to list files: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GetFile handles GET /api/v1/files/{fileID}
func (h *FileHandler) GetFile(w http.ResponseWriter, r *http.Request) {
	log.Printf("FileHandler: Processing get file request")
	
	// Get file ID from URL
	fileIDStr := chi.URLParam(r, "fileID")
	fileID := int64(0)
	fmt.Sscanf(fileIDStr, "%d", &fileID)
	
	if fileID == 0 {
		http.Error(w, "Invalid file ID", http.StatusBadRequest)
		return
	}
	
	userID := int64(1) // Default user ID
	
	// Call service layer
	file, err := h.fileService.GetFile(r.Context(), userID, fileID)
	if err != nil {
		log.Printf("FileHandler: Service error: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get file: %v", err), http.StatusNotFound)
		return
	}
	
	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(file)
}

// DeleteFile handles DELETE /api/v1/files/{fileID}
func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	log.Printf("FileHandler: Processing delete file request")
	
	// Get file ID from URL
	fileIDStr := chi.URLParam(r, "fileID")
	fileID := int64(0)
	fmt.Sscanf(fileIDStr, "%d", &fileID)
	
	if fileID == 0 {
		http.Error(w, "Invalid file ID", http.StatusBadRequest)
		return
	}
	
	userID := int64(1) // Default user ID
	
	// Call service layer
	err := h.fileService.DeleteFile(r.Context(), userID, fileID)
	if err != nil {
		log.Printf("FileHandler: Service error: %v", err)
		http.Error(w, fmt.Sprintf("Failed to delete file: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "File deleted successfully",
	})
}
