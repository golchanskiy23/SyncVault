CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_files_name ON files (file_name);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_files_updated_at ON files (updated_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_files_user_path_deleted ON files (user_id, file_path, is_deleted);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_files_name_gin ON files USING gin (to_tsvector('english', file_name));

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_storage_nodes_health_check ON storage_nodes (last_health_check);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_file_versions_storage_node ON file_versions (storage_node_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_file_versions_created_at ON file_versions (created_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_file_versions_file_created ON file_versions (file_id, created_at DESC);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_file_versions_unique_current 
    ON file_versions (file_id) WHERE is_current = true;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_jobs_file_id ON sync_jobs (file_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_jobs_storage_node ON sync_jobs (storage_node_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_jobs_queue ON sync_jobs (status, priority, scheduled_at) 
    WHERE status IN ('pending', 'running');
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_jobs_status_priority_created ON sync_jobs (status, priority DESC, scheduled_at ASC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_events_storage_node ON sync_events (storage_node_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_events_created_status ON sync_events (created_at DESC, status);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sync_events_metadata_gin ON sync_events USING gin (metadata);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sessions_active ON sessions (user_id, expires_at) 
    WHERE expires_at > CURRENT_TIMESTAMP;

CREATE OR REPLACE FUNCTION check_storage_quota()
RETURNS TRIGGER AS $$
DECLARE
    user_quota BIGINT;
    user_used BIGINT;
BEGIN
    SELECT storage_quota_bytes, used_storage_bytes 
    INTO user_quota, user_used
    FROM users 
    WHERE id = NEW.user_id;
    
    IF user_used + NEW.file_size_bytes > user_quota THEN
        RAISE EXCEPTION 'Storage quota exceeded for user %', NEW.user_id;
    END IF;
    
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER check_storage_quota_trigger BEFORE INSERT ON files
    FOR EACH ROW EXECUTE FUNCTION check_storage_quota();

ALTER TABLE files ENABLE ROW LEVEL SECURITY;
ALTER TABLE sync_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE sync_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY files_user_policy ON files
    FOR ALL TO authenticated_users
    USING (user_id = current_setting('app.current_user_id')::BIGINT);

CREATE POLICY sync_jobs_user_policy ON sync_jobs
    FOR ALL TO authenticated_users
    USING (user_id = current_setting('app.current_user_id')::BIGINT);

CREATE POLICY sync_events_user_policy ON sync_events
    FOR ALL TO authenticated_users
    USING (job_id IN (
        SELECT id FROM sync_jobs 
        WHERE user_id = current_setting('app.current_user_id')::BIGINT
    ));

ALTER TABLE files ALTER COLUMN user_id SET STATISTICS 1000;
ALTER TABLE files ALTER COLUMN file_path SET STATISTICS 1000;
ALTER TABLE sync_jobs ALTER COLUMN status SET STATISTICS 100;
ALTER TABLE sync_events ALTER COLUMN event_type SET STATISTICS 100;

CREATE OR REPLACE FUNCTION cleanup_expired_sessions()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM sessions 
    WHERE expires_at < CURRENT_TIMESTAMP;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_user_storage_usage(p_user_id BIGINT)
RETURNS VOID AS $$
BEGIN
    UPDATE users 
    SET used_storage_bytes = (
        SELECT COALESCE(SUM(file_size_bytes), 0)
        FROM files 
        WHERE user_id = p_user_id AND is_deleted = false
    )
    WHERE id = p_user_id;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION cleanup_old_sync_events()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
    cutoff_date TIMESTAMP WITH TIME ZONE := CURRENT_TIMESTAMP - INTERVAL '90 days';
BEGIN
    DELETE FROM sync_events 
    WHERE created_at < cutoff_date;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS query_stats (
    id BIGSERIAL PRIMARY KEY,
    query_hash VARCHAR(64),
    query_text TEXT,
    execution_time_ms INTEGER,
    rows_returned INTEGER,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_query_stats_timestamp ON query_stats (timestamp);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_query_stats_hash ON query_stats (query_hash);
