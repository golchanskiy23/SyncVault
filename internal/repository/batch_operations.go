package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BatchRepository struct {
	pool *pgxpool.Pool
}

func NewBatchRepository(pool *pgxpool.Pool) *BatchRepository {
	return &BatchRepository{
		pool: pool,
	}
}

type BatchFile struct {
	ID               int64     `json:"id"`
	UserID           int64     `json:"user_id"`
	FilePath         string    `json:"file_path"`
	FileName         string    `json:"file_name"`
	FileSizeBytes    int64     `json:"file_size_bytes"`
	MimeType         string    `json:"mime_type"`
	ChecksumMD5      string    `json:"checksum_md5"`
	ChecksumSHA256   string    `json:"checksum_sha256"`
	IsDeleted        bool      `json:"is_deleted"`
	CurrentVersionID *int64    `json:"current_version_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type BatchFileVersion struct {
	ID             int64     `json:"id"`
	FileID         int64     `json:"file_id"`
	VersionNumber  int       `json:"version_number"`
	StorageNodeID  *int64    `json:"storage_node_id"`
	FileSizeBytes  int64     `json:"file_size_bytes"`
	ChecksumSHA256 string    `json:"checksum_sha256"`
	StoragePath    string    `json:"storage_path"`
	IsCurrent      bool      `json:"is_current"`
	CreatedAt      time.Time `json:"created_at"`
}

type BatchSyncEvent struct {
	ID               int64      `json:"id"`
	JobID            int64      `json:"job_id"`
	FileID           *int64     `json:"file_id"`
	StorageNodeID    *int64     `json:"storage_node_id"`
	EventType        string     `json:"event_type"`
	Status           string     `json:"status"`
	SourcePath       string     `json:"source_path"`
	DestinationPath  string     `json:"destination_path"`
	FileSizeBytes    int64      `json:"file_size_bytes"`
	BytesTransferred int64      `json:"bytes_transferred"`
	TransferRateMbps float64    `json:"transfer_rate_mbps"`
	ErrorCode        *string    `json:"error_code"`
	ErrorMessage     *string    `json:"error_message"`
	Metadata         string     `json:"metadata"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at"`
}

func (br *BatchRepository) BatchInsertFiles(ctx context.Context, files []*BatchFile) (int64, error) {
	if len(files) == 0 {
		return 0, nil
	}

	rows := make([][]interface{}, len(files))
	for i, file := range files {
		rows[i] = []interface{}{
			file.ID,
			file.UserID,
			file.FilePath,
			file.FileName,
			file.FileSizeBytes,
			file.MimeType,
			file.ChecksumMD5,
			file.ChecksumSHA256,
			file.IsDeleted,
			file.CurrentVersionID,
			file.CreatedAt,
			file.UpdatedAt,
		}
	}

	columnNames := []string{
		"id", "user_id", "file_path", "file_name", "file_size_bytes",
		"mime_type", "checksum_md5", "checksum_sha256", "is_deleted",
		"current_version_id", "created_at", "updated_at",
	}

	copyCount, err := br.pool.CopyFrom(
		ctx,
		pgx.Identifier{"files"},
		columnNames,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to batch insert files: %w", err)
	}

	return copyCount, nil
}

func (br *BatchRepository) BatchInsertFileVersions(ctx context.Context, versions []*BatchFileVersion) (int64, error) {
	if len(versions) == 0 {
		return 0, nil
	}

	rows := make([][]interface{}, len(versions))
	for i, version := range versions {
		rows[i] = []interface{}{
			version.ID,
			version.FileID,
			version.VersionNumber,
			version.StorageNodeID,
			version.FileSizeBytes,
			version.ChecksumSHA256,
			version.StoragePath,
			version.IsCurrent,
			version.CreatedAt,
		}
	}

	columnNames := []string{
		"id", "file_id", "version_number", "storage_node_id",
		"file_size_bytes", "checksum_sha256", "storage_path",
		"is_current", "created_at",
	}

	copyCount, err := br.pool.CopyFrom(
		ctx,
		pgx.Identifier{"file_versions"},
		columnNames,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to batch insert file versions: %w", err)
	}

	return copyCount, nil
}

func (br *BatchRepository) BatchInsertSyncEvents(ctx context.Context, events []*BatchSyncEvent) (int64, error) {
	if len(events) == 0 {
		return 0, nil
	}

	rows := make([][]interface{}, len(events))
	for i, event := range events {
		rows[i] = []interface{}{
			event.ID,
			event.JobID,
			event.FileID,
			event.StorageNodeID,
			event.EventType,
			event.Status,
			event.SourcePath,
			event.DestinationPath,
			event.FileSizeBytes,
			event.BytesTransferred,
			event.TransferRateMbps,
			event.ErrorCode,
			event.ErrorMessage,
			event.Metadata,
			event.CreatedAt,
			event.CompletedAt,
		}
	}

	columnNames := []string{
		"id", "job_id", "file_id", "storage_node_id", "event_type", "status",
		"source_path", "destination_path", "file_size_bytes", "bytes_transferred",
		"transfer_rate_mbps", "error_code", "error_message", "metadata",
		"created_at", "completed_at",
	}

	copyCount, err := br.pool.CopyFrom(
		ctx,
		pgx.Identifier{"sync_events"},
		columnNames,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to batch insert sync events: %w", err)
	}

	return copyCount, nil
}

func (br *BatchRepository) BatchUpdateFiles(ctx context.Context, updates []FileUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := br.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}

	for _, update := range updates {
		query := `
			UPDATE files 
			SET file_name = $1, file_size_bytes = $2, updated_at = $3
			WHERE id = $4 AND user_id = $5
		`
		batch.Queue(query, update.FileName, update.FileSizeBytes, time.Now(), update.ID, update.UserID)
	}

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	for i := 0; i < len(updates); i++ {
		_, err := results.Exec()
		if err != nil {
			return fmt.Errorf("failed to update file at index %d: %w", i, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

type FileUpdate struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"user_id"`
	FileName      string `json:"file_name"`
	FileSizeBytes int64  `json:"file_size_bytes"`
}

func (br *BatchRepository) BatchDeleteFiles(ctx context.Context, fileIDs []int64, userID int64) error {
	if len(fileIDs) == 0 {
		return nil
	}

	query := `
		UPDATE files 
		SET is_deleted = true, updated_at = $1
		WHERE id = ANY($2) AND user_id = $3
	`

	_, err := br.pool.Exec(ctx, query, time.Now(), fileIDs, userID)
	if err != nil {
		return fmt.Errorf("failed to batch delete files: %w", err)
	}

	return nil
}

func (br *BatchRepository) BatchUpsertFiles(ctx context.Context, files []*BatchFile) (int64, error) {
	if len(files) == 0 {
		return 0, nil
	}

	tx, err := br.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}

	for _, file := range files {
		query := `
			INSERT INTO files (
				id, user_id, file_path, file_name, file_size_bytes,
				mime_type, checksum_md5, checksum_sha256, is_deleted,
				current_version_id, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (id) DO UPDATE SET
				file_name = EXCLUDED.file_name,
				file_size_bytes = EXCLUDED.file_size_bytes,
				mime_type = EXCLUDED.mime_type,
				checksum_md5 = EXCLUDED.checksum_md5,
				checksum_sha256 = EXCLUDED.checksum_sha256,
				current_version_id = EXCLUDED.current_version_id,
				updated_at = EXCLUDED.updated_at
		`
		batch.Queue(query,
			file.ID, file.UserID, file.FilePath, file.FileName, file.FileSizeBytes,
			file.MimeType, file.ChecksumMD5, file.ChecksumSHA256, file.IsDeleted,
			file.CurrentVersionID, file.CreatedAt, file.UpdatedAt,
		)
	}

	results := tx.SendBatch(ctx, batch)
	defer results.Close()

	var successCount int64
	for i := 0; i < len(files); i++ {
		cmdTag, err := results.Exec()
		if err != nil {
			return 0, fmt.Errorf("failed to upsert file at index %d: %w", i, err)
		}
		successCount += cmdTag.RowsAffected()
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return successCount, nil
}

func (br *BatchRepository) BatchInsertWithConflictHandling(ctx context.Context, files []*BatchFile, conflictAction string) (int64, error) {
	if len(files) == 0 {
		return 0, nil
	}

	rows := make([][]interface{}, len(files))
	for i, file := range files {
		rows[i] = []interface{}{
			file.ID,
			file.UserID,
			file.FilePath,
			file.FileName,
			file.FileSizeBytes,
			file.MimeType,
			file.ChecksumMD5,
			file.ChecksumSHA256,
			file.IsDeleted,
			file.CurrentVersionID,
			file.CreatedAt,
			file.UpdatedAt,
		}
	}

	columnNames := []string{
		"id", "user_id", "file_path", "file_name", "file_size_bytes",
		"mime_type", "checksum_md5", "checksum_sha256", "is_deleted",
		"current_version_id", "created_at", "updated_at",
	}

	tempTableName := fmt.Sprintf("temp_files_import_%d", time.Now().Unix())

	createTempTable := fmt.Sprintf(`
		CREATE TEMP TABLE %s (LIKE files INCLUDING ALL)
	`, tempTableName)

	_, err := br.pool.Exec(ctx, createTempTable)
	if err != nil {
		return 0, fmt.Errorf("failed to create temporary table: %w", err)
	}
	defer br.pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tempTableName))

	_, err = br.pool.CopyFrom(
		ctx,
		pgx.Identifier{tempTableName},
		columnNames,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to copy to temporary table: %w", err)
	}

	var insertQuery string
	switch conflictAction {
	case "ignore":
		insertQuery = fmt.Sprintf(`
			INSERT INTO files SELECT * FROM %s
			ON CONFLICT (id) DO NOTHING
		`, tempTableName)
	case "update":
		insertQuery = fmt.Sprintf(`
			INSERT INTO files SELECT * FROM %s
			ON CONFLICT (id) DO UPDATE SET
				file_name = EXCLUDED.file_name,
				file_size_bytes = EXCLUDED.file_size_bytes,
				mime_type = EXCLUDED.mime_type,
				checksum_md5 = EXCLUDED.checksum_md5,
				checksum_sha256 = EXCLUDED.checksum_sha256,
				current_version_id = EXCLUDED.current_version_id,
				updated_at = EXCLUDED.updated_at
		`, tempTableName)
	default:
		return 0, fmt.Errorf("unsupported conflict action: %s", conflictAction)
	}

	cmdTag, err := br.pool.Exec(ctx, insertQuery)
	if err != nil {
		return 0, fmt.Errorf("failed to insert from temporary table: %w", err)
	}

	return cmdTag.RowsAffected(), nil
}

type BatchOperationStats struct {
	TotalRows      int64         `json:"total_rows"`
	SuccessfulRows int64         `json:"successful_rows"`
	FailedRows     int64         `json:"failed_rows"`
	Duration       time.Duration `json:"duration"`
	RowsPerSecond  float64       `json:"rows_per_second"`
}

func (br *BatchRepository) BatchInsertWithStats(ctx context.Context, files []*BatchFile) (*BatchOperationStats, error) {
	startTime := time.Now()

	totalRows := int64(len(files))
	if totalRows == 0 {
		return &BatchOperationStats{
			TotalRows:      0,
			SuccessfulRows: 0,
			FailedRows:     0,
			Duration:       0,
			RowsPerSecond:  0,
		}, nil
	}

	successfulRows, err := br.BatchInsertFiles(ctx, files)
	if err != nil {
		return nil, fmt.Errorf("batch insert failed: %w", err)
	}

	duration := time.Since(startTime)
	failedRows := totalRows - successfulRows
	rowsPerSecond := float64(successfulRows) / duration.Seconds()

	stats := &BatchOperationStats{
		TotalRows:      totalRows,
		SuccessfulRows: successfulRows,
		FailedRows:     failedRows,
		Duration:       duration,
		RowsPerSecond:  rowsPerSecond,
	}

	return stats, nil
}
