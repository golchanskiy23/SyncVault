-- Create additional indexes for performance optimization
-- Simple indexes without CONCURRENTLY to avoid transaction issues

-- Files table indexes
CREATE INDEX IF NOT EXISTS idx_files_name ON files (file_name);
CREATE INDEX IF NOT EXISTS idx_files_updated_at ON files (updated_at);
CREATE INDEX IF NOT EXISTS idx_files_user_path_deleted ON files (user_id, file_path, is_deleted);

-- Storage nodes indexes
CREATE INDEX IF NOT EXISTS idx_storage_nodes_health_check ON storage_nodes (last_health_check);

-- File versions indexes
CREATE INDEX IF NOT EXISTS idx_file_versions_storage_node ON file_versions (storage_node_id);
CREATE INDEX IF NOT EXISTS idx_file_versions_created_at ON file_versions (created_at);
CREATE INDEX IF NOT EXISTS idx_file_versions_file_created ON file_versions (file_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_file_versions_unique_current 
    ON file_versions (file_id) WHERE is_current = true;

-- Sync jobs indexes
CREATE INDEX IF NOT EXISTS idx_sync_jobs_file_id ON sync_jobs (file_id);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_storage_node ON sync_jobs (storage_node_id);

-- Sync events indexes
CREATE INDEX IF NOT EXISTS idx_sync_events_storage_node ON sync_events (storage_node_id);
CREATE INDEX IF NOT EXISTS idx_sync_events_created_status ON sync_events (created_at DESC, status);

-- Sessions indexes
CREATE INDEX IF NOT EXISTS idx_sessions_active ON sessions (user_id, expires_at) 
    WHERE expires_at > CURRENT_TIMESTAMP;
