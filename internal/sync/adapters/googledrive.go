package adapters

import (
	"context"
	"fmt"

	"syncvault/internal/oauth/google"
	"syncvault/internal/sync"
)

// GoogleDriveAdapter реализует sync.ComplexStorageAPI поверх google.DriveAdapter.
// Это единственное место где sync пакет знает о Google Drive.
type GoogleDriveAdapter struct {
	drive     *google.DriveAdapter
	userID    string
	accountID string // email аккаунта
}

func NewGoogleDriveAdapter(drive *google.DriveAdapter, userID, accountID string) *GoogleDriveAdapter {
	return &GoogleDriveAdapter{drive: drive, userID: userID, accountID: accountID}
}

func (a *GoogleDriveAdapter) ListFiles(ctx context.Context, folderID string) ([]sync.RemoteFile, error) {
	resp, err := a.drive.ListFilesByAccount(ctx, a.userID, a.accountID, folderID, 1000)
	if err != nil {
		return nil, fmt.Errorf("google drive list: %w", err)
	}

	result := make([]sync.RemoteFile, 0, len(resp.Files))
	for _, f := range resp.Files {
		result = append(result, sync.RemoteFile{
			ID:       f.ID,
			Path:     f.Name,
			Size:     f.Size,
			IsDir:    f.MimeType == "application/vnd.google-apps.folder",
			MimeType: f.MimeType,
			// Google Drive не возвращает SHA-256 напрямую — используем ID как прокси хеша
			// В реальной системе можно использовать md5Checksum из Drive API
			Hash: f.ID,
		})
	}
	return result, nil
}

func (a *GoogleDriveAdapter) Download(ctx context.Context, fileID, localPath string) error {
	return a.drive.DownloadFileByAccount(ctx, a.userID, a.accountID, fileID, localPath)
}

func (a *GoogleDriveAdapter) Upload(ctx context.Context, localPath, folderID string) (string, error) {
	uploaded, err := a.drive.UploadFileByAccount(ctx, a.userID, a.accountID, localPath, folderID)
	if err != nil {
		return "", fmt.Errorf("google drive upload: %w", err)
	}
	return uploaded.ID, nil
}

func (a *GoogleDriveAdapter) Delete(_ context.Context, _ string) error {
	// Google Drive API поддерживает удаление через files.delete
	// Оставляем как TODO — требует добавления метода в DriveAdapter
	return fmt.Errorf("delete not yet implemented for Google Drive")
}

func (a *GoogleDriveAdapter) MkDir(ctx context.Context, name, parentID string) (string, error) {
	folder, err := a.drive.CreateFolderByAccount(ctx, a.userID, a.accountID, name, parentID)
	if err != nil {
		return "", fmt.Errorf("google drive mkdir: %w", err)
	}
	return folder.ID, nil
}
