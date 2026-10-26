package adapters

import (
	"context"
	"fmt"
	"strings"

	"syncvault/internal/oauth/google"
	"syncvault/internal/sync"
)

// listPageSize — размер страницы при листинге папок Drive
const listPageSize = 1000

// GoogleDriveAdapter реализует sync.ComplexStorageAPI поверх google.DriveAdapter.
// Это единственное место где sync пакет знает о Google Drive.
type GoogleDriveAdapter struct {
	drive     *google.DriveAdapter
	userID    string
	accountID string // email аккаунта
	rootID    string // корневая папка узла на Drive ("root" — корень Drive)
}

func NewGoogleDriveAdapter(drive *google.DriveAdapter, userID, accountID, rootFolderID string) *GoogleDriveAdapter {
	if rootFolderID == "" {
		rootFolderID = "root"
	}
	return &GoogleDriveAdapter{drive: drive, userID: userID, accountID: accountID, rootID: rootFolderID}
}

func (a *GoogleDriveAdapter) folder(folderID string) string {
	if folderID == "" {
		return a.rootID
	}
	return folderID
}

func (a *GoogleDriveAdapter) ListFiles(ctx context.Context, folderID string) ([]sync.RemoteFile, error) {
	resp, err := a.drive.ListFilesByAccount(ctx, a.userID, a.accountID, a.folder(folderID), listPageSize)
	if err != nil {
		return nil, fmt.Errorf("google drive list: %w", err)
	}

	result := make([]sync.RemoteFile, 0, len(resp.Files))
	for _, f := range resp.Files {
		isDir := f.MimeType == "application/vnd.google-apps.folder"
		// Google Docs/Sheets/... нельзя скачать как файл (нужен export) — пропускаем
		if !isDir && strings.HasPrefix(f.MimeType, "application/vnd.google-apps.") {
			continue
		}
		hash := f.Sha256Checksum
		if hash == "" && !isDir {
			// Без хеша содержимого файл не сравнить с локальным — считаем его изменившимся
			hash = "drive:" + f.ID + ":" + f.ModifiedTime.String()
		}
		result = append(result, sync.RemoteFile{
			ID:       f.ID,
			Path:     f.Name,
			Hash:     hash,
			Size:     f.Size,
			IsDir:    isDir,
			MimeType: f.MimeType,
			ModTime:  f.ModifiedTime,
		})
	}
	return result, nil
}

func (a *GoogleDriveAdapter) Download(ctx context.Context, fileID, localPath string) error {
	return a.drive.DownloadFileByAccount(ctx, a.userID, a.accountID, fileID, localPath)
}

func (a *GoogleDriveAdapter) Upload(ctx context.Context, localPath, folderID string) (string, error) {
	uploaded, err := a.drive.UploadFileByAccount(ctx, a.userID, a.accountID, localPath, a.folder(folderID))
	if err != nil {
		return "", fmt.Errorf("google drive upload: %w", err)
	}
	return uploaded.ID, nil
}

func (a *GoogleDriveAdapter) Update(ctx context.Context, fileID, localPath string) error {
	if _, err := a.drive.UpdateFileByAccount(ctx, a.userID, a.accountID, fileID, localPath); err != nil {
		return fmt.Errorf("google drive update: %w", err)
	}
	return nil
}

// Delete перемещает файл в корзину Drive — его можно восстановить в течение 30 дней
func (a *GoogleDriveAdapter) Delete(ctx context.Context, fileID string) error {
	return a.drive.TrashFileByAccount(ctx, a.userID, a.accountID, fileID)
}

func (a *GoogleDriveAdapter) MkDir(ctx context.Context, name, parentID string) (string, error) {
	folder, err := a.drive.CreateFolderByAccount(ctx, a.userID, a.accountID, name, a.folder(parentID))
	if err != nil {
		return "", fmt.Errorf("google drive mkdir: %w", err)
	}
	return folder.ID, nil
}
