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
);

CREATE INDEX idx_users_email ON users (email);
CREATE INDEX idx_users_username ON users (username);
CREATE INDEX idx_users_active ON users (is_active);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_token_hash ON sessions (token_hash);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

CREATE INDEX idx_files_user_id ON files (user_id);
CREATE INDEX idx_files_path ON files (user_id, file_path);
CREATE INDEX idx_files_checksum_sha256 ON files (checksum_sha256);
CREATE INDEX idx_files_deleted ON files (is_deleted);

CREATE INDEX idx_storage_nodes_active ON storage_nodes (is_active);
CREATE INDEX idx_storage_nodes_type ON storage_nodes (node_type);

CREATE INDEX idx_file_versions_file_id ON file_versions (file_id);
CREATE INDEX idx_file_versions_checksum ON file_versions (checksum_sha256);
CREATE INDEX idx_file_versions_current ON file_versions (is_current);

CREATE INDEX idx_sync_jobs_user_id ON sync_jobs (user_id);
CREATE INDEX idx_sync_jobs_status ON sync_jobs (status);
CREATE INDEX idx_sync_jobs_type ON sync_jobs (job_type);
CREATE INDEX idx_sync_jobs_priority ON sync_jobs (priority);
CREATE INDEX idx_sync_jobs_scheduled_at ON sync_jobs (scheduled_at);

CREATE INDEX idx_sync_events_job_id ON sync_events (job_id);
CREATE INDEX idx_sync_events_file_id ON sync_events (file_id);
CREATE INDEX idx_sync_events_type ON sync_events (event_type);
CREATE INDEX idx_sync_events_status ON sync_events (status);
CREATE INDEX idx_sync_events_created_at ON sync_events (created_at);

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
WHERE sj.status IN ('pending', 'running')
ORDER BY sj.priority DESC, sj.scheduled_at ASC;
