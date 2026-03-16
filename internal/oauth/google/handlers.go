package google

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"syncvault/internal/auth"
	"syncvault/internal/config"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OAuthHandlers обрабатывает HTTP запросы для OAuth flow
type OAuthHandlers struct {
	oauthService *OAuthService
	driveAdapter *DriveAdapter
	jwtService   *auth.JWTService
}

// NewOAuthHandlers создает новые OAuth handlers
func NewOAuthHandlers(db *pgxpool.Pool, config *config.GoogleDriveConfig, jwtService *auth.JWTService) *OAuthHandlers {
	oauthService := NewOAuthService(config, db)
	driveAdapter := NewDriveAdapter(config, oauthService)

	return &OAuthHandlers{
		oauthService: oauthService,
		driveAdapter: driveAdapter,
		jwtService:   jwtService,
	}
}

// AuthRequest обрабатывает запрос на авторизацию
func (h *OAuthHandlers) AuthRequest(w http.ResponseWriter, r *http.Request) {
	// Получаем пользователя из JWT токена или query параметра
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		// Fallback: принимаем user_id из query для начала OAuth flow
		userID = r.URL.Query().Get("user_id")
		if userID == "" {
			http.Error(w, "Unauthorized: provide JWT or user_id query param", http.StatusUnauthorized)
			return
		}
	}

	// Генерируем auth URL
	authURL, pkce, err := h.oauthService.GetAuthURL(r.Context(), userID)
	if err != nil {
		log.Printf("Failed to generate auth URL: %v", err)
		http.Error(w, "Failed to generate auth URL", http.StatusInternalServerError)
		return
	}

	// Сохраняем PKCE данные в сессии или cookie (в данном случае в cookie)
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    pkce.State,
		Path:     "/",
		MaxAge:   600, // 10 минут
		HttpOnly: true,
		Secure:   false, // localhost — без HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_verifier",
		Value:    pkce.CodeVerifier,
		Path:     "/",
		MaxAge:   600, // 10 минут
		HttpOnly: true,
		Secure:   false, // localhost — без HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	// Перенаправляем на Google OAuth
	http.Redirect(w, r, authURL, http.StatusFound)
}

// AuthCallback обрабатывает OAuth callback от Google
func (h *OAuthHandlers) AuthCallback(w http.ResponseWriter, r *http.Request) {
	// Получаем параметры из query
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		log.Printf("OAuth error: %s", errorParam)
		http.Error(w, fmt.Sprintf("OAuth error: %s", errorParam), http.StatusBadRequest)
		return
	}

	if code == "" || state == "" {
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	// Получаем сохраненный state из cookie
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	// Проверяем state
	if stateCookie.Value != state {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// Получаем пользователя из JWT токена (если доступен)
	var userID string
	if jwtUserID, ok := auth.GetUserIDFromContext(r.Context()); ok {
		userID = jwtUserID
	} else {
		// Если нет JWT, пытаемся получить из OAuth state
		oauthState, err := h.oauthService.GetState(r.Context(), state)
		if err != nil {
			http.Error(w, "Failed to get OAuth state", http.StatusInternalServerError)
			return
		}
		userID = oauthState.UserID
	}

	// Обмениваем code на токен
	token, err := h.oauthService.ExchangeCode(r.Context(), state, code)
	if err != nil {
		log.Printf("Failed to exchange code: %v", err)
		http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
		return
	}

	// Сохраняем токен
	err = h.oauthService.SaveToken(r.Context(), userID, token)
	if err != nil {
		log.Printf("Failed to save token: %v", err)
		http.Error(w, "Failed to save token", http.StatusInternalServerError)
		return
	}

	// Очищаем cookies
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_verifier",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	// Перенаправляем на фронтенд с успехом
	redirectURL := fmt.Sprintf("/auth/success?provider=google&user_id=%s", url.QueryEscape(userID))
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// ListFiles обрабатывает запрос на список файлов
func (h *OAuthHandlers) ListFiles(w http.ResponseWriter, r *http.Request) {
	// Получаем пользователя из JWT токена
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Получаем параметры запроса
	folderID := r.URL.Query().Get("folder_id")
	pageSizeStr := r.URL.Query().Get("page_size")

	pageSize := int64(100) // по умолчанию
	if pageSizeStr != "" {
		if ps, err := strconv.ParseInt(pageSizeStr, 10, 64); err == nil && ps > 0 && ps <= 1000 {
			pageSize = ps
		}
	}

	// Получаем список файлов
	result, err := h.driveAdapter.ListFiles(r.Context(), userID, folderID, pageSize)
	if err != nil {
		log.Printf("Failed to list files: %v", err)
		http.Error(w, "Failed to list files", http.StatusInternalServerError)
		return
	}

	// Отправляем ответ
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GetFile обрабатывает запрос на получение информации о файле
func (h *OAuthHandlers) GetFile(w http.ResponseWriter, r *http.Request) {
	// Получаем пользователя из JWT токена
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Получаем file ID из URL
	fileID := chi.URLParam(r, "fileID")
	if fileID == "" {
		http.Error(w, "File ID is required", http.StatusBadRequest)
		return
	}

	// Получаем информацию о файле
	file, err := h.driveAdapter.GetFile(r.Context(), userID, fileID)
	if err != nil {
		log.Printf("Failed to get file: %v", err)
		http.Error(w, "Failed to get file", http.StatusInternalServerError)
		return
	}

	// Отправляем ответ
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(file)
}

// DownloadFile обрабатывает запрос на скачивание файла
func (h *OAuthHandlers) DownloadFile(w http.ResponseWriter, r *http.Request) {
	// Получаем пользователя из JWT токена
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Получаем file ID из URL
	fileID := chi.URLParam(r, "fileID")
	if fileID == "" {
		http.Error(w, "File ID is required", http.StatusBadRequest)
		return
	}

	// Получаем информацию о файле для определения имени
	file, err := h.driveAdapter.GetFile(r.Context(), userID, fileID)
	if err != nil {
		log.Printf("Failed to get file info: %v", err)
		http.Error(w, "Failed to get file info", http.StatusInternalServerError)
		return
	}

	// Устанавливаем заголовки для скачивания
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.Name))
	w.Header().Set("Content-Type", "application/octet-stream")

	// Получаем download URL
	downloadURL, err := h.driveAdapter.GetDownloadURL(r.Context(), userID, fileID)
	if err != nil {
		log.Printf("Failed to get download URL: %v", err)
		http.Error(w, "Failed to get download URL", http.StatusInternalServerError)
		return
	}

	// Перенаправляем на download URL
	http.Redirect(w, r, downloadURL, http.StatusFound)
}

// SearchFiles обрабатывает запрос на поиск файлов
func (h *OAuthHandlers) SearchFiles(w http.ResponseWriter, r *http.Request) {
	// Получаем пользователя из JWT токена
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Получаем параметры запроса
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Search query is required", http.StatusBadRequest)
		return
	}

	pageSizeStr := r.URL.Query().Get("page_size")
	pageSize := int64(50) // по умолчанию
	if pageSizeStr != "" {
		if ps, err := strconv.ParseInt(pageSizeStr, 10, 64); err == nil && ps > 0 && ps <= 1000 {
			pageSize = ps
		}
	}

	// Ищем файлы
	result, err := h.driveAdapter.SearchFiles(r.Context(), userID, query, pageSize)
	if err != nil {
		log.Printf("Failed to search files: %v", err)
		http.Error(w, "Failed to search files", http.StatusInternalServerError)
		return
	}

	// Отправляем ответ
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// CreateFolder обрабатывает запрос на создание папки
func (h *OAuthHandlers) CreateFolder(w http.ResponseWriter, r *http.Request) {
	// Получаем пользователя из JWT токена
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Парсим запрос
	var req struct {
		Name           string `json:"name"`
		ParentFolderID string `json:"parent_folder_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Folder name is required", http.StatusBadRequest)
		return
	}

	// Создаем папку
	folder, err := h.driveAdapter.CreateFolder(r.Context(), userID, req.Name, req.ParentFolderID)
	if err != nil {
		log.Printf("Failed to create folder: %v", err)
		http.Error(w, "Failed to create folder", http.StatusInternalServerError)
		return
	}

	// Отправляем ответ
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folder)
}

// syncDownloadRecursive рекурсивно скачивает папку с Drive
func (h *OAuthHandlers) syncDownloadRecursive(ctx context.Context, userID, folderID, localPath string) {
	if err := os.MkdirAll(localPath, 0755); err != nil {
		log.Printf("SyncDownload: failed to create dir %s: %v", localPath, err)
		return
	}

	result, err := h.driveAdapter.ListFiles(ctx, userID, folderID, 1000)
	if err != nil {
		log.Printf("SyncDownload: failed to list folder %s: %v", folderID, err)
		return
	}

	for _, file := range result.Files {
		if file.MimeType == "application/vnd.google-apps.folder" {
			// Рекурсивно обходим подпапку
			h.syncDownloadRecursive(ctx, userID, file.ID, localPath+"/"+file.Name)
		} else {
			dest := localPath + "/" + file.Name
			if err := h.driveAdapter.DownloadFile(ctx, userID, file.ID, dest); err != nil {
				log.Printf("SyncDownload: failed to download %s: %v", file.Name, err)
			} else {
				log.Printf("SyncDownload: ✓ %s → %s", file.Name, dest)
			}
		}
	}
}

// SyncDownload скачивает папку с Google Drive на локальный путь (рекурсивно)
func (h *OAuthHandlers) SyncDownload(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		FolderID  string `json:"folder_id"`
		LocalPath string `json:"local_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FolderID == "" || req.LocalPath == "" {
		http.Error(w, "folder_id and local_path are required", http.StatusBadRequest)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		h.syncDownloadRecursive(ctx, userID, req.FolderID, req.LocalPath)
		log.Printf("SyncDownload: completed %s → %s", req.FolderID, req.LocalPath)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "Download sync started",
		"folder_id":  req.FolderID,
		"local_path": req.LocalPath,
		"status":     "in_progress",
	})
}

// syncUploadRecursive рекурсивно загружает локальную папку на Drive
func (h *OAuthHandlers) syncUploadRecursive(ctx context.Context, userID, localPath, folderID string) {
	entries, err := os.ReadDir(localPath)
	if err != nil {
		log.Printf("SyncUpload: failed to read dir %s: %v", localPath, err)
		return
	}

	for _, entry := range entries {
		fullPath := localPath + "/" + entry.Name()
		if entry.IsDir() {
			// Создаём папку на Drive и рекурсируем
			subFolder, err := h.driveAdapter.CreateFolder(ctx, userID, entry.Name(), folderID)
			if err != nil {
				log.Printf("SyncUpload: failed to create folder %s: %v", entry.Name(), err)
				continue
			}
			h.syncUploadRecursive(ctx, userID, fullPath, subFolder.ID)
		} else {
			uploaded, err := h.driveAdapter.UploadFile(ctx, userID, fullPath, folderID)
			if err != nil {
				log.Printf("SyncUpload: failed to upload %s: %v", entry.Name(), err)
			} else {
				log.Printf("SyncUpload: ✓ %s → %s", entry.Name(), uploaded.WebViewLink)
			}
		}
	}
}

// SyncUpload загружает локальную папку на Google Drive (рекурсивно)
func (h *OAuthHandlers) SyncUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		LocalPath string `json:"local_path"`
		FolderID  string `json:"folder_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LocalPath == "" || req.FolderID == "" {
		http.Error(w, "local_path and folder_id are required", http.StatusBadRequest)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		h.syncUploadRecursive(ctx, userID, req.LocalPath, req.FolderID)
		log.Printf("SyncUpload: completed %s → %s", req.LocalPath, req.FolderID)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "Upload sync started",
		"local_path": req.LocalPath,
		"folder_id":  req.FolderID,
		"status":     "in_progress",
	})
}

// SyncFiles обрабатывает запрос на синхронизацию файлов
func (h *OAuthHandlers) SyncFiles(w http.ResponseWriter, r *http.Request) {
	// Получаем пользователя из JWT токена
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Запускаем синхронизацию в фоне
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		err := h.driveAdapter.SyncUserFiles(ctx, userID)
		if err != nil {
			log.Printf("Failed to sync files for user %s: %v", userID, err)
		}
	}()

	// Отправляем ответ о запуске синхронизации
	response := map[string]interface{}{
		"message": "Sync started",
		"user_id": userID,
		"status":  "in_progress",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetSyncStatus обрабатывает запрос на получение статуса синхронизации
func (h *OAuthHandlers) GetSyncStatus(w http.ResponseWriter, r *http.Request) {
	// Получаем пользователя из JWT токена
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Получаем статус синхронизации
	query := `
		SELECT sync_status, error_message, total_files, synced_files, last_sync_at
		FROM google_drive_sync_status
		WHERE user_id = $1
	`

	var status, errorMessage string
	var totalFiles, syncedFiles int
	var lastSyncAt time.Time

	err := h.oauthService.db.QueryRow(r.Context(), query, userID).Scan(
		&status, &errorMessage, &totalFiles, &syncedFiles, &lastSyncAt,
	)

	if err != nil {
		// Если нет записи, возвращаем статус pending
		response := map[string]interface{}{
			"status":       "pending",
			"total_files":  0,
			"synced_files": 0,
			"last_sync_at": nil,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	response := map[string]interface{}{
		"status":        status,
		"error_message": errorMessage,
		"total_files":   totalFiles,
		"synced_files":  syncedFiles,
		"last_sync_at":  lastSyncAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Disconnect обрабатывает запрос на отключение от Google Drive
func (h *OAuthHandlers) Disconnect(w http.ResponseWriter, r *http.Request) {
	// Получаем пользователя из JWT токена
	userID, ok := auth.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Отзываем все токены
	err := h.oauthService.RevokeTokens(r.Context(), userID, ProviderGoogle)
	if err != nil {
		log.Printf("Failed to revoke tokens: %v", err)
		http.Error(w, "Failed to revoke tokens", http.StatusInternalServerError)
		return
	}

	// Очищаем кэш файлов
	_, err = h.oauthService.db.Exec(r.Context(),
		"DELETE FROM google_drive_files WHERE user_id = $1", userID)
	if err != nil {
		log.Printf("Warning: failed to clear file cache: %v", err)
	}

	// Очищаем статус синхронизации
	_, err = h.oauthService.db.Exec(r.Context(),
		"DELETE FROM google_drive_sync_status WHERE user_id = $1", userID)
	if err != nil {
		log.Printf("Warning: failed to clear sync status: %v", err)
	}

	response := map[string]interface{}{
		"message": "Successfully disconnected from Google Drive",
		"user_id": userID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RegisterRoutes регистрирует OAuth роуты
func (h *OAuthHandlers) RegisterRoutes(router chi.Router) {
	// OAuth flow роуты (без JWT)
	router.Group(func(r chi.Router) {
		r.Get("/auth/google", h.AuthRequest)
		r.Get("/auth/google/callback", h.AuthCallback)
	})

	// Google Drive API роуты (требуют JWT)
	router.Group(func(r chi.Router) {
		r.Use(h.jwtService.AuthMiddleware)

		// Файлы и папки
		r.Get("/drive/files", h.ListFiles)
		r.Get("/drive/files/{fileID}", h.GetFile)
		r.Get("/drive/files/{fileID}/download", h.DownloadFile)
		r.Get("/drive/search", h.SearchFiles)
		r.Post("/drive/folders", h.CreateFolder)

		// Синхронизация
		r.Post("/drive/sync", h.SyncFiles)
		r.Get("/drive/sync/status", h.GetSyncStatus)
		r.Post("/drive/sync/download", h.SyncDownload)
		r.Post("/drive/sync/upload", h.SyncUpload)

		// Управление аккаунтом
		r.Post("/drive/disconnect", h.Disconnect)
	})

	// Success page после OAuth
	router.Get("/auth/success", func(w http.ResponseWriter, r *http.Request) {
		provider := r.URL.Query().Get("provider")
		userID := r.URL.Query().Get("user_id")
		response := map[string]interface{}{
			"message":  "OAuth authorization successful",
			"provider": provider,
			"user_id":  userID,
			"status":   "connected",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Health check для OAuth сервиса
	router.Get("/oauth/health", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now().UTC(),
			"service":   "oauth-service",
			"provider":  "google",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
}
