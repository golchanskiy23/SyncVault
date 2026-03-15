package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type QueryAnalyzer struct {
	pool *pgxpool.Pool
}

func NewQueryAnalyzer(pool *pgxpool.Pool) *QueryAnalyzer {
	return &QueryAnalyzer{
		pool: pool,
	}
}

type QueryPlan struct {
	Query          string                 `json:"query"`
	ExecutionTime  time.Duration          `json:"execution_time"`
	TotalCost      float64                `json:"total_cost"`
	PlanningTime   time.Duration          `json:"planning_time"`
	ActualRows     int64                  `json:"actual_rows"`
	PlannedRows    int64                  `json:"planned_rows"`
	SeqScanCount   int                    `json:"seq_scan_count"`
	IndexScanCount int                    `json:"index_scan_count"`
	Details        map[string]interface{} `json:"details"`
}

func (qa *QueryAnalyzer) AnalyzeFileQuery(ctx context.Context, query string, args ...interface{}) (*QueryPlan, error) {
	explainQuery := fmt.Sprintf("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) %s", query)

	rows, err := qa.pool.Query(ctx, explainQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute explain analyze: %w", err)
	}
	defer rows.Close()

	var plan QueryPlan
	if rows.Next() {
		err := rows.Scan(&plan.Query, &plan.ExecutionTime, &plan.TotalCost,
			&plan.PlanningTime, &plan.ActualRows, &plan.PlannedRows,
			&plan.SeqScanCount, &plan.IndexScanCount, &plan.Details)
		if err != nil {
			return nil, fmt.Errorf("failed to scan query plan: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating query plan: %w", err)
	}

	return &plan, nil
}

func (qa *QueryAnalyzer) AnalyzeFileByIDQuery(ctx context.Context, fileID int64) (*QueryPlan, error) {
	query := `
		SELECT id, user_id, file_path, file_name, file_size_bytes,
			   mime_type, checksum_md5, checksum_sha256, is_deleted,
			   current_version_id, created_at, updated_at
		FROM files 
		WHERE id = $1 AND is_deleted = false
	`

	return qa.AnalyzeFileQuery(ctx, query, fileID)
}

func (qa *QueryAnalyzer) AnalyzeFileListQuery(ctx context.Context, userID int64, offset, limit int) (*QueryPlan, error) {
	query := `
		SELECT id, user_id, file_path, file_name, file_size_bytes,
			   mime_type, checksum_md5, checksum_sha256, is_deleted,
			   current_version_id, created_at, updated_at
		FROM files 
		WHERE user_id = $1 AND is_deleted = false
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`

	return qa.AnalyzeFileQuery(ctx, query, userID, limit, offset)
}

func (qa *QueryAnalyzer) AnalyzeFileSearchQuery(ctx context.Context, userID int64, searchTerm string) (*QueryPlan, error) {
	query := `
		SELECT id, user_id, file_path, file_name, file_size_bytes,
			   mime_type, checksum_md5, checksum_sha256, is_deleted,
			   current_version_id, created_at, updated_at
		FROM files 
		WHERE user_id = $1 AND is_deleted = false 
		AND file_name ILIKE $2
		ORDER BY updated_at DESC
	`

	return qa.AnalyzeFileQuery(ctx, query, userID, "%"+searchTerm+"%")
}

func (qa *QueryAnalyzer) AnalyzeVersionHistoryQuery(ctx context.Context, fileID int64) (*QueryPlan, error) {
	query := `
		WITH RECURSIVE versioned_files AS (
			SELECT 
					id, file_id, version_number, storage_node_id, file_size_bytes,
					checksum_sha256, storage_path, is_current, created_at,
					ROW_NUMBER() OVER (PARTITION BY file_id ORDER BY version_number DESC) as row_num,
					COUNT(*) OVER (PARTITION BY file_id) as total_versions
			FROM file_versions 
			WHERE file_id = $1
		)
		SELECT 
				id, file_id, version_number, storage_node_id, file_size_bytes,
				checksum_sha256, storage_path, is_current, created_at,
				row_num, total_versions
		FROM versioned_files
		ORDER BY version_number DESC
	`

	return qa.AnalyzeFileQuery(ctx, query, fileID)
}

func (qa *QueryAnalyzer) AnalyzeDirectoryTreeQuery(ctx context.Context, userID int64) (*QueryPlan, error) {
	query := `
		WITH RECURSIVE directory_tree AS (
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name as name,
					f.file_path as path,
					true as is_dir,
					f.file_size_bytes as file_size,
					f.created_at,
					f.updated_at,
					0 as level
			FROM files f
			WHERE f.user_id = $1 
			AND f.is_deleted = false 
			AND f.parent_id IS NULL
			
			UNION ALL
			
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name as name,
					f.file_path as path,
					true as is_dir,
					f.file_size_bytes as file_size,
					f.created_at,
					f.updated_at,
					dt.level + 1 as level
			FROM files f
			INNER JOIN directory_tree dt ON f.parent_id = dt.id
			WHERE f.user_id = $1 
			AND f.is_deleted = false
		)
		SELECT 
				id, user_id, parent_id, name, path, is_dir, 
				file_size, created_at, updated_at, level
		FROM directory_tree
		ORDER BY path, level, name
	`

	return qa.AnalyzeFileQuery(ctx, query, userID)
}

func (qa *QueryAnalyzer) AnalyzeSyncEventsQuery(ctx context.Context, userID int64, limit int) (*QueryPlan, error) {
	query := `
		SELECT se.id, se.job_id, se.file_id, se.storage_node_id, 
			   se.event_type, se.status, se.source_path, se.destination_path,
			   se.file_size_bytes, se.bytes_transferred, se.transfer_rate_mbps,
			   se.error_code, se.error_message, se.metadata, se.created_at, se.completed_at
		FROM sync_events se
		JOIN sync_jobs sj ON se.job_id = sj.id
		WHERE sj.user_id = $1
		ORDER BY se.created_at DESC
		LIMIT $2
	`

	return qa.AnalyzeFileQuery(ctx, query, userID, limit)
}

func (qa *QueryAnalyzer) GetTableStats(ctx context.Context, tableName string) (*TableStats, error) {
	query := `
		SELECT 
			schemaname,
			tablename,
			attname,
			n_distinct,
			n_tup_ins,
			n_tup_upd,
			n_tup_del,
			n_live_tup,
			vacuum_count,
			autovacuum_count,
			analyze_count,
			autoanalyze_count
		FROM pg_stat_user_tables 
		WHERE schemaname = 'public' AND tablename = $1
	`

	stats := &TableStats{}
	err := qa.pool.QueryRow(ctx, query, tableName).Scan(
		&stats.SchemaName,
		&stats.TableName,
		&stats.ColumnName,
		&stats.NumDistinct,
		&stats.NumInserts,
		&stats.NumUpdates,
		&stats.NumDeletes,
		&stats.LiveTuples,
		&stats.VacuumCount,
		&stats.AutoVacuumCount,
		&stats.AnalyzeCount,
		&stats.AutoAnalyzeCount,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get table stats: %w", err)
	}

	return stats, nil
}

type TableStats struct {
	SchemaName       string `json:"schema_name"`
	TableName        string `json:"table_name"`
	ColumnName       string `json:"column_name"`
	NumDistinct      int64  `json:"n_distinct"`
	NumInserts       int64  `json:"n_tup_ins"`
	NumUpdates       int64  `json:"n_tup_upd"`
	NumDeletes       int64  `json:"n_tup_del"`
	LiveTuples       int64  `json:"n_live_tup"`
	VacuumCount      int64  `json:"vacuum_count"`
	AutoVacuumCount  int64  `json:"autovacuum_count"`
	AnalyzeCount     int64  `json:"analyze_count"`
	AutoAnalyzeCount int64  `json:"autoanalyze_count"`
}

func (qa *QueryAnalyzer) CheckForSeqScans(plan *QueryPlan) []string {
	var warnings []string

	if plan.SeqScanCount > 0 {
		warnings = append(warnings, fmt.Sprintf("Sequential scan detected: %d seq scans", plan.SeqScanCount))
	}

	if plan.TotalCost > 1000 {
		warnings = append(warnings, fmt.Sprintf("High cost query: %.2f", plan.TotalCost))
	}

	if plan.ActualRows > 1000 && plan.PlannedRows < 100 {
		warnings = append(warnings, fmt.Sprintf("Row estimation error: planned %d, actual %d", plan.PlannedRows, plan.ActualRows))
	}

	return warnings
}

func (qa *QueryAnalyzer) RecommendIndexes(ctx context.Context, query string, tableName string) ([]IndexRecommendation, error) {
	plan, err := qa.AnalyzeFileQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze query for index recommendations: %w", err)
	}

	var recommendations []IndexRecommendation

	if plan.SeqScanCount > 0 {
		recommendations = append(recommendations, IndexRecommendation{
			Type:          "btree",
			Columns:       []string{"user_id", "is_deleted"},
			Reason:        "Sequential scan detected on user_id/is_deleted columns",
			EstimatedGain: "High",
		})
	}

	if plan.TotalCost > 500 {
		recommendations = append(recommendations, IndexRecommendation{
			Type:          "btree",
			Columns:       []string{"updated_at"},
			Reason:        "High cost query with ORDER BY updated_at",
			EstimatedGain: "Medium",
		})
	}

	return recommendations, nil
}

type IndexRecommendation struct {
	Type          string   `json:"type"`
	Columns       []string `json:"columns"`
	Reason        string   `json:"reason"`
	EstimatedGain string   `json:"estimated_gain"`
}

func (qa *QueryAnalyzer) GetSlowQueries(ctx context.Context, limit int) ([]SlowQuery, error) {
	query := `
		SELECT 
			query,
			calls,
			total_exec_time,
			rows,
			100.0 * total_exec_time / calls as avg_exec_time_ms,
			stddev_exec_time
		FROM pg_stat_statements 
		WHERE calls > 10
		ORDER BY total_exec_time DESC
		LIMIT $1
	`

	rows, err := qa.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get slow queries: %w", err)
	}
	defer rows.Close()

	var queries []SlowQuery
	for rows.Next() {
		var q SlowQuery
		err := rows.Scan(
			&q.Query,
			&q.Calls,
			&q.TotalExecTime,
			&q.Rows,
			&q.AvgExecTimeMs,
			&q.StddevExecTime,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan slow query: %w", err)
		}
		queries = append(queries, q)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating slow queries: %w", err)
	}

	return queries, nil
}

type SlowQuery struct {
	Query          string  `json:"query"`
	Calls          int64   `json:"calls"`
	TotalExecTime  float64 `json:"total_exec_time"`
	Rows           int64   `json:"rows"`
	AvgExecTimeMs  float64 `json:"avg_exec_time_ms"`
	StddevExecTime float64 `json:"stddev_exec_time"`
}

func (qa *QueryAnalyzer) AnalyzeIndexUsage(ctx context.Context, tableName string) ([]IndexUsage, error) {
	query := `
		SELECT 
			schemaname,
			tablename,
			indexname,
			indexdef,
			idx_scan,
			idx_tup_read,
			idx_tup_fetch
			pg_relation_size(schemaname, tablename) as table_size
		FROM pg_stat_user_indexes 
		WHERE schemaname = 'public' AND tablename = $1
		ORDER BY idx_scan DESC
	`

	rows, err := qa.pool.Query(ctx, query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get index usage: %w", err)
	}
	defer rows.Close()

	var indexes []IndexUsage
	for rows.Next() {
		var idx IndexUsage
		err := rows.Scan(
			&idx.SchemaName,
			&idx.TableName,
			&idx.IndexName,
			&idx.IndexDef,
			&idx.IdxScan,
			&idx.IdxTupRead,
			&idx.IdxTupFetch,
			&idx.TableSize,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan index usage: %w", err)
		}
		indexes = append(indexes, idx)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating index usage: %w", err)
	}

	return indexes, nil
}

type IndexUsage struct {
	SchemaName  string `json:"schema_name"`
	TableName   string `json:"table_name"`
	IndexName   string `json:"index_name"`
	IndexDef    string `json:"index_def"`
	IdxScan     int64  `json:"idx_scan"`
	IdxTupRead  int64  `json:"idx_tup_read"`
	IdxTupFetch int64  `json:"idx_tup_fetch"`
	TableSize   int64  `json:"table_size"`
}

func (qa *QueryAnalyzer) GetBufferUsage(ctx context.Context, query string) (*BufferUsage, error) {
	explainQuery := fmt.Sprintf("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) %s", query)

	rows, err := qa.pool.Query(ctx, explainQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute explain analyze for buffers: %w", err)
	}
	defer rows.Close()

	var usage BufferUsage
	if rows.Next() {
		var plan map[string]interface{}
		err := rows.Scan(&plan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan buffer plan: %w", err)
		}

		if plan, ok := plan["Plan"].(map[string]interface{}); ok {
			if buffers, ok := plan["Buffers"].(map[string]interface{}); ok {
				if shared, ok := buffers["Shared Hit Blocks"].(float64); ok {
					usage.SharedHitBlocks = int64(shared)
				}
				if sharedRead, ok := buffers["Shared Read Blocks"].(float64); ok {
					usage.SharedReadBlocks = int64(sharedRead)
				}
				if local, ok := buffers["Local Hit Blocks"].(float64); ok {
					usage.LocalHitBlocks = int64(local)
				}
				if localRead, ok := buffers["Local Read Blocks"].(float64); ok {
					usage.LocalReadBlocks = int64(localRead)
				}
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating buffer plan: %w", err)
	}

	return &usage, nil
}

type BufferUsage struct {
	SharedHitBlocks  int64 `json:"shared_hit_blocks"`
	SharedReadBlocks int64 `json:"shared_read_blocks"`
	LocalHitBlocks   int64 `json:"local_hit_blocks"`
	LocalReadBlocks  int64 `json:"local_read_blocks"`
}
