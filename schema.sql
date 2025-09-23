CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    full_name VARCHAR(100),
    storage_quota_bytes BIGINT DEFAULT 5368709120, -- 5GB
    used_storage_bytes BIGINT DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_accessed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    ip_address INET,
    user_agent TEXT
);

CREATE TABLE files (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    file_size_bytes BIGINT NOT NULL,
    mime_type VARCHAR(100),
    checksum_md5 VARCHAR(32),
    checksum_sha256 VARCHAR(64),
    is_deleted BOOLEAN DEFAULT false,
    current_version_id BIGINT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(user_id, file_path)
);

CREATE TABLE storage_nodes (
    id BIGSERIAL PRIMARY KEY,
    node_name VARCHAR(100) UNIQUE NOT NULL,
    node_type VARCHAR(20) NOT NULL CHECK (node_type IN ('local', 's3', 'gcs', 'azure')),
    endpoint_url TEXT,
    access_key_encrypted TEXT,
    secret_key_encrypted TEXT,
    bucket_name VARCHAR(255),
    region VARCHAR(50),
    is_active BOOLEAN DEFAULT true,
    storage_capacity_bytes BIGINT,
    used_storage_bytes BIGINT DEFAULT 0,
    health_check_url TEXT,
    last_health_check TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE file_versions (
    id BIGSERIAL PRIMARY KEY,
    file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    version_number INTEGER NOT NULL,
    storage_node_id BIGINT REFERENCES storage_nodes(id),
    file_size_bytes BIGINT NOT NULL,
    checksum_sha256 VARCHAR(64) NOT NULL,
    storage_path TEXT NOT NULL,
    is_current BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(file_id, version_number)
);

CREATE TABLE sync_jobs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    job_type VARCHAR(20) NOT NULL CHECK (job_type IN ('upload', 'download', 'sync', 'cleanup')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    priority INTEGER DEFAULT 5 CHECK (priority BETWEEN 1 AND 10),
    source_path TEXT,
    destination_path TEXT,
    file_id BIGINT REFERENCES files(id),
    storage_node_id BIGINT REFERENCES storage_nodes(id),
    total_bytes BIGINT,
    processed_bytes BIGINT DEFAULT 0,
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    scheduled_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sync_events (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT REFERENCES sync_jobs(id) ON DELETE CASCADE,
    file_id BIGINT REFERENCES files(id) ON DELETE CASCADE,
    storage_node_id BIGINT REFERENCES storage_nodes(id),
    event_type VARCHAR(20) NOT NULL CHECK (event_type IN ('created', 'updated', 'deleted', 'copied', 'moved', 'synced', 'error')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
    source_path TEXT,
    destination_path TEXT,
    file_size_bytes BIGINT,
    bytes_transferred BIGINT DEFAULT 0,
    transfer_rate_mbps DECIMAL(10,2),
    error_code VARCHAR(50),
    error_message TEXT,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE
) PARTITION BY RANGE (created_at);


CREATE INDEX idx_users_email ON users (email);
CREATE INDEX idx_users_username ON users (username);
CREATE INDEX idx_users_active ON users (is_active);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_token_hash ON sessions (token_hash);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);
CREATE INDEX idx_sessions_active ON sessions (user_id, expires_at) 
    WHERE expires_at > CURRENT_TIMESTAMP;

CREATE INDEX idx_files_user_id ON files (user_id);
CREATE INDEX idx_files_path ON files (user_id, file_path);
CREATE INDEX idx_files_name ON files (file_name);
CREATE INDEX idx_files_checksum_sha256 ON files (checksum_sha256);
CREATE INDEX idx_files_deleted ON files (is_deleted);
CREATE INDEX idx_files_updated_at ON files (updated_at);
CREATE INDEX idx_files_user_path_deleted ON files (user_id, file_path, is_deleted);

CREATE INDEX idx_files_name_gin ON files USING gin (to_tsvector('english', file_name));

CREATE INDEX idx_storage_nodes_active ON storage_nodes (is_active);
CREATE INDEX idx_storage_nodes_type ON storage_nodes (node_type);
CREATE INDEX idx_storage_nodes_health_check ON storage_nodes (last_health_check);

CREATE INDEX idx_file_versions_file_id ON file_versions (file_id);
CREATE INDEX idx_file_versions_storage_node ON file_versions (storage_node_id);
CREATE INDEX idx_file_versions_checksum ON file_versions (checksum_sha256);
CREATE INDEX idx_file_versions_current ON file_versions (is_current);
CREATE INDEX idx_file_versions_created_at ON file_versions (created_at);
CREATE INDEX idx_file_versions_file_created ON file_versions (file_id, created_at DESC);

CREATE UNIQUE INDEX idx_file_versions_unique_current 
    ON file_versions (file_id) WHERE is_current = true;

CREATE INDEX idx_sync_jobs_user_id ON sync_jobs (user_id);
CREATE INDEX idx_sync_jobs_status ON sync_jobs (status);
CREATE INDEX idx_sync_jobs_type ON sync_jobs (job_type);
CREATE INDEX idx_sync_jobs_priority ON sync_jobs (priority);
CREATE INDEX idx_sync_jobs_scheduled_at ON sync_jobs (scheduled_at);
CREATE INDEX idx_sync_jobs_file_id ON sync_jobs (file_id);
CREATE INDEX idx_sync_jobs_storage_node ON sync_jobs (storage_node_id);

CREATE INDEX idx_sync_jobs_queue ON sync_jobs (status, priority, scheduled_at) 
    WHERE status IN ('pending', 'running');
CREATE INDEX idx_sync_jobs_status_priority_created ON sync_jobs (status, priority DESC, scheduled_at ASC);

CREATE INDEX idx_sync_events_job_id ON sync_events (job_id);
CREATE INDEX idx_sync_events_file_id ON sync_events (file_id);
CREATE INDEX idx_sync_events_storage_node ON sync_events (storage_node_id);
CREATE INDEX idx_sync_events_type ON sync_events (event_type);
CREATE INDEX idx_sync_events_status ON sync_events (status);
CREATE INDEX idx_sync_events_created_at ON sync_events (created_at);
CREATE INDEX idx_sync_events_created_status ON sync_events (created_at DESC, status);

CREATE INDEX idx_sync_events_metadata_gin ON sync_events USING gin (metadata);


DO $$
DECLARE
    start_date DATE := DATE_TRUNC('month', CURRENT_DATE);
    end_date DATE := start_date + INTERVAL '1 year';
    current_date DATE := start_date;
BEGIN
    WHILE current_date < end_date LOOP
        EXECUTE format(
            'CREATE TABLE sync_events_%s PARTITION OF sync_events
             FOR VALUES FROM (%L) TO (%L)',
            to_char(current_date, 'YYYY"m"MM'),
            current_date,
            current_date + INTERVAL '1 month'
        );
        current_date := current_date + INTERVAL '1 month';
    END LOOP;
END $$;

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_files_updated_at BEFORE UPDATE ON files
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_storage_nodes_updated_at BEFORE UPDATE ON storage_nodes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE OR REPLACE FUNCTION manage_current_file_version()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.is_current = true THEN
        UPDATE file_versions 
        SET is_current = false 
        WHERE file_id = NEW.file_id AND id != NEW.id;
        
        UPDATE files 
        SET current_version_id = NEW.id 
        WHERE id = NEW.file_id;
    END IF;
    
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER manage_current_version_trigger AFTER INSERT OR UPDATE ON file_versions
    FOR EACH ROW EXECUTE FUNCTION manage_current_file_version();

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

CREATE VIEW user_storage_stats AS
SELECT 
    u.id,
    u.username,
    u.email,
    u.storage_quota_bytes,
    u.used_storage_bytes,
    COUNT(f.id) as file_count,
    COALESCE(SUM(f.file_size_bytes), 0) as actual_used_bytes,
    ROUND(
        (COALESCE(SUM(f.file_size_bytes), 0)::DECIMAL / u.storage_quota_bytes) * 100, 
        2
    ) as quota_usage_percent,
    u.created_at
FROM users u
LEFT JOIN files f ON u.id = f.user_id AND f.is_deleted = false
GROUP BY u.id, u.username, u.email, u.storage_quota_bytes, u.used_storage_bytes, u.created_at;

CREATE VIEW active_sync_jobs AS
SELECT 
    sj.id,
    sj.job_type,
    sj.status,
    sj.priority,
    u.username,
    f.file_name,
    sn.node_name,
    sj.processed_bytes,
    sj.total_bytes,
    ROUND(
        (sj.processed_bytes::DECIMAL / NULLIF(sj.total_bytes, 0)) * 100, 
        2
    ) as progress_percent,
    sj.started_at,
    EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - sj.started_at)) as duration_seconds
FROM sync_jobs sj
JOIN users u ON sj.user_id = u.id
LEFT JOIN files f ON sj.file_id = f.id
LEFT JOIN storage_nodes sn ON sj.storage_node_id = sn.id
WHERE sj.status IN ('pending', 'running')
ORDER BY sj.priority DESC, sj.scheduled_at ASC;

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


INSERT INTO users (username, email, password_hash, full_name) VALUES
('admin', 'admin@syncvault.com', '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj6ukx.LFvO6', 'Administrator'),
('john_doe', 'john@example.com', '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj6ukx.LFvO6', 'John Doe'),
('jane_smith', 'jane@example.com', '$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj6ukx.LFvO6', 'Jane Smith');

INSERT INTO storage_nodes (node_name, node_type, endpoint_url, bucket_name, region, is_active) VALUES
('local_primary', 'local', '/var/lib/syncvault/storage', NULL, NULL, true),
('s3_backup', 's3', 'https://s3.amazonaws.com', 'syncvault-backup', 'us-east-1', true);

INSERT INTO files (user_id, file_path, file_name, file_size_bytes, mime_type, checksum_sha256) VALUES
(1, '/documents/report.pdf', 'report.pdf', 1024000, 'application/pdf', 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'),
(2, '/images/photo.jpg', 'photo.jpg', 2048000, 'image/jpeg', 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'),
(3, '/videos/presentation.mp4', 'presentation.mp4', 10240000, 'video/mp4', 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855');

ALTER TABLE files ALTER COLUMN user_id SET STATISTICS 1000;
ALTER TABLE files ALTER COLUMN file_path SET STATISTICS 1000;
ALTER TABLE sync_jobs ALTER COLUMN status SET STATISTICS 100;
ALTER TABLE sync_events ALTER COLUMN event_type SET STATISTICS 100;

CREATE TABLE query_stats (
    id BIGSERIAL PRIMARY KEY,
    query_hash VARCHAR(64),
    query_text TEXT,
    execution_time_ms INTEGER,
    rows_returned INTEGER,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

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
