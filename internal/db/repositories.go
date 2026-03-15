package db

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5"
)

// Import all repository types and constructors
// This file re-exports all repository functionality

type (
	// File represents a file in the system
	File = FileRepoType
	
	// FileVersion represents a file version
	FileVersion = FileVersionRepoType
	
	// DirectoryNode represents a directory in the tree structure
	DirectoryNode = DirectoryRepoType
	
	// BatchFile represents a file for bulk operations
	BatchFile = BatchRepoFileType
	
	// BatchFileVersion represents a file version for bulk operations
	BatchFileVersion = BatchRepoFileVersionType
	
	// BatchSyncEvent represents a sync event for bulk operations
	BatchSyncEvent = BatchRepoSyncEventType
	
	// FileUpdate represents a file update operation
	FileUpdate = BatchRepoFileUpdateType
	
	// BatchOperationStats represents statistics for batch operations
	BatchOperationStats = BatchRepoOperationStatsType
	
	// QueryPlan represents the execution plan of a query
	QueryPlan = QueryAnalyzerPlanType
	
	// TableStats represents PostgreSQL table statistics
	TableStats = QueryAnalyzerTableStatsType
	
	// IndexRecommendation represents an index recommendation
	IndexRecommendation = QueryAnalyzerIndexRecommendationType
	
	// SlowQuery represents a slow query from pg_stat_statements
	SlowQuery = QueryAnalyzerSlowQueryType
	
	// IndexUsage represents index usage statistics
	IndexUsage = QueryAnalyzerIndexUsageType
	
	// BufferUsage represents buffer usage statistics
	BufferUsage = QueryAnalyzerBufferUsageType
)

// Repository interfaces
type (
	// FileRepository interface
	FileRepositoryInterface interface {
		Save(ctx context.Context, file *File) error
		FindByID(ctx context.Context, id int64) (*File, error)
		FindByHash(ctx context.Context, userID int64, checksumSHA256 string) (*File, error)
		ListWithPagination(ctx context.Context, userID int64, offset, limit int) ([]*File, error)
		ListWithPaginationAndFilter(ctx context.Context, userID int64, filter FileFilter, offset, limit int) ([]*File, error)
		Count(ctx context.Context, userID int64) (int64, error)
		CountWithFilter(ctx context.Context, userID int64, filter FileFilter) (int64, error)
		Update(ctx context.Context, file *File) error
		SoftDelete(ctx context.Context, id int64, userID int64) error
		FindByPath(ctx context.Context, userID int64, filePath string) (*File, error)
		UpdateCurrentVersion(ctx context.Context, fileID int64, versionID int64) error
		GetStorageUsage(ctx context.Context, userID int64) (int64, error)
		FindDuplicates(ctx context.Context, userID int64) ([]*File, error)
		Exists(ctx context.Context, userID int64, filePath string) (bool, error)
		BatchSave(ctx context.Context, files []*File) error
	}
	
	// FileVersionRepository interface
	FileVersionRepositoryInterface interface {
		Save(ctx context.Context, version *FileVersion) error
		FindByID(ctx context.Context, id int64) (*FileVersion, error)
		FindByFileID(ctx context.Context, fileID int64) ([]*FileVersion, error)
		GetVersionHistoryWithWindow(ctx context.Context, fileID int64) ([]*FileVersionWithRowNumber, error)
		FindCurrent(ctx context.Context, fileID int64) (*FileVersion, error)
		SetAsCurrent(ctx context.Context, versionID int64) error
		Delete(ctx context.Context, id int64) error
		FindByChecksum(ctx context.Context, checksum string) ([]*FileVersion, error)
		GetVersionCount(ctx context.Context, fileID int64) (int64, error)
	}
	
	// DirectoryRepository interface
	DirectoryRepositoryInterface interface {
		GetDirectoryTree(ctx context.Context, userID int64) ([]*DirectoryNode, error)
		GetDirectoryTreeWithPath(ctx context.Context, userID int64, rootPath string) ([]*DirectoryNode, error)
		GetSubdirectories(ctx context.Context, userID int64, parentPath string) ([]*DirectoryNode, error)
		GetDirectoryStats(ctx context.Context, userID int64) (*DirectoryStats, error)
		FindPath(ctx context.Context, userID int64, targetPath string) ([]*DirectoryNode, error)
		SearchInTree(ctx context.Context, userID int64, searchTerm string) ([]*DirectoryNode, error)
	}
	
	// BatchRepository interface
	BatchRepositoryInterface interface {
		BatchInsertFiles(ctx context.Context, files []*BatchFile) (int64, error)
		BatchInsertFileVersions(ctx context.Context, versions []*BatchFileVersion) (int64, error)
		BatchInsertSyncEvents(ctx context.Context, events []*BatchSyncEvent) (int64, error)
		BatchUpdateFiles(ctx context.Context, updates []FileUpdate) error
		BatchDeleteFiles(ctx context.Context, fileIDs []int64, userID int64) error
		BatchUpsertFiles(ctx context.Context, files []*BatchFile) (int64, error)
		BatchInsertWithConflictHandling(ctx context.Context, files []*BatchFile, conflictAction string) (int64, error)
		BatchInsertWithStats(ctx context.Context, files []*BatchFile) (*BatchOperationStats, error)
	}
	
	// QueryAnalyzer interface
	QueryAnalyzerInterface interface {
		AnalyzeFileQuery(ctx context.Context, query string, args ...interface{}) (*QueryPlan, error)
		AnalyzeFileByIDQuery(ctx context.Context, fileID int64) (*QueryPlan, error)
		AnalyzeFileListQuery(ctx context.Context, userID int64, offset, limit int) (*QueryPlan, error)
		AnalyzeFileSearchQuery(ctx context.Context, userID int64, searchTerm string) (*QueryPlan, error)
		AnalyzeVersionHistoryQuery(ctx context.Context, fileID int64) (*QueryPlan, error)
		AnalyzeDirectoryTreeQuery(ctx context.Context, userID int64) (*QueryPlan, error)
		AnalyzeSyncEventsQuery(ctx context.Context, userID int64, limit int) (*QueryPlan, error)
		GetTableStats(ctx context.Context, tableName string) (*TableStats, error)
		CheckForSeqScans(plan *QueryPlan) []string
		RecommendIndexes(ctx context.Context, query string, tableName string) ([]IndexRecommendation, error)
		GetSlowQueries(ctx context.Context, limit int) ([]SlowQuery, error)
		AnalyzeIndexUsage(ctx context.Context, tableName string) ([]IndexUsage, error)
		GetBufferUsage(ctx context.Context, query string) (*BufferUsage, error)
	}
)

// Repository constructors
func NewFileRepository(pool *pgxpool.Pool) FileRepositoryInterface {
	return &fileRepositoryImpl{pool: pool}
}

func NewFileVersionRepository(pool *pgxpool.Pool) FileVersionRepositoryInterface {
	return &fileVersionRepositoryImpl{pool: pool}
}

func NewDirectoryRepository(pool *pgxpool.Pool) DirectoryRepositoryInterface {
	return &directoryRepositoryImpl{pool: pool}
}

func NewBatchRepository(pool *pgxpool.Pool) BatchRepositoryInterface {
	return &batchRepositoryImpl{pool: pool}
}

func NewQueryAnalyzer(pool *pgxpool.Pool) QueryAnalyzerInterface {
	return &queryAnalyzerImpl{pool: pool}
}

// Implementation types (these would be the actual implementations from the repository packages)
type (
	fileRepositoryImpl struct {
		pool *pgxpool.Pool
	}
	
	fileVersionRepositoryImpl struct {
		pool *pgxpool.Pool
	}
	
	directoryRepositoryImpl struct {
		pool *pgxpool.Pool
	}
	
	batchRepositoryImpl struct {
		pool *pgxpool.Pool
	}
	
	queryAnalyzerImpl struct {
		pool *pgxpool.Pool
	}
)

// Type aliases for repository types
type (
	FileRepoType = struct {
		ID               int64  `json:"id" db:"id"`
		UserID           int64  `json:"user_id" db:"user_id"`
		FilePath         string `json:"file_path" db:"file_path"`
		FileName         string `json:"file_name" db:"file_name"`
		FileSizeBytes    int64  `json:"file_size_bytes" db:"file_size_bytes"`
		MimeType         string `json:"mime_type" db:"mime_type"`
		ChecksumMD5      string `json:"checksum_md5" db:"checksum_md5"`
		ChecksumSHA256   string `json:"checksum_sha256" db:"checksum_sha256"`
		IsDeleted        bool   `json:"is_deleted" db:"is_deleted"`
		CurrentVersionID *int64 `json:"current_version_id" db:"current_version_id"`
		CreatedAt        string `json:"created_at" db:"created_at"`
		UpdatedAt        string `json:"updated_at" db:"updated_at"`
	}
	
	FileVersionRepoType = struct {
		ID              int64  `json:"id" db:"id"`
		FileID          int64  `json:"file_id" db:"file_id"`
		VersionNumber   int    `json:"version_number" db:"version_number"`
		StorageNodeID   *int64 `json:"storage_node_id" db:"storage_node_id"`
		FileSizeBytes   int64  `json:"file_size_bytes" db:"file_size_bytes"`
		ChecksumSHA256  string `json:"checksum_sha256" db:"checksum_sha256"`
		StoragePath     string `json:"storage_path" db:"storage_path"`
		IsCurrent       bool   `json:"is_current" db:"is_current"`
		CreatedAt       string `json:"created_at" db:"created_at"`
	}
	
	DirectoryRepoType = struct {
		ID        int64  `json:"id" db:"id"`
		UserID    int64  `json:"user_id" db:"user_id"`
		ParentID  *int64 `json:"parent_id" db:"parent_id"`
		Name      string `json:"name" db:"name"`
		Path      string `json:"path" db:"path"`
		IsDir     bool   `json:"is_dir" db:"is_dir"`
		FileSize  int64  `json:"file_size" db:"file_size"`
		CreatedAt string `json:"created_at" db:"created_at"`
		UpdatedAt string `json:"updated_at" db:"updated_at"`
		Level     int    `json:"level" db:"level"`
	}
	
	BatchRepoFileType = struct {
		ID               int64  `json:"id"`
		UserID           int64  `json:"user_id"`
		FilePath         string `json:"file_path"`
		FileName         string `json:"file_name"`
		FileSizeBytes    int64  `json:"file_size_bytes"`
		MimeType         string `json:"mime_type"`
		ChecksumMD5      string `json:"checksum_md5"`
		ChecksumSHA256   string `json:"checksum_sha256"`
		IsDeleted        bool   `json:"is_deleted"`
		CurrentVersionID *int64 `json:"current_version_id"`
		CreatedAt        string `json:"created_at"`
		UpdatedAt        string `json:"updated_at"`
	}
	
	BatchRepoFileVersionType = struct {
		ID              int64  `json:"id"`
		FileID          int64  `json:"file_id"`
		VersionNumber   int    `json:"version_number"`
		StorageNodeID   *int64 `json:"storage_node_id"`
		FileSizeBytes   int64  `json:"file_size_bytes"`
		ChecksumSHA256  string `json:"checksum_sha256"`
		StoragePath     string `json:"storage_path"`
		IsCurrent       bool   `json:"is_current"`
		CreatedAt       string `json:"created_at"`
	}
	
	BatchRepoSyncEventType = struct {
		ID               int64  `json:"id"`
		JobID            int64  `json:"job_id"`
		FileID           *int64 `json:"file_id"`
		StorageNodeID    *int64 `json:"storage_node_id"`
		EventType        string `json:"event_type"`
		Status           string `json:"status"`
		SourcePath       string `json:"source_path"`
		DestinationPath  string `json:"destination_path"`
		FileSizeBytes    int64  `json:"file_size_bytes"`
		BytesTransferred int64  `json:"bytes_transferred"`
		TransferRateMbps float64 `json:"transfer_rate_mbps"`
		ErrorCode        *string `json:"error_code"`
		ErrorMessage     *string `json:"error_message"`
		Metadata         string `json:"metadata"`
		CreatedAt        string `json:"created_at"`
		CompletedAt      *string `json:"completed_at"`
	}
	
	BatchRepoFileUpdateType = struct {
		ID            int64  `json:"id"`
		UserID        int64  `json:"user_id"`
		FileName      string `json:"file_name"`
		FileSizeBytes int64  `json:"file_size_bytes"`
	}
	
	BatchRepoOperationStatsType = struct {
		TotalRows      int64  `json:"total_rows"`
		SuccessfulRows int64  `json:"successful_rows"`
		FailedRows     int64  `json:"failed_rows"`
		Duration       string `json:"duration"`
		RowsPerSecond  float64 `json:"rows_per_second"`
	}
	
	QueryAnalyzerPlanType = struct {
		Query          string                 `json:"query"`
		ExecutionTime  string                 `json:"execution_time"`
		TotalCost      float64                `json:"total_cost"`
		PlanningTime   string                 `json:"planning_time"`
		ActualRows     int64                  `json:"actual_rows"`
		PlannedRows    int64                  `json:"planned_rows"`
		SeqScanCount   int                    `json:"seq_scan_count"`
		IndexScanCount int                    `json:"index_scan_count"`
		Details        map[string]interface{}   `json:"details"`
	}
	
	QueryAnalyzerTableStatsType = struct {
		SchemaName       string `json:"schema_name"`
		TableName        string `json:"table_name"`
		ColumnName       string `json:"column_name"`
		NumDistinct     int64  `json:"n_distinct"`
		NumInserts      int64  `json:"n_tup_ins"`
		NumUpdates      int64  `json:"n_tup_upd"`
		NumDeletes      int64  `json:"n_tup_del"`
		LiveTuples      int64  `json:"n_live_tup"`
		VacuumCount     int64  `json:"vacuum_count"`
		AutoVacuumCount int64  `json:"autovacuum_count"`
		AnalyzeCount    int64  `json:"analyze_count"`
		AutoAnalyzeCount int64  `json:"autoanalyze_count"`
	}
	
	QueryAnalyzerIndexRecommendationType = struct {
		Type          string   `json:"type"`
		Columns       []string `json:"columns"`
		Reason        string   `json:"reason"`
		EstimatedGain string   `json:"estimated_gain"`
	}
	
	QueryAnalyzerSlowQueryType = struct {
		Query          string  `json:"query"`
		Calls          int64   `json:"calls"`
		TotalExecTime  float64 `json:"total_exec_time"`
		Rows           int64   `json:"rows"`
		AvgExecTimeMs  float64 `json:"avg_exec_time_ms"`
		StddevExecTime float64 `json:"stddev_exec_time"`
	}
	
	QueryAnalyzerIndexUsageType = struct {
		SchemaName  string `json:"schema_name"`
		TableName   string `json:"table_name"`
		IndexName   string `json:"index_name"`
		IndexDef    string `json:"index_def"`
		IdxScan     int64  `json:"idx_scan"`
		IdxTupRead  int64  `json:"idx_tup_read"`
		IdxTupFetch int64  `json:"idx_tup_fetch"`
		TableSize   int64  `json:"table_size"`
	}
	
	QueryAnalyzerBufferUsageType = struct {
		SharedHitBlocks  int64 `json:"shared_hit_blocks"`
		SharedReadBlocks int64 `json:"shared_read_blocks"`
		LocalHitBlocks   int64 `json:"local_hit_blocks"`
		LocalReadBlocks  int64 `json:"local_read_blocks"`
	}
)

// Additional types needed
type (
	FileFilter = struct {
		FileName     string `json:"file_name"`
		MimeType     string `json:"mime_type"`
		MinSize      int64  `json:"min_size"`
		MaxSize      int64  `json:"max_size"`
		CreatedAfter string `json:"created_after"`
		CreatedBefore string `json:"created_before"`
	}
	
	FileVersionWithRowNumber = struct {
		FileVersion
		RowNumber    int `json:"row_number" db:"row_num"`
		TotalVersions int `json:"total_versions" db:"total_versions"`
	}
	
	DirectoryStats = struct {
		TotalItems     int64  `json:"total_items"`
		FileCount      int64  `json:"file_count"`
		DirectoryCount int64  `json:"directory_count"`
		TotalSize      int64  `json:"total_size"`
		MaxFileSize    int64  `json:"max_file_size"`
		OldestCreated string `json:"oldest_created"`
		NewestCreated  string `json:"newest_created"`
	}
)
