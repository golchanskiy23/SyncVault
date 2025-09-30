DROP FUNCTION IF EXISTS cleanup_old_sync_events();
DROP FUNCTION IF EXISTS update_user_storage_usage(BIGINT);
DROP FUNCTION IF EXISTS cleanup_expired_sessions();

DROP INDEX IF EXISTS idx_query_stats_hash;
DROP INDEX IF EXISTS idx_query_stats_timestamp;
DROP TABLE IF EXISTS query_stats;

DROP POLICY IF EXISTS sync_events_user_policy ON sync_events;
DROP POLICY IF EXISTS sync_jobs_user_policy ON sync_jobs;
DROP POLICY IF EXISTS files_user_policy ON sync_events;

ALTER TABLE sync_events DISABLE ROW LEVEL SECURITY;
ALTER TABLE sync_jobs DISABLE ROW LEVEL SECURITY;
ALTER TABLE files DISABLE ROW LEVEL SECURITY;

DROP TRIGGER IF EXISTS check_storage_quota_trigger ON files;
DROP FUNCTION IF EXISTS check_storage_quota();

DROP INDEX IF EXISTS idx_sessions_active;
DROP INDEX IF EXISTS idx_sync_events_metadata_gin;
DROP INDEX IF EXISTS idx_sync_events_created_status;
DROP INDEX IF EXISTS idx_sync_events_storage_node;
DROP INDEX IF EXISTS idx_sync_jobs_status_priority_created;
DROP INDEX IF EXISTS idx_sync_jobs_queue;
DROP INDEX IF EXISTS idx_sync_jobs_storage_node;
DROP INDEX IF EXISTS idx_sync_jobs_file_id;
DROP INDEX IF EXISTS idx_file_versions_unique_current;
DROP INDEX IF EXISTS idx_file_versions_file_created;
DROP INDEX IF EXISTS idx_file_versions_created_at;
DROP INDEX IF EXISTS idx_file_versions_storage_node;
DROP INDEX IF EXISTS idx_storage_nodes_health_check;
DROP INDEX IF EXISTS idx_files_name_gin;
DROP INDEX IF EXISTS idx_files_user_path_deleted;
DROP INDEX IF EXISTS idx_files_updated_at;
DROP INDEX IF EXISTS idx_files_name;
