-- OAuth states table for PKCE flow
CREATE TABLE IF NOT EXISTS oauth_states (
    id SERIAL PRIMARY KEY,
    state VARCHAR(255) UNIQUE NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    code_verifier TEXT NOT NULL,
    provider VARCHAR(50) NOT NULL DEFAULT 'google',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    is_used BOOLEAN DEFAULT FALSE
);

-- Index for fast lookup
CREATE INDEX IF NOT EXISTS idx_oauth_states_state ON oauth_states(state);
CREATE INDEX IF NOT EXISTS idx_oauth_states_user_id ON oauth_states(user_id);
CREATE INDEX IF NOT EXISTS idx_oauth_states_expires_at ON oauth_states(expires_at);

-- OAuth tokens table for encrypted storage
CREATE TABLE IF NOT EXISTS oauth_tokens (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    provider VARCHAR(50) NOT NULL DEFAULT 'google',
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    token_type VARCHAR(50) NOT NULL DEFAULT 'Bearer',
    expiry TIMESTAMP WITH TIME ZONE NOT NULL,
    scope TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    is_active BOOLEAN DEFAULT TRUE
);

-- Indexes for fast lookup
CREATE INDEX IF NOT EXISTS idx_oauth_tokens_user_id ON oauth_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_oauth_tokens_provider ON oauth_tokens(provider);
CREATE INDEX IF NOT EXISTS idx_oauth_tokens_is_active ON oauth_tokens(is_active);
CREATE INDEX IF NOT EXISTS idx_oauth_tokens_expiry ON oauth_tokens(expiry);

-- Unique constraint to prevent multiple active tokens per user/provider
CREATE UNIQUE INDEX IF NOT EXISTS idx_oauth_tokens_unique_active 
ON oauth_tokens(user_id, provider) 
WHERE is_active = TRUE;

-- Function to automatically update updated_at timestamp
CREATE OR REPLACE FUNCTION update_oauth_tokens_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to automatically update updated_at
CREATE TRIGGER oauth_tokens_updated_at
    BEFORE UPDATE ON oauth_tokens
    FOR EACH ROW
    EXECUTE FUNCTION update_oauth_tokens_updated_at();

-- Google Drive files cache table (optional)
CREATE TABLE IF NOT EXISTS google_drive_files (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    file_id VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    mime_type VARCHAR(100),
    size BIGINT,
    created_time TIMESTAMP WITH TIME ZONE,
    modified_time TIMESTAMP WITH TIME ZONE,
    parents TEXT[], -- Array of parent folder IDs
    web_view_link TEXT,
    checksum_md5 VARCHAR(32),
    sync_version INTEGER DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for Google Drive files
CREATE INDEX IF NOT EXISTS idx_google_drive_files_user_id ON google_drive_files(user_id);
CREATE INDEX IF NOT EXISTS idx_google_drive_files_file_id ON google_drive_files(file_id);
CREATE INDEX IF NOT EXISTS idx_google_drive_files_parent_id ON google_drive_files USING GIN(parents);
CREATE INDEX IF NOT EXISTS idx_google_drive_files_modified_time ON google_drive_files(modified_time);

-- Trigger to automatically update updated_at for Google Drive files
CREATE TRIGGER google_drive_files_updated_at
    BEFORE UPDATE ON google_drive_files
    FOR EACH ROW
    EXECUTE FUNCTION update_oauth_tokens_updated_at();

-- Google Drive sync status table
CREATE TABLE IF NOT EXISTS google_drive_sync_status (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255) UNIQUE NOT NULL,
    last_sync_at TIMESTAMP WITH TIME ZONE,
    sync_status VARCHAR(50) DEFAULT 'pending', -- pending, in_progress, completed, error
    error_message TEXT,
    total_files INTEGER DEFAULT 0,
    synced_files INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for sync status
CREATE INDEX IF NOT EXISTS idx_google_drive_sync_status_user_id ON google_drive_sync_status(user_id);
CREATE INDEX IF NOT EXISTS idx_google_drive_sync_status_status ON google_drive_sync_status(sync_status);

-- Trigger to automatically update updated_at for sync status
CREATE TRIGGER google_drive_sync_status_updated_at
    BEFORE UPDATE ON google_drive_sync_status
    FOR EACH ROW
    EXECUTE FUNCTION update_oauth_tokens_updated_at();

-- Cleanup procedure for expired OAuth states
CREATE OR REPLACE FUNCTION cleanup_expired_oauth_states()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM oauth_states WHERE expires_at < NOW();
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    -- Log the cleanup (optional)
    RAISE NOTICE 'Cleaned up % expired OAuth states', deleted_count;
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Cleanup procedure for inactive OAuth tokens
CREATE OR REPLACE FUNCTION cleanup_inactive_oauth_tokens()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM oauth_tokens 
    WHERE is_active = FALSE 
    AND updated_at < NOW() - INTERVAL '30 days';
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    -- Log the cleanup (optional)
    RAISE NOTICE 'Cleaned up % inactive OAuth tokens older than 30 days', deleted_count;
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Comments for documentation
COMMENT ON TABLE oauth_states IS 'Stores OAuth 2.0 state parameters for PKCE flow';
COMMENT ON TABLE oauth_tokens IS 'Stores encrypted OAuth 2.0 tokens for users';
COMMENT ON TABLE google_drive_files IS 'Cache of Google Drive files metadata';
COMMENT ON TABLE google_drive_sync_status IS 'Tracks synchronization status for Google Drive';

COMMENT ON COLUMN oauth_states.state IS 'Unique state parameter for CSRF protection';
COMMENT ON COLUMN oauth_states.code_verifier IS 'PKCE code verifier for security';
COMMENT ON COLUMN oauth_states.expires_at IS 'Expiration time for security';

COMMENT ON COLUMN oauth_tokens.access_token IS 'Encrypted OAuth access token';
COMMENT ON COLUMN oauth_tokens.refresh_token IS 'Encrypted OAuth refresh token';
COMMENT ON COLUMN oauth_tokens.expiry IS 'Token expiration time';
COMMENT ON COLUMN oauth_tokens.is_active IS 'Whether token is currently active';

COMMENT ON COLUMN google_drive_files.file_id IS 'Google Drive file ID';
COMMENT ON COLUMN google_drive_files.parents IS 'Array of parent folder IDs';
COMMENT ON COLUMN google_drive_files.sync_version IS 'Version for conflict detection';

COMMENT ON COLUMN google_drive_sync_status.sync_status IS 'Current sync status: pending, in_progress, completed, error';
COMMENT ON COLUMN google_drive_sync_status.total_files IS 'Total files discovered during sync';
COMMENT ON COLUMN google_drive_sync_status.synced_files IS 'Number of files successfully synced';
