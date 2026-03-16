package google

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syncvault/internal/config"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// DriveAdapter предоставляет адаптер для работы с Google Drive
type DriveAdapter struct {
	config       *GoogleDriveConfig
	oauthService *OAuthService
}

// NewDriveAdapter создает новый Drive адаптер
func NewDriveAdapter(config *config.GoogleDriveConfig, oauthService *OAuthService) *DriveAdapter {
	// Конвертируем config
	googleConfig := &GoogleDriveConfig{
		OAuth: &GoogleOAuthConfig{
			ClientID:     config.OAuth.ClientID,
			ClientSecret: config.OAuth.ClientSecret,
			RedirectURL:  config.OAuth.RedirectURL,
			Scopes:       config.OAuth.Scopes,
		},
		APIBaseURL:  config.APIBaseURL,
		UploadURL:   config.UploadURL,
		MaxFileSize: config.MaxFileSize,
		ChunkSize:   config.ChunkSize,
		RetryCount:  config.RetryCount,
		RetryDelay:  config.RetryDelay,
	}

	return &DriveAdapter{
		config:       googleConfig,
		oauthService: oauthService,
	}
}

// getHTTPClient получает HTTP клиент с валидным токеном
func (d *DriveAdapter) getHTTPClient(ctx context.Context, userID string) (*http.Client, error) {
	// Получаем валидный токен с автоматическим обновлением
	token, err := d.oauthService.GetValidToken(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get valid token: %w", err)
	}

	// Создаем HTTP клиент с токеном
	client := d.config.OAuth.ToOAuth2Config().Client(ctx, token)
	return client, nil
}

// getDriveService получает Google Drive service
func (d *DriveAdapter) getDriveService(ctx context.Context, userID string) (*drive.Service, error) {
	client, err := d.getHTTPClient(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get HTTP client: %w", err)
	}

	// Создаем Drive service
	service, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create Drive service: %w", err)
	}

	return service, nil
}

// ListFiles список файлов в Google Drive
func (d *DriveAdapter) ListFiles(ctx context.Context, userID, folderID string, pageSize int64) (*GoogleDriveResponse, error) {
	service, err := d.getDriveService(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get Drive service: %w", err)
	}

	// Базовый query для файлов
	query := "trashed=false"
	if folderID != "" {
		query += fmt.Sprintf(" and '%s' in parents", folderID)
	}

	// Выполняем запрос с retry логикой
	var files []*drive.File
	var nextPageToken string

	for i := 0; i < d.config.RetryCount; i++ {
		q := service.Files.List().
			Q(query).
			Fields("nextPageToken", "files(id,name,mimeType,size,createdTime,modifiedTime,parents,webViewLink)").
			PageSize(pageSize)

		if nextPageToken != "" {
			q = q.PageToken(nextPageToken)
		}

		fileList, err := q.Do()
		if err != nil {
			if i < d.config.RetryCount-1 {
				log.Printf("Retry %d/%d for ListFiles: %v", i+1, d.config.RetryCount, err)
				time.Sleep(d.config.RetryDelay)
				continue
			}
			return nil, fmt.Errorf("failed to list files after retries: %w", err)
		}

		files = append(files, fileList.Files...)
		nextPageToken = fileList.NextPageToken

		// Если есть еще страницы и мы не достигли лимита
		if nextPageToken == "" || len(files) >= int(pageSize) {
			break
		}
	}

	// Конвертируем в нашу структуру
	result := &GoogleDriveResponse{
		Files:         make([]GoogleDriveFile, 0, len(files)),
		NextPageToken: nextPageToken,
	}

	for _, file := range files {
		driveFile := d.convertDriveFile(file)
		result.Files = append(result.Files, *driveFile)
	}

	return result, nil
}

// GetFile получает информацию о файле
func (d *DriveAdapter) GetFile(ctx context.Context, userID, fileID string) (*GoogleDriveFile, error) {
	service, err := d.getDriveService(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get Drive service: %w", err)
	}

	for i := 0; i < d.config.RetryCount; i++ {
		file, err := service.Files.Get(fileID).
			Fields("id,name,mimeType,size,createdTime,modifiedTime,parents,webViewLink").
			Do()

		if err != nil {
			if i < d.config.RetryCount-1 {
				log.Printf("Retry %d/%d for GetFile: %v", i+1, d.config.RetryCount, err)
				time.Sleep(d.config.RetryDelay)
				continue
			}
			return nil, fmt.Errorf("failed to get file after retries: %w", err)
		}

		return d.convertDriveFile(file), nil
	}

	return nil, fmt.Errorf("failed to get file after all retries")
}

// DownloadFile скачивает файл
func (d *DriveAdapter) DownloadFile(ctx context.Context, userID, fileID, localPath string) error {
	service, err := d.getDriveService(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get Drive service: %w", err)
	}

	// Получаем информацию о файле
	file, err := service.Files.Get(fileID).Fields("name", "size").Do()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Проверяем размер файла
	if file.Size > int64(d.config.MaxFileSize) {
		return fmt.Errorf("file size %d exceeds maximum allowed size %d", file.Size, d.config.MaxFileSize)
	}

	// Создаем директорию если нужно
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Создаем локальный файл
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	// Скачиваем файл
	for i := 0; i < d.config.RetryCount; i++ {
		resp, err := service.Files.Get(fileID).Download()
		if err != nil {
			if i < d.config.RetryCount-1 {
				log.Printf("Retry %d/%d for DownloadFile: %v", i+1, d.config.RetryCount, err)
				time.Sleep(d.config.RetryDelay)
				continue
			}
			return fmt.Errorf("failed to download file after retries: %w", err)
		}
		defer resp.Body.Close()

		// Копируем контент
		_, err = io.Copy(localFile, resp.Body)
		if err != nil {
			if i < d.config.RetryCount-1 {
				log.Printf("Retry %d/%d for file copy: %v", i+1, d.config.RetryCount, err)
				time.Sleep(d.config.RetryDelay)
				continue
			}
			return fmt.Errorf("failed to copy file content after retries: %w", err)
		}

		log.Printf("Successfully downloaded file %s to %s", file.Name, localPath)
		return nil
	}

	return fmt.Errorf("failed to download file after all retries")
}

// UploadFile загружает файл в Google Drive
func (d *DriveAdapter) UploadFile(ctx context.Context, userID, localPath, folderID string) (*GoogleDriveFile, error) {
	service, err := d.getDriveService(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get Drive service: %w", err)
	}

	// Открываем локальный файл
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Проверяем размер файла
	if fileInfo.Size() > int64(d.config.MaxFileSize) {
		return nil, fmt.Errorf("file size %d exceeds maximum allowed size %d", fileInfo.Size(), d.config.MaxFileSize)
	}

	localFile, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	// Создаем metadata для файла
	fileName := filepath.Base(localPath)
	driveFile := &drive.File{
		Name:    fileName,
		Parents: []string{folderID},
	}

	// Загружаем файл
	for i := 0; i < d.config.RetryCount; i++ {
		uploadedFile, err := service.Files.Create(driveFile).
			Media(localFile).
			Fields("id,name,mimeType,size,createdTime,modifiedTime,parents,webViewLink").
			Do()

		if err != nil {
			if i < d.config.RetryCount-1 {
				log.Printf("Retry %d/%d for UploadFile: %v", i+1, d.config.RetryCount, err)
				time.Sleep(d.config.RetryDelay)
				continue
			}
			return nil, fmt.Errorf("failed to upload file after retries: %w", err)
		}

		log.Printf("Successfully uploaded file %s to Google Drive", fileName)
		return d.convertDriveFile(uploadedFile), nil
	}

	return nil, fmt.Errorf("failed to upload file after all retries")
}

// CreateFolder создает папку в Google Drive
func (d *DriveAdapter) CreateFolder(ctx context.Context, userID, folderName, parentFolderID string) (*GoogleDriveFolder, error) {
	service, err := d.getDriveService(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get Drive service: %w", err)
	}

	// Создаем metadata для папки
	folder := &drive.File{
		Name:     folderName,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{parentFolderID},
	}

	for i := 0; i < d.config.RetryCount; i++ {
		createdFolder, err := service.Files.Create(folder).
			Fields("id,name,createdTime,modifiedTime,parents,webViewLink").
			Do()

		if err != nil {
			if i < d.config.RetryCount-1 {
				log.Printf("Retry %d/%d for CreateFolder: %v", i+1, d.config.RetryCount, err)
				time.Sleep(d.config.RetryDelay)
				continue
			}
			return nil, fmt.Errorf("failed to create folder after retries: %w", err)
		}

		driveFolder := &GoogleDriveFolder{
			ID:           createdFolder.Id,
			Name:         createdFolder.Name,
			CreatedTime:  parseTime(createdFolder.CreatedTime),
			ModifiedTime: parseTime(createdFolder.ModifiedTime),
			Parents:      createdFolder.Parents,
			WebViewLink:  createdFolder.WebViewLink,
		}

		log.Printf("Successfully created folder %s in Google Drive", folderName)
		return driveFolder, nil
	}

	return nil, fmt.Errorf("failed to create folder after all retries")
}

// SearchFiles ищет файлы по имени
func (d *DriveAdapter) SearchFiles(ctx context.Context, userID, query string, pageSize int64) (*GoogleDriveResponse, error) {
	service, err := d.getDriveService(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get Drive service: %w", err)
	}

	// Формируем поисковый запрос
	searchQuery := fmt.Sprintf("name contains '%s' and trashed=false", query)

	for i := 0; i < d.config.RetryCount; i++ {
		fileList, err := service.Files.List().
			Q(searchQuery).
			Fields("nextPageToken", "files(id,name,mimeType,size,createdTime,modifiedTime,parents,webViewLink)").
			PageSize(pageSize).
			Do()

		if err != nil {
			if i < d.config.RetryCount-1 {
				log.Printf("Retry %d/%d for SearchFiles: %v", i+1, d.config.RetryCount, err)
				time.Sleep(d.config.RetryDelay)
				continue
			}
			return nil, fmt.Errorf("failed to search files after retries: %w", err)
		}

		// Конвертируем результаты
		result := &GoogleDriveResponse{
			Files:         make([]GoogleDriveFile, 0, len(fileList.Files)),
			NextPageToken: fileList.NextPageToken,
		}

		for _, file := range fileList.Files {
			driveFile := d.convertDriveFile(file)
			result.Files = append(result.Files, *driveFile)
		}

		return result, nil
	}

	return nil, fmt.Errorf("failed to search files after all retries")
}

// GetDownloadURL получает URL для скачивания файла
func (d *DriveAdapter) GetDownloadURL(ctx context.Context, userID, fileID string) (string, error) {
	service, err := d.getDriveService(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get Drive service: %w", err)
	}

	for i := 0; i < d.config.RetryCount; i++ {
		file, err := service.Files.Get(fileID).Fields("webContentLink").Do()
		if err != nil {
			if i < d.config.RetryCount-1 {
				log.Printf("Retry %d/%d for GetDownloadURL: %v", i+1, d.config.RetryCount, err)
				time.Sleep(d.config.RetryDelay)
				continue
			}
			return "", fmt.Errorf("failed to get download URL after retries: %w", err)
		}

		return file.WebContentLink, nil
	}

	return "", fmt.Errorf("failed to get download URL after all retries")
}

// convertDriveFile конвертирует drive.File в GoogleDriveFile
func (d *DriveAdapter) convertDriveFile(file *drive.File) *GoogleDriveFile {
	size := file.Size

	return &GoogleDriveFile{
		ID:           file.Id,
		Name:         file.Name,
		MimeType:     file.MimeType,
		Size:         size,
		CreatedTime:  parseTime(file.CreatedTime),
		ModifiedTime: parseTime(file.ModifiedTime),
		Parents:      file.Parents,
		WebViewLink:  file.WebViewLink,
	}
}

// parseTime парсит время из RFC3339 формата
func parseTime(timeStr string) time.Time {
	if timeStr == "" {
		return time.Time{}
	}

	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		log.Printf("Failed to parse time %s: %v", timeStr, err)
		return time.Time{}
	}

	return t
}

// getDriveServiceByAccount получает Drive service для конкретного Google аккаунта
func (d *DriveAdapter) getDriveServiceByAccount(ctx context.Context, userID, accountID string) (*drive.Service, error) {
	token, err := d.oauthService.GetValidTokenByAccount(ctx, userID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token for account %s: %w", accountID, err)
	}
	client := d.config.OAuth.ToOAuth2Config().Client(ctx, token)
	return drive.NewService(ctx, option.WithHTTPClient(client))
}

// ListFilesByAccount список файлов для конкретного аккаунта
func (d *DriveAdapter) ListFilesByAccount(ctx context.Context, userID, accountID, folderID string, pageSize int64) (*GoogleDriveResponse, error) {
	service, err := d.getDriveServiceByAccount(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	query := "trashed=false"
	if folderID != "" {
		query += fmt.Sprintf(" and '%s' in parents", folderID)
	}
	fileList, err := service.Files.List().
		Q(query).
		Fields("nextPageToken", "files(id,name,mimeType,size,createdTime,modifiedTime,parents,webViewLink)").
		PageSize(pageSize).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	result := &GoogleDriveResponse{Files: make([]GoogleDriveFile, 0, len(fileList.Files)), NextPageToken: fileList.NextPageToken}
	for _, f := range fileList.Files {
		result.Files = append(result.Files, *d.convertDriveFile(f))
	}
	return result, nil
}

// DownloadFileByAccount скачивает файл используя конкретный аккаунт
func (d *DriveAdapter) DownloadFileByAccount(ctx context.Context, userID, accountID, fileID, localPath string) error {
	service, err := d.getDriveServiceByAccount(ctx, userID, accountID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	resp, err := service.Files.Get(fileID).Download()
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// UploadFileByAccount загружает файл используя конкретный аккаунт
func (d *DriveAdapter) UploadFileByAccount(ctx context.Context, userID, accountID, localPath, folderID string) (*GoogleDriveFile, error) {
	service, err := d.getDriveServiceByAccount(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	uploaded, err := service.Files.Create(&drive.File{
		Name:    filepath.Base(localPath),
		Parents: []string{folderID},
	}).Media(f).Fields("id,name,mimeType,size,createdTime,modifiedTime,parents,webViewLink").Do()
	if err != nil {
		return nil, fmt.Errorf("failed to upload: %w", err)
	}
	return d.convertDriveFile(uploaded), nil
}

// CreateFolderByAccount создаёт папку используя конкретный аккаунт
func (d *DriveAdapter) CreateFolderByAccount(ctx context.Context, userID, accountID, name, parentID string) (*GoogleDriveFolder, error) {
	service, err := d.getDriveServiceByAccount(ctx, userID, accountID)
	if err != nil {
		return nil, err
	}
	created, err := service.Files.Create(&drive.File{
		Name:     name,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{parentID},
	}).Fields("id,name,createdTime,modifiedTime,parents,webViewLink").Do()
	if err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}
	return &GoogleDriveFolder{
		ID: created.Id, Name: created.Name,
		CreatedTime: parseTime(created.CreatedTime), ModifiedTime: parseTime(created.ModifiedTime),
		Parents: created.Parents, WebViewLink: created.WebViewLink,
	}, nil
}

// IsRetryableError проверяет можно ли повторить запрос
func (d *DriveAdapter) IsRetryableError(err error) bool {
	if e, ok := err.(*googleapi.Error); ok {
		// Retryable HTTP codes
		return e.Code >= 500 || e.Code == 429
	}

	// Network errors
	return strings.Contains(err.Error(), "connection") ||
		strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "temporary")
}

// SyncUserFiles синхронизирует файлы пользователя
func (d *DriveAdapter) SyncUserFiles(ctx context.Context, userID string) error {
	log.Printf("Starting file sync for user %s", userID)

	// Обновляем статус синхронизации
	err := d.updateSyncStatus(ctx, userID, "in_progress", "", 0, 0)
	if err != nil {
		log.Printf("Warning: failed to update sync status: %v", err)
	}

	// Получаем все файлы
	var allFiles []GoogleDriveFile
	totalFiles := 0

	for {
		result, err := d.ListFiles(ctx, userID, "", 1000)
		if err != nil {
			d.updateSyncStatus(ctx, userID, "error", fmt.Sprintf("Failed to list files: %v", err), 0, 0)
			return fmt.Errorf("failed to list files: %w", err)
		}

		allFiles = append(allFiles, result.Files...)
		totalFiles += len(result.Files)

		if result.NextPageToken == "" {
			break
		}
	}

	// Сохраняем файлы в базу данных
	syncedFiles := 0
	for _, file := range allFiles {
		err := d.saveFileToDB(ctx, userID, &file)
		if err != nil {
			log.Printf("Failed to save file %s to DB: %v", file.ID, err)
			continue
		}
		syncedFiles++
	}

	// Обновляем статус завершения
	d.updateSyncStatus(ctx, userID, "completed", "", totalFiles, syncedFiles)

	log.Printf("Sync completed for user %s: %d total files, %d synced", userID, totalFiles, syncedFiles)
	return nil
}

// saveFileToDB сохраняет информацию о файле в базу данных
func (d *DriveAdapter) saveFileToDB(ctx context.Context, userID string, file *GoogleDriveFile) error {
	query := `
		INSERT INTO google_drive_files (user_id, file_id, name, mime_type, size, created_time, modified_time, parents, web_view_link, sync_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 1)
		ON CONFLICT (file_id) 
		DO UPDATE SET 
			name = EXCLUDED.name,
			mime_type = EXCLUDED.mime_type,
			size = EXCLUDED.size,
			modified_time = EXCLUDED.modified_time,
			parents = EXCLUDED.parents,
			web_view_link = EXCLUDED.web_view_link,
			sync_version = google_drive_files.sync_version + 1,
			updated_at = NOW()
	`

	_, err := d.oauthService.db.Exec(ctx, query,
		userID, file.ID, file.Name, file.MimeType, file.Size,
		file.CreatedTime, file.ModifiedTime, file.Parents, file.WebViewLink,
	)

	return err
}

// updateSyncStatus обновляет статус синхронизации
func (d *DriveAdapter) updateSyncStatus(ctx context.Context, userID, status, errorMessage string, totalFiles, syncedFiles int) error {
	query := `
		INSERT INTO google_drive_sync_status (user_id, sync_status, error_message, total_files, synced_files)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id)
		DO UPDATE SET 
			sync_status = EXCLUDED.sync_status,
			error_message = EXCLUDED.error_message,
			total_files = EXCLUDED.total_files,
			synced_files = EXCLUDED.synced_files,
			last_sync_at = NOW(),
			updated_at = NOW()
	`

	_, err := d.oauthService.db.Exec(ctx, query, userID, status, errorMessage, totalFiles, syncedFiles)
	return err
}
